package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/providers"
	transform "github.com/garyblankenship/wormhole/pkg/providers/transform"
	"github.com/garyblankenship/wormhole/pkg/types"
)

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// Gemini provider implementation
type Gemini struct {
	*providers.BaseProvider
	requestBuilder       *providers.RequestBuilder
	responseTransform    *transform.ResponseTransform
	streamingTransformer *transform.StreamingTransformer
}

var _ types.Provider = (*Gemini)(nil)

// New creates a new Gemini provider
func New(apiKey string, config types.ProviderConfig) *Gemini {
	if config.BaseURL == "" {
		config.BaseURL = defaultBaseURL
	}

	// Gemini authenticates via a ?key= URL query param. Resolve the key (APIKeys[0]
	// when only APIKeys is set) and route it through config.APIKey +
	// QueryParamAuthStrategy so the HTTP wrapper applies ?key= on every attempt AND
	// re-derives it after a 429: the wrapper extracts the failed key from the query
	// param, advances the keyPool, and re-applies the new ?key= on the retry. Baking
	// the key into the URL string (the old approach) made mid-flight key rotation a
	// no-op because the retried request reused the original key.
	if apiKey == "" && len(config.APIKeys) > 0 {
		apiKey = config.APIKeys[0]
	}
	authStrategy := providers.AuthStrategy(&providers.NoAuthStrategy{})
	if apiKey != "" {
		config.APIKey = apiKey
		authStrategy = providers.NewQueryParamAuthStrategy("key")
	}

	return &Gemini{
		BaseProvider:         providers.NewBaseProviderWithAuth("gemini", config, nil, authStrategy, nil),
		requestBuilder:       providers.NewRequestBuilder(),
		responseTransform:    transform.NewResponseTransform(),
		streamingTransformer: nil,
	}
}

// Name returns the provider name
func (g *Gemini) Name() string {
	return "gemini"
}

// SupportedCapabilities returns the capabilities supported by Gemini provider
func (g *Gemini) SupportedCapabilities() []types.ModelCapability {
	return []types.ModelCapability{
		types.CapabilityText,
		types.CapabilityChat,
		types.CapabilityStructured,
		types.CapabilityEmbeddings,
		types.CapabilityImages,
		types.CapabilityStream,
		types.CapabilityFunctions,
	}
}

// Text generates text using Gemini models
func (g *Gemini) Text(ctx context.Context, request types.TextRequest) (*types.TextResponse, error) {
	payload, err := g.buildTextPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent",
		g.GetBaseURL(),
		modelName,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	resp, err := g.transformTextResponse(&response)
	if err != nil {
		return nil, err
	}
	resp.Provider = g.Name()
	return resp, nil
}

