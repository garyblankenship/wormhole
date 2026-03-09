package wormhole

import (
	"context"
	"fmt"

	"github.com/garyblankenship/wormhole/pkg/types"
)

// Capability represents a provider/model capability.
type Capability string

const (
	CapabilityText          Capability = "text"
	CapabilityStructured    Capability = "structured"
	CapabilityEmbeddings    Capability = "embeddings"
	CapabilityImages        Capability = "images"
	CapabilityAudio         Capability = "audio"
	CapabilityToolCalling   Capability = "tool_calling"
	CapabilityStreaming     Capability = "streaming"
	CapabilityVision        Capability = "vision"
	CapabilityCodeExecution Capability = "code_execution"
)

// ProviderCapabilities returns the capabilities supported by a provider.
func (p *Wormhole) ProviderCapabilities(provider string) *Capabilities {
	if resolved, err := p.resolveProviderName(provider); err == nil {
		provider = resolved
	}

	if configuredProvider, err := p.Provider(provider); err == nil {
		return capabilitiesFromModelCapabilities(provider, configuredProvider.SupportedCapabilities())
	}

	return conservativeProviderCapabilities(provider)
}

// ModelCapabilities returns capabilities for a specific provider/model pair.
func (p *Wormhole) ModelCapabilities(provider, model string) (*Capabilities, error) {
	if provider == "" {
		return nil, fmt.Errorf("provider must be specified")
	}
	if model == "" {
		return nil, fmt.Errorf("model must be specified")
	}

	if p.discoveryService != nil {
		models, err := p.discoveryService.GetModels(context.Background(), provider)
		if err == nil {
			for _, info := range models {
				if info != nil && info.ID == model {
					return capabilitiesFromModelCapabilities(provider, info.Capabilities), nil
				}
			}
		}
	}

	return p.ProviderCapabilities(provider), nil
}

func capabilitiesFromModelCapabilities(provider string, modelCaps []types.ModelCapability) *Capabilities {
	caps := &Capabilities{provider: provider, caps: make(map[Capability]bool)}

	for _, capability := range modelCaps {
		switch capability {
		case types.CapabilityText, types.CapabilityChat:
			caps.caps[CapabilityText] = true
		case types.CapabilityStructured:
			caps.caps[CapabilityStructured] = true
		case types.CapabilityEmbeddings:
			caps.caps[CapabilityEmbeddings] = true
		case types.CapabilityImages:
			caps.caps[CapabilityImages] = true
		case types.CapabilityAudio:
			caps.caps[CapabilityAudio] = true
		case types.CapabilityFunctions:
			caps.caps[CapabilityToolCalling] = true
		case types.CapabilityStream:
			caps.caps[CapabilityStreaming] = true
		case types.CapabilityVision:
			caps.caps[CapabilityVision] = true
		}
	}

	return caps
}

func conservativeProviderCapabilities(provider string) *Capabilities {
	caps := &Capabilities{provider: provider, caps: make(map[Capability]bool)}

	switch provider {
	case providerOpenAI, providerAnthropic, providerGemini, providerOpenRouter, providerOllama:
		caps.caps[CapabilityText] = true
	}

	return caps
}

// Capabilities holds the capabilities of a provider.
type Capabilities struct {
	provider string
	caps     map[Capability]bool
}

// Has returns true if the capability is supported.
func (c *Capabilities) Has(cap Capability) bool {
	if c == nil || c.caps == nil {
		return false
	}
	return c.caps[cap]
}

// All returns all supported capabilities.
func (c *Capabilities) All() []Capability {
	if c == nil || c.caps == nil {
		return nil
	}
	var result []Capability
	for cap, supported := range c.caps {
		if supported {
			result = append(result, cap)
		}
	}
	return result
}

func (c *Capabilities) SupportsText() bool       { return c.Has(CapabilityText) }
func (c *Capabilities) SupportsStructured() bool { return c.Has(CapabilityStructured) }
func (c *Capabilities) SupportsEmbeddings() bool { return c.Has(CapabilityEmbeddings) }
func (c *Capabilities) SupportsToolCalling() bool {
	return c.Has(CapabilityToolCalling)
}
func (c *Capabilities) SupportsStreaming() bool { return c.Has(CapabilityStreaming) }
func (c *Capabilities) SupportsVision() bool    { return c.Has(CapabilityVision) }
func (c *Capabilities) SupportsImages() bool    { return c.Has(CapabilityImages) }
func (c *Capabilities) SupportsAudio() bool     { return c.Has(CapabilityAudio) }
