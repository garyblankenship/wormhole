package types

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewIncompleteGenerationError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  *TextResponse
		want      IncompleteGenerationReason
		wantClass ErrorClass
		thinking  bool
	}{
		{
			name: "truncated reasoning",
			response: &TextResponse{
				FinishReason: FinishReasonLength,
				Thinking:     &Thinking{Content: "private reasoning"},
			},
			want:      IncompleteGenerationTruncated,
			wantClass: ErrorClassTruncation,
			thinking:  true,
		},
		{
			name: "reasoning only",
			response: &TextResponse{
				FinishReason: FinishReasonStop,
				Thinking:     &Thinking{Content: "private reasoning"},
			},
			want:      IncompleteGenerationReasoningOnly,
			wantClass: ErrorClassUnknown,
			thinking:  true,
		},
		{
			name:      "malformed empty",
			response:  &TextResponse{FinishReason: FinishReasonStop},
			want:      IncompleteGenerationEmpty,
			wantClass: ErrorClassUnknown,
		},
		{
			name:      "nil response",
			response:  nil,
			want:      IncompleteGenerationEmpty,
			wantClass: ErrorClassUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := NewIncompleteGenerationError(tt.response)
			if err.Reason != tt.want {
				t.Fatalf("reason = %q, want %q", err.Reason, tt.want)
			}
			if err.ReasoningPresent != tt.thinking {
				t.Fatalf("reasoning present = %t", err.ReasoningPresent)
			}
			if IsRetryableError(err) {
				t.Fatal("incomplete generation must not be retryable")
			}
			if got := ClassifyError(err); got != tt.wantClass {
				t.Fatalf("class = %q, want %q", got, tt.wantClass)
			}

			encoded, marshalErr := json.Marshal(err)
			if marshalErr != nil {
				t.Fatalf("marshal error: %v", marshalErr)
			}
			if strings.Contains(string(encoded), "private reasoning") {
				t.Fatalf("error exposed reasoning content: %s", encoded)
			}
		})
	}
}

func TestIncompleteGenerationErrorInterfaces(t *testing.T) {
	t.Parallel()

	err := NewIncompleteGenerationError(&TextResponse{FinishReason: FinishReasonLength})
	wrapped := fmt.Errorf("request failed: %w", err)
	extracted, ok := AsIncompleteGenerationError(wrapped)
	if !ok || extracted != err {
		t.Fatal("wrapped typed error could not be extracted")
	}
	if !errors.Is(wrapped, err.WormholeError) {
		t.Fatal("wrapped error did not expose WormholeError")
	}
	if _, ok := AsIncompleteGenerationError(errors.New("other")); ok {
		t.Fatal("unrelated error classified as incomplete generation")
	}

	var nilErr *IncompleteGenerationError
	if got := nilErr.Error(); got != "incomplete generation" {
		t.Fatalf("nil error string = %q", got)
	}
	if nilErr.Unwrap() != nil {
		t.Fatal("nil error unwrap must be nil")
	}
	zeroErr := &IncompleteGenerationError{}
	if got := zeroErr.Error(); got != "incomplete generation" {
		t.Fatalf("zero error string = %q", got)
	}
}

func TestIncompleteGenerationErrorPreservesSafeMetadata(t *testing.T) {
	t.Parallel()

	response := &TextResponse{
		ID:           "chatcmpl-1",
		Provider:     "openai",
		Model:        "gpt-5.6-luna",
		FinishReason: FinishReasonLength,
		Usage: &Usage{
			PromptTokens:     13217,
			CompletionTokens: 8192,
			TotalTokens:      21409,
			ReasoningTokens:  8192,
		},
	}

	err := NewIncompleteGenerationError(response)
	if err.RequestID != response.ID || err.Provider != response.Provider || err.Model != response.Model {
		t.Fatalf("metadata was not preserved: %+v", err)
	}
	if err.FinishReason != FinishReasonLength || err.Usage == nil || err.Usage.ReasoningTokens != 8192 {
		t.Fatalf("completion metadata was not preserved: %+v", err)
	}
	if err.Usage == response.Usage {
		t.Fatal("usage metadata was not copied")
	}

	extracted, ok := AsIncompleteGenerationError(err)
	if !ok || extracted != err {
		t.Fatal("typed error could not be extracted")
	}
}
