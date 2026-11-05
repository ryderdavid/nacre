package chat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// Per-chat image budget. The resumed session holds every image a chat has
// sent, so a chat as a whole takes at most 20 images and 15 MB decoded (in
// MiB, as the per-request limits are counted).
const (
	MaxChatImages     = 20
	MaxChatImageBytes = 15 << 20
)

// ImageLimitReason is the refusal of a message that would pass the chat's
// image budget.
const ImageLimitReason = "This chat has reached its image limit. Start a new chat to send more images."

// ErrImageLimit is a message whose images would pass the chat's budget.
var ErrImageLimit = errors.New(ImageLimitReason)

// ErrNoImage is an unknown or missing stored image.
var ErrNoImage = errors.New("unknown image")

// ImageRef is a stored image a user message sent, as its event names it.
type ImageRef struct {
	// ID is the stored file's name, <image-id>.<ext>, and the {image} of
	// GET /v1/chats/{id}/images/{image}.
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	MediaType string `json:"media_type"`
	Bytes     int    `json:"bytes"`
}

var imageExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
}

var imageMedia = map[string]string{
	"jpg":  "image/jpeg",
	"png":  "image/png",
	"gif":  "image/gif",
	"webp": "image/webp",
}

var imageIDRe = regexp.MustCompile(`^im_[a-f0-9]{24}\.(jpg|png|gif|webp)$`)

// imagesDir is chats/<id>/images.
func (m *Manager) imagesDir(id string) string {
	return filepath.Join(m.opts.Dir, id, "images")
}

// overImageBudget reports whether images would take the chat past its
// budget.
func overImageBudget(rec Record, images []claude.Image) bool {
	if len(images) == 0 {
		return false
	}
	n := rec.ImageCount + len(images)
	size := rec.ImageBytes
	for _, img := range images {
		size += int64(len(img.Data))
	}
	return n > MaxChatImages || size > MaxChatImageBytes
}

// storeImagesLocked writes a message's validated images to
// chats/<id>/images (directory 0700, files 0600), in order. On a failure it
// removes what it wrote and returns the error.
func (m *Manager) storeImagesLocked(id string, images []claude.Image) ([]ImageRef, error) {
	if len(images) == 0 {
		return nil, nil
	}
	dir := m.imagesDir(id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store images: %w", err)
	}
	// MkdirAll leaves an existing directory's mode, and the umask may have
	// narrowed a new one; both are set here.
	_ = os.Chmod(filepath.Join(m.opts.Dir, id), 0o700)
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store images: %w", err)
	}
	refs := make([]ImageRef, 0, len(images))
	undo := func() {
		for _, r := range refs {
			_ = os.Remove(filepath.Join(dir, r.ID))
		}
	}
	for i, img := range images {
		ext, ok := imageExt[img.MediaType]
		if !ok {
			undo()
			return nil, fmt.Errorf("image %d: unsupported media type %q", i+1, img.MediaType)
		}
		ref := ImageRef{ID: newID("im_") + "." + ext, Name: img.Name, MediaType: img.MediaType, Bytes: len(img.Data)}
		f, err := os.OpenFile(filepath.Join(dir, ref.ID), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, err = f.Write(img.Data)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				_ = os.Remove(filepath.Join(dir, ref.ID))
			}
		}
		if err != nil {
			undo()
			return nil, fmt.Errorf("store image %d: %w", i+1, err)
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// stagedImages are the stored files of refs, in order, as a turn sends them.
func (m *Manager) stagedImages(id string, refs []ImageRef) []claude.StagedImage {
	out := make([]claude.StagedImage, 0, len(refs))
	for i, r := range refs {
		name := r.Name
		if name == "" {
			name = fmt.Sprintf("image %d", i+1)
		}
		out = append(out, claude.StagedImage{MediaType: r.MediaType, Name: name, Path: filepath.Join(m.imagesDir(id), r.ID)})
	}
	return out
}

// Image returns the path and media type of a chat's stored image.
func (m *Manager) Image(id, image string) (path, mediaType string, err error) {
	if !imageIDRe.MatchString(image) {
		return "", "", ErrNoImage
	}
	m.mu.Lock()
	c := m.chats[id]
	m.mu.Unlock()
	if c == nil {
		return "", "", ErrNotFound
	}
	path = filepath.Join(m.imagesDir(id), image)
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return "", "", ErrNoImage
	}
	ext := image[len(image)-len(filepath.Ext(image))+1:]
	return path, imageMedia[ext], nil
}