// stampProvider sets Provider on the terminal chunk. Sole closer of out;
// exits when the upstream channel closes.
func (g *Gemini) stampProvider(ctx context.Context, in <-chan types.TextChunk) <-chan types.TextChunk {
	out := make(chan types.TextChunk)
	go func() {
		defer close(out)
		for chunk := range in {
			if chunk.IsDone() {
				chunk.Provider = g.Name()
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Stream generates streaming text using Gemini models
func (g *Gemini) Stream(ctx context.Context, request types.TextRequest) (<-chan types.TextChunk, error) {
	payload, err := g.buildTextPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	// alt=sse is REQUIRED: streamGenerateContent defaults to a JSON-array stream,
	// but handleStream parses with an SSE scanner. Without it the live endpoint
	// returns an unparseable array. (Streaming endpoint only — not generateContent.)
	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse",
		g.GetBaseURL(),
		modelName,
	)

	stream, err := g.StreamRequest(ctx, "POST", endpoint, payload)
	if err != nil {
		return nil, err
	}

	return g.stampProvider(ctx, g.handleStream(ctx, stream)), nil
}

// Structured generates structured output using Gemini models
func (g *Gemini) Structured(ctx context.Context, request types.StructuredRequest) (*types.StructuredResponse, error) {
	payload, err := g.buildStructuredPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent",
		g.GetBaseURL(),
		modelName,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformStructuredResponse(&response, request.Schema)
}

// Audio is not supported by Gemini
func (g *Gemini) Audio(ctx context.Context, request types.AudioRequest) (*types.AudioResponse, error) {
	return nil, g.NotImplementedError("Audio")
}

// Images generates images using Gemini's native generateContent endpoint.
func (g *Gemini) Images(ctx context.Context, request types.ImagesRequest) (*types.ImagesResponse, error) {
	payload, err := g.buildImagesPayload(request)
	if err != nil {
		return nil, err
	}

	modelName := normalizeModelResource(request.Model)
	endpoint := fmt.Sprintf("%s/models/%s:generateContent",
		g.GetBaseURL(),
		modelName,
	)

	var response geminiTextResponse
	if err := g.DoRequest(ctx, "POST", endpoint, payload, &response); err != nil {
		return nil, err
	}

	return g.transformImagesResponse(&response, request.Model)
}

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

// buildTextPayload builds the request payload for text generation
func (g *Gemini) buildTextPayload(request types.TextRequest) (map[string]any, error) {
	prepared, _, prepareErr := providers.PrepareMessages(request.Messages)
	if prepareErr != nil {
		prepared = request.Messages
	}
	contents, err := g.transformMessages(prepared, request.Model)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"contents": contents,
	}

	if systemText := mergeSystemInstruction(request.SystemPrompt, request.Messages); systemText != "" {
		payload["systemInstruction"] = map[string]any{
			"parts": []map[string]any{
				{"text": systemText},
			},
		}
	}

	// Add generation config using shared utility
	generationConfig := map[string]any{}
	// Use shared utility for common parameters, then map to Gemini field names
	stdConfig := map[string]any{}
	g.requestBuilder.AddGenerationParams(stdConfig, request.Temperature, request.TopP, request.MaxTokens, request.Stop)

	// Map standard field names to Gemini-specific names
	if maxTokens, ok := stdConfig["max_tokens"]; ok {
		generationConfig["maxOutputTokens"] = maxTokens
	}
	if temp, ok := stdConfig["temperature"]; ok {
		generationConfig["temperature"] = temp
	}
	if topP, ok := stdConfig["top_p"]; ok {
		generationConfig["topP"] = topP
	}
	if stop, ok := stdConfig["stop"]; ok {
		generationConfig["stopSequences"] = stop
	}
	if thinking := geminiThinkingConfig(request.Reasoning); len(thinking) > 0 {
		generationConfig["thinkingConfig"] = thinking
	}

	if len(generationConfig) > 0 {
		payload["generationConfig"] = generationConfig
	}

	// Add tools if provided
	if len(request.Tools) > 0 {
		tools := g.transformTools(request.Tools)
		payload["tools"] = tools

		// Add tool config if specified
		if request.ToolChoice != nil {
			payload["toolConfig"] = g.transformToolChoice(request.ToolChoice)
		}
	}

	for k, v := range g.Config.MergedProviderOptions(request.Model, request.ProviderOptions) {
		payload[k] = v
	}

	return payload, nil
}

func geminiThinkingConfig(reasoning *types.Reasoning) map[string]any {
	if reasoning == nil {
		return nil
	}
	out := make(map[string]any, 2)
	if reasoning.MaxTokens > 0 {
		out["thinkingBudget"] = reasoning.MaxTokens
	}
	if reasoning.Enabled != nil {
		out["includeThoughts"] = *reasoning.Enabled
	}
	return out
}

// buildStructuredPayload builds the request payload for structured generation
func (g *Gemini) buildStructuredPayload(request types.StructuredRequest) (map[string]any, error) {
	// For Gemini, we use response schema in generation config
	textRequest := types.TextRequest{
		BaseRequest:  request.BaseRequest,
		Messages:     request.Messages,
		SystemPrompt: request.SystemPrompt,
	}

	payload, err := g.buildTextPayload(textRequest)
	if err != nil {
		return nil, err
	}

	// Add response schema to generation config
	if generationConfig, ok := payload["generationConfig"].(map[string]any); ok {
		generationConfig["responseMimeType"] = "application/json"
		generationConfig["responseSchema"] = g.transformSchema(request.Schema)
	} else {
		payload["generationConfig"] = map[string]any{
			"responseMimeType": "application/json",
			"responseSchema":   g.transformSchema(request.Schema),
		}
	}

	return payload, nil
}

func normalizeModelResource(model string) string {
	model = strings.TrimPrefix(model, "google/")
	model = strings.TrimPrefix(model, "models/")
	// The result is interpolated directly into a URL path segment
	// (see Text/Structured/Images/StreamText endpoint construction), so
	// metacharacters (/, ?, #, ..) must be percent-escaped here — the
	// single call site that all 4 endpoint builders route through.
	return url.PathEscape(model)
}
