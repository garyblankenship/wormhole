package wormhole

import (
	"github.com/garyblankenship/wormhole/v3/types"
)

// ProviderUsesResponses reports whether the configured route uses the OpenAI
// Responses wire dialect. It does not probe the provider or its models.
func (p *Wormhole) ProviderUsesResponses(name string) bool {
	resolved, err := p.resolveProviderName(name)
	if err != nil {
		return false
	}
	cfg, ok := p.config.Providers[resolved]
	if !ok {
		return false
	}
	if profile, known := providerProfile(resolved); known {
		if profile.Kind != providerKindOpenAICompatible && resolved != providerOpenAI {
			return false
		}
	}
	return cfg.UseResponsesAPI
}

// prepareAttemptMessages retains opaque signatures only on the original route.
// References remain intact: incompatible ownership is rejected, never stripped.
func prepareAttemptMessages(messages []types.Message, original, target string) []types.Message {
	result := types.CloneMessages(messages)
	for _, message := range result {
		assistant, ok := message.(*types.AssistantMessage)
		if !ok {
			continue
		}
		if assistant.Thinking != nil && assistant.Thinking.Signature != "" {
			owner := assistant.Thinking.Provider
			if owner == "" {
				owner = original
			}
			if owner != target {
				assistant.Thinking.Signature = ""
			} else {
				assistant.Thinking.Provider = owner
			}
		}
		if original != target {
			for i := range assistant.ToolCalls {
				assistant.ToolCalls[i].ThoughtSignature = ""
			}
		}
	}
	return result
}

func (p *Wormhole) validateFileOwnership(messages []types.Message, target string) error {
	hasReferences := false
	for _, message := range messages {
		if user, ok := message.(*types.UserMessage); ok {
			for _, media := range user.Media {
				if _, ok := media.(*types.FileReferenceMedia); ok {
					hasReferences = true
				}
			}
		}
	}
	if !hasReferences {
		return nil
	}
	resolved, err := p.resolveProviderName(target)
	if err != nil {
		return err
	}
	for _, message := range messages {
		user, ok := message.(*types.UserMessage)
		if !ok {
			continue
		}
		for _, media := range user.Media {
			reference, ok := media.(*types.FileReferenceMedia)
			if !ok {
				continue
			}
			if err := reference.Validate(); err != nil {
				return err
			}
			if _, registered := p.config.Providers[reference.Provider]; !registered {
				return types.ErrInvalidRequest.WithDetails("file reference owner is not a registered provider/profile")
			}
			if reference.Provider != resolved {
				return types.ErrInvalidRequest.WithDetails("file reference cannot cross provider/profile routes")
			}
		}
	}
	return nil
}
