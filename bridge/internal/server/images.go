package server

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/ryderdavid/nacre/bridge/internal/claude"
)

// maxTurnBody bounds the body of the endpoints that can carry images: the
// two per-repo turn endpoints, /v1/repos/{repo}/claude/json and
// /v1/repos/{repo}/claude/stream, and POST /v1/chats/{id}/messages. Every
// other endpoint keeps maxBody.
const maxTurnBody = 32 << 20

// Image limits for one turn request. A message has at most 5 images of at
// most 3 MB each, and the app keeps a request, earlier turns included, to 20
// images and 15 MB; the bridge enforces the same ceilings (in MiB, so a
// client that counts in either unit fits).
const (
	maxImages          = 20
	maxImageBytes      = 3 << 20
	maxImagesBytes     = 15 << 20
	maxImageNameLength = 255
	// maxChatImages is the most images one Chat message may carry. The
	// per-repo turn endpoints keep maxImages for older apps.
	maxChatImages = 5
)

// chatImage is one entry of a turn request's ordered images array.
type chatImage struct {
	MediaType string `json:"media_type"`
	Name      string `json:"name"`
	// Data is the image, base64 (standard alphabet, padded).
	Data string `json:"data"`
}

// imageSignatures are the leading bytes each accepted media type must have.
var imageSignatures = map[string]func([]byte) bool{
	"image/jpeg": func(b []byte) bool { return bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}) },
	"image/png":  func(b []byte) bool { return bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")) },
	"image/gif": func(b []byte) bool {
		return bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a"))
	},
	"image/webp": func(b []byte) bool {
		return len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP"))
	},
}

// decodeImages validates a request's images and decodes them, before
// anything is written or started. The reason names the image (by number and
// name) or the limit that failed.
func decodeImages(in []chatImage) ([]claude.Image, string) {
	if len(in) == 0 {
		return nil, ""
	}
	if len(in) > maxImages {
		return nil, fmt.Sprintf("too many images: %d, at most %d per request", len(in), maxImages)
	}
	out := make([]claude.Image, 0, len(in))
	total := 0
	for i, img := range in {
		label := fmt.Sprintf("image %d", i+1)
		if img.Name != "" {
			label += " (" + claude.OneLine(img.Name, 80) + ")"
		}
		check, ok := imageSignatures[img.MediaType]
		switch {
		case !ok:
			return nil, fmt.Sprintf("%s: media type %q is not image/jpeg, image/png, image/gif or image/webp", label, claude.OneLine(img.MediaType, 40))
		case len(img.Name) > maxImageNameLength:
			return nil, fmt.Sprintf("%s: name is longer than %d bytes", label, maxImageNameLength)
		case strings.ContainsAny(img.Name, "\r\n"):
			return nil, fmt.Sprintf("%s: name has a line break", label)
		case base64.StdEncoding.DecodedLen(len(img.Data)) > maxImageBytes+3:
			return nil, fmt.Sprintf("%s: over the %d MiB limit for one image", label, maxImageBytes>>20)
		}
		data, err := base64.StdEncoding.DecodeString(img.Data)
		switch {
		case err != nil:
			return nil, label + ": data is not valid base64"
		case len(data) == 0:
			return nil, label + ": data is empty"
		case len(data) > maxImageBytes:
			return nil, fmt.Sprintf("%s: over the %d MiB limit for one image", label, maxImageBytes>>20)
		case !check(data):
			return nil, fmt.Sprintf("%s: bytes are not a %s image", label, strings.TrimPrefix(img.MediaType, "image/"))
		}
		total += len(data)
		if total > maxImagesBytes {
			return nil, fmt.Sprintf("images total more than the %d MiB limit per request", maxImagesBytes>>20)
		}
		out = append(out, claude.Image{MediaType: img.MediaType, Name: img.Name, Data: data})
	}
	return out, ""
}

// decodeChatImages validates one Chat message's images: at most
// maxChatImages, then every check of decodeImages.
func decodeChatImages(in []chatImage) ([]claude.Image, string) {
	if len(in) > maxChatImages {
		return nil, fmt.Sprintf("too many images: %d, at most %d per chat message", len(in), maxChatImages)
	}
	return decodeImages(in)
}

// decodeTurn reads a body that can carry images with its own limit (the
// endpoints that call it pass maxTurnBody). It writes the error itself: 413
// with a reason over the limit, otherwise 400.
func decodeTurn(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	err := decodeLimit(r, v, false, limit)
	var tooBig *http.MaxBytesError
	switch {
	case err == nil:
		return true
	case errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("request body is larger than %d MiB; send fewer or smaller images", limit>>20), "")
	default:
		writeError(w, http.StatusBadRequest, "invalid JSON body", err.Error())
	}
	return false
}
