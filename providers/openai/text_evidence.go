package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

var _ types.TextEvidenceProvider = (*Provider)(nil)

// TextWithEvidence performs one nonstreaming Chat Completions request. It never
// retries, even when Text's configured retry policy permits retries. The result
// remains available with an error after a complete body fails normalization.
func (p *Provider) TextWithEvidence(ctx context.Context, request types.TextRequest) (*types.TextEvidenceResult, error) {
	if p.Config.UseResponsesAPI {
		return nil, p.ValidationError("text evidence requires Chat Completions")
	}
	if err := p.validateV330Request(request); err != nil {
		return nil, err
	}
	prepared, _, err := providers.PrepareMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	payload := p.buildChatPayload(&request, prepared)
	if stream, exists := payload["stream"]; exists && stream != false {
		return nil, p.ValidationError("text evidence does not support streaming")
	}
	body, metadata, dispatchErr := p.DoRequestOnceWithBody(ctx, http.MethodPost, p.chatCompletionsURL(), payload)
	result := &types.TextEvidenceResult{Evidence: types.TextEvidence{
		Provider: p.Name(), APIFamily: "chat_completions", RawBody: body,
		Complete: body != nil, ResponseMetadata: metadata,
	}}
	if body == nil {
		return result, dispatchErr
	}
	// Decode evidence independently so a malformed normalized field cannot erase
	// the completed original body or unrelated presence-aware usage fields.
	evidenceErr := decodeTextEvidence(body, &result.Evidence)
	if dispatchErr != nil {
		return result, dispatchErr
	}
	if evidenceErr != nil {
		return result, fmt.Errorf("decode generation evidence: %w", evidenceErr)
	}
	var response chatCompletionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return result, fmt.Errorf("normalize generation response: %w", err)
	}
	text := p.transformTextResponse(&response)
	text.ResponseMetadata = types.CloneResponseMetadata(metadata)
	text.Provider = p.Name()
	if text.Model == "" {
		text.Model = request.Model
	}
	result.Response = text
	if text.Text == "" && text.Refusal == "" && len(text.ToolCalls) == 0 {
		return result, types.NewIncompleteGenerationError(text)
	}
	return result, nil
}

func decodeTextEvidence(body []byte, evidence *types.TextEvidence) error {
	var wire struct {
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role         string                          `json:"role"`
				Content      json.RawMessage                 `json:"content"`
				Refusal      json.RawMessage                 `json:"refusal"`
				ToolCalls    []types.TextToolCallEvidence    `json:"tool_calls"`
				FunctionCall *types.TextFunctionCallEvidence `json:"function_call"`
			} `json:"message"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	var envelope struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}
	evidence.RawUsage = envelope.Usage
	evidence.Usage = decodeTokenEvidence(envelope.Usage)
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	evidence.Choices = make([]types.TextChoiceEvidence, len(wire.Choices))
	for i, choice := range wire.Choices {
		evidence.Choices[i] = types.TextChoiceEvidence{
			Index: choice.Index, Role: choice.Message.Role, Content: choice.Message.Content,
			Refusal: choice.Message.Refusal, FinishReason: choice.FinishReason,
			ToolCalls: choice.Message.ToolCalls, FunctionCall: choice.Message.FunctionCall,
		}
	}
	return nil
}

func decodeTokenEvidence(data json.RawMessage) types.TokenUsageEvidence {
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return types.TokenUsageEvidence{}
	}
	var promptDetails, completionDetails map[string]json.RawMessage
	_ = json.Unmarshal(raw["prompt_tokens_details"], &promptDetails)
	_ = json.Unmarshal(raw["completion_tokens_details"], &completionDetails)
	cached := nonnegativeTokenCount(promptDetails["cached_tokens"])
	if _, present := raw["prompt_tokens_details"]; !present {
		cached = nonnegativeTokenCount(raw["prompt_cache_hit_tokens"])
	}
	return types.TokenUsageEvidence{
		PromptTokens:     nonnegativeTokenCount(raw["prompt_tokens"]),
		CompletionTokens: nonnegativeTokenCount(raw["completion_tokens"]),
		TotalTokens:      nonnegativeTokenCount(raw["total_tokens"]),
		CachedTokens:     cached,
		ReasoningTokens:  nonnegativeTokenCount(completionDetails["reasoning_tokens"]),
	}
}

func nonnegativeTokenCount(raw json.RawMessage) *int64 {
	var count *int64
	if err := json.Unmarshal(raw, &count); err != nil || count == nil || *count < 0 {
		return nil
	}
	return count
}
