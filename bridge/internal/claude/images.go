package claude

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Image is one image a headless turn sends to Claude, already validated.
type Image struct {
	// MediaType is image/jpeg, image/png, image/gif or image/webp.
	MediaType string
	// Name is the file name shown to Claude as "Image N: <name>".
	Name string
	// Data is the decoded image.
	Data []byte
}

// StagedImage is an Image written to a private file for the length of a turn.
type StagedImage struct {
	MediaType string
	Name      string
	Path      string
}

// UserMessage is the one stream-json user message a turn with images writes
// to claude's stdin (--input-format stream-json): for each image in order an
// "Image N: <name>" text block and its image block, then the prompt as the
// last text block. It ends with a newline, as stream-json input is one JSON
// object per line.
func UserMessage(prompt string, images []StagedImage) ([]byte, error) {
	content := make([]map[string]any, 0, 2*len(images)+1)
	for i, img := range images {
		data, err := os.ReadFile(img.Path)
		if err != nil {
			return nil, fmt.Errorf("read staged image %d: %w", i+1, err)
		}
		content = append(content,
			map[string]any{"type": "text", "text": ImageLabel(i+1, img.Name)},
			map[string]any{"type": "image", "source": map[string]any{
				"type": "base64", "media_type": img.MediaType, "data": base64.StdEncoding.EncodeToString(data),
			}},
		)
	}
	content = append(content, map[string]any{"type": "text", "text": prompt})
	msg := map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": content},
	}
	out, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// ImageLabel is the text block that names image n before its image block.
func ImageLabel(n int, name string) string {
	return "Image " + strconv.Itoa(n) + ": " + name
}
