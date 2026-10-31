package anthropic

import (
	"encoding/base64"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

func appendUserMedia(contentParts []map[string]any, mediaItems []types.Media) []map[string]any {
	for _, media := range mediaItems {
		switch media := media.(type) {
		case *types.FileReferenceMedia:
			contentParts = append(contentParts, map[string]any{"type": media.Kind, "source": map[string]any{"type": "file", "file_id": media.ID}})
		case *types.ImageMedia:
			if media == nil {
				continue
			}
			var source map[string]any
			if header, data, ok := strings.Cut(media.URL, ";base64,"); ok && strings.HasPrefix(header, "data:image/") {
				source = map[string]any{"type": "base64", "media_type": strings.TrimPrefix(header, "data:"), "data": data}
			} else if media.URL != "" {
				source = map[string]any{"type": "url", "url": media.URL}
			} else {
				data := media.Base64Data
				if data == "" && len(media.Data) > 0 {
					data = base64.StdEncoding.EncodeToString(media.Data)
				}
				if data == "" {
					continue
				}
				mimeType := media.MimeType
				if mimeType == "" {
					mimeType = "image/png"
				}
				source = map[string]any{"type": "base64", "media_type": mimeType, "data": data}
			}
			contentParts = append(contentParts, map[string]any{"type": "image", "source": source})
		}
	}
	return contentParts
}
