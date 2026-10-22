package ollama

import "github.com/garyblankenship/wormhole/v3/types"

func (p *Provider) rejectFileReferences(messages []types.Message) error {
	for _, message := range messages {
		if user, ok := message.(*types.UserMessage); ok {
			for _, media := range user.Media {
				if _, ok := media.(*types.FileReferenceMedia); ok {
					return p.ValidationError("Ollama does not support provider-owned file references")
				}
			}
		}
	}
	return nil
}
