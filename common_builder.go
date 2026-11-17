package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

// CommonBuilder contains shared fields and methods for all request builders
type CommonBuilder struct {
	wormhole *Wormhole
	provider string
	baseURL  string
}

// newCommonBuilder creates a new CommonBuilder with the given wormhole instance
func newCommonBuilder(wormhole *Wormhole) CommonBuilder {
	return CommonBuilder{
		wormhole: wormhole,
		provider: wormhole.config.DefaultProvider,
	}
}

// getWormhole returns the wormhole instance
func (cb *CommonBuilder) getWormhole() *Wormhole {
	return cb.wormhole
}

// getProvider returns the current provider
func (cb *CommonBuilder) getProvider() string {
	return cb.provider
}

// setProvider sets the provider to use
func (cb *CommonBuilder) setProvider(provider string) {
	cb.provider = provider
}

// getBaseURL returns the current base URL
func (cb *CommonBuilder) getBaseURL() string {
	return cb.baseURL
}

// setBaseURL sets a custom base URL for OpenAI-compatible APIs
func (cb *CommonBuilder) setBaseURL(url string) {
	cb.baseURL = url
}

// getProviderWithBaseURL gets a provider lease for the duration of a request.
// When BaseURL is overridden, a temporary provider is created with the full
// configured provider settings preserved and only BaseURL changed.
func (cb *CommonBuilder) getProviderWithBaseURL() (types.Provider, func(), error) {
	return cb.getProviderWithBaseURLContext(context.Background())
}

func (cb *CommonBuilder) getProviderWithBaseURLContext(ctx context.Context) (types.Provider, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if cb.getBaseURL() == "" {
		return cb.getWormhole().leaseProviderContext(ctx, cb.getProvider())
	}

	providerName, err := cb.getWormhole().resolveProviderName(cb.getProvider())
	if err != nil {
		return nil, nil, err
	}

	config, err := cb.getWormhole().configuredProviderConfig(providerName)
	if err != nil {
		return nil, nil, err
	}
	config.BaseURL = cb.getBaseURL()

	w := cb.getWormhole()
	if !w.beginProviderAcquisition() {
		return nil, nil, fmt.Errorf("client is shutting down")
	}
	type result struct {
		provider types.Provider
		err      error
	}
	ready := make(chan result)
	go func() {
		defer w.providerAcquisitionWg.Done()
		provider, err := w.createProviderWithConfig(providerName, config)
		select {
		case ready <- result{provider, err}:
		case <-ctx.Done():
			if provider != nil {
				_ = provider.Close()
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case initialized := <-ready:
		if initialized.err != nil {
			return nil, nil, initialized.err
		}
		if err := ctx.Err(); err != nil {
			_ = initialized.provider.Close()
			return nil, nil, err
		}
		if w.finishProviderAcquisition() {
			_ = initialized.provider.Close()
			return nil, nil, fmt.Errorf("client is shutting down")
		}
		return initialized.provider, func() { _ = initialized.provider.Close() }, nil
	}
}

// executeProviderOperation owns the provider lease and operation context shared
// by non-streaming request builders. The caller retains modality-specific
// middleware and provider invocation.
func executeProviderOperation[T any](
	ctx context.Context,
	builder *CommonBuilder,
	operation string,
	invoke func(context.Context, types.Provider) (T, error),
) (T, error) {
	var zero T
	provider, release, err := builder.getProviderWithBaseURLContext(ctx)
	if err != nil {
		return zero, err
	}
	defer release()

	return invoke(contextWithProviderOperation(ctx, provider, operation, builder.getBaseURL()), provider)
}

func (cb *CommonBuilder) idempotencyScope(operation string) string {
	providerName, err := cb.getWormhole().resolveProviderName(cb.getProvider())
	if err != nil {
		providerName = cb.getProvider()
	}
	return operation + ":" + providerName + ":" + cb.getBaseURL()
}

func cloneBaseRequestFields(dst, src *types.BaseRequest) {
	if src.Temperature != nil {
		temp := *src.Temperature
		dst.Temperature = &temp
	}
	if src.TopP != nil {
		topP := *src.TopP
		dst.TopP = &topP
	}
	if src.MaxTokens != nil {
		maxTokens := *src.MaxTokens
		dst.MaxTokens = &maxTokens
	}
	if src.PresencePenalty != nil {
		presencePenalty := *src.PresencePenalty
		dst.PresencePenalty = &presencePenalty
	}
	if src.FrequencyPenalty != nil {
		frequencyPenalty := *src.FrequencyPenalty
		dst.FrequencyPenalty = &frequencyPenalty
	}
	if src.Seed != nil {
		seed := *src.Seed
		dst.Seed = &seed
	}
	if src.ParallelToolCalls != nil {
		parallelToolCalls := *src.ParallelToolCalls
		dst.ParallelToolCalls = &parallelToolCalls
	}
	if src.Reasoning != nil {
		reasoning := *src.Reasoning
		if src.Reasoning.IncludeThoughts != nil {
			include := *src.Reasoning.IncludeThoughts
			reasoning.IncludeThoughts = &include
		}
		if src.Reasoning.Enabled != nil {
			enabled := *src.Reasoning.Enabled
			reasoning.Enabled = &enabled
		}
		dst.Reasoning = &reasoning
	}
	if len(src.Stop) > 0 {
		dst.Stop = make([]string, len(src.Stop))
		copy(dst.Stop, src.Stop)
	}
	dst.ProviderOptions = cloneProviderOptions(src.ProviderOptions)
}

// cloneProviderOptions returns a detached copy of provider options.
func cloneProviderOptions(src map[string]any) map[string]any {
	return types.CloneMap(src)
}

func prepareExecutionMessages(systemPrompt string, messages []types.Message) []types.Message {
	if systemPrompt == "" {
		return messages
	}
	result := make([]types.Message, 0, 1+len(messages))
	result = append(result, types.NewSystemMessage(systemPrompt))
	result = append(result, messages...)
	return result
}
