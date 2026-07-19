package gemini

import (
	"context"
	"encoding/base64"

	"github.com/garyblankenship/wormhole/v2/types"
)

// GenerateImage generates images through the unified image-generation interface.
func (g *Gemini) GenerateImage(ctx context.Context, request types.ImageRequest) (*types.ImageResponse, error) {
	return g.Images(ctx, request)
}

func (g *Gemini) buildImagesPayload(request types.ImagesRequest) (map[string]any, error) {
	generationConfig := map[string]any{
		"responseModalities": []string{"TEXT", "IMAGE"},
	}
	parts := []map[string]any{{"text": request.Prompt}}
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": parts,
			},
		},
		"generationConfig": generationConfig,
	}

	options := g.Config.MergedProviderOptions(request.Model, request.ProviderOptions)
	if err := g.addImageReferenceParts(&parts, options); err != nil {
		return nil, err
	}
	if len(parts) > 1 {
		payload["contents"].([]map[string]any)[0]["parts"] = parts
	}
	g.addImageConfig(generationConfig, options)

	for k, v := range options {
		switch k {
		case "images", "aspect_ratio", "image_size":
			continue
		case "generationConfig":
			if opts, ok := v.(map[string]any); ok {
				for optKey, optValue := range opts {
					generationConfig[optKey] = optValue
				}
				continue
			}
		}
		payload[k] = v
	}

	return payload, nil
}

func (g *Gemini) addImageReferenceParts(parts *[]map[string]any, options map[string]any) error {
	if len(options) == 0 {
		return nil
	}
	images, ok := options["images"]
	if !ok || images == nil {
		return nil
	}

	switch typed := images.(type) {
	case []ImageInput:
		for _, image := range typed {
			part, err := g.imageInputPart(image)
			if err != nil {
				return err
			}
			*parts = append(*parts, part)
		}
	case []*ImageInput:
		for _, image := range typed {
			if image == nil {
				return g.ValidationError("Gemini image reference is nil")
			}
			part, err := g.imageInputPart(*image)
			if err != nil {
				return err
			}
			*parts = append(*parts, part)
		}
	default:
		return g.ValidationError("Gemini images provider option must be []gemini.ImageInput")
	}
	return nil
}

func (g *Gemini) imageInputPart(image ImageInput) (map[string]any, error) {
	data := image.Base64Data
	if data == "" && len(image.Data) > 0 {
		data = base64.StdEncoding.EncodeToString(image.Data)
	}
	if data == "" {
		return nil, g.ValidationError("Gemini requires inline image data")
	}
	mimeType := image.MimeType
	if mimeType == "" {
		mimeType = "image/png"
	}
	return map[string]any{
		"inlineData": map[string]any{
			"mimeType": mimeType,
			"data":     data,
		},
	}, nil
}

func (g *Gemini) addImageConfig(generationConfig map[string]any, options map[string]any) {
	imageConfig := map[string]any{}
	if aspectRatio, ok := options["aspect_ratio"].(string); ok && aspectRatio != "" {
		imageConfig["aspectRatio"] = aspectRatio
	}
	if imageSize, ok := options["image_size"].(string); ok && imageSize != "" {
		imageConfig["imageSize"] = imageSize
	}
	if len(imageConfig) > 0 {
		generationConfig["imageConfig"] = imageConfig
	}
}
