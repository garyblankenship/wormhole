package types

import (
	"errors"
	"testing"
	"time"
)

func TestNewToolPopulatesOpenAICompatibilityFields(t *testing.T) {
	t.Parallel()

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"city": map[string]any{"type": "string"},
		},
	}

	tool := NewTool("weather", "Get weather", schema)

	if tool.Type != "function" {
		t.Fatalf("Type = %q, want function", tool.Type)
	}
	if tool.Name != "weather" || tool.Description != "Get weather" {
		t.Fatalf("tool identity = (%q, %q), want (weather, Get weather)", tool.Name, tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Fatalf("InputSchema type = %v, want object", tool.InputSchema["type"])
	}
	if tool.Function == nil {
		t.Fatal("Function is nil")
	}
	if tool.Function.Name != tool.Name || tool.Function.Description != tool.Description {
		t.Fatalf("function identity = (%q, %q), want (%q, %q)", tool.Function.Name, tool.Function.Description, tool.Name, tool.Description)
	}
	if tool.Function.Parameters["type"] != "object" {
		t.Fatalf("Function.Parameters type = %v, want object", tool.Function.Parameters["type"])
	}
}

func TestSmallTypeHelpers(t *testing.T) {
	t.Parallel()

	if got := (WormholeProviderError{Message: "provider failed"}).Error(); got != "provider failed" {
		t.Fatalf("provider error = %q, want provider failed", got)
	}
	if got := (&ImageMedia{}).GetType(); got != "image" {
		t.Fatalf("image media type = %q, want image", got)
	}
	if got := (&DocumentMedia{}).GetType(); got != "document" {
		t.Fatalf("document media type = %q, want document", got)
	}
}

func TestGetRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want time.Duration
	}{
		{name: "nil", want: 0},
		{name: "regular error", err: errors.New("plain"), want: 0},
		{name: "non retryable", err: NewWormholeError(ErrorCodeAuth, "auth", false), want: 0},
		{name: "rate limit", err: NewWormholeError(ErrorCodeRateLimit, "rate", true), want: 30 * time.Second},
		{name: "network", err: NewWormholeError(ErrorCodeNetwork, "network", true), want: 5 * time.Second},
		{name: "timeout", err: NewWormholeError(ErrorCodeTimeout, "timeout", true), want: 10 * time.Second},
		{name: "other retryable", err: NewWormholeError(ErrorCodeProvider, "provider", true), want: time.Second},
		{name: "wrapped wormhole error", err: Errorf("call provider", NewWormholeError(ErrorCodeNetwork, "network", true)), want: 5 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := GetRetryAfter(tt.err); got != tt.want {
				t.Fatalf("GetRetryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestErrorfHelpers(t *testing.T) {
	t.Parallel()

	if err := Errorf("read file", nil); err != nil {
		t.Fatalf("Errorf with nil error = %v, want nil", err)
	}
	if err := Errorff("read %s", nil, "file"); err != nil {
		t.Fatalf("Errorff with nil error = %v, want nil", err)
	}

	cause := errors.New("disk")
	err := Errorf("read file", cause)
	if err == nil {
		t.Fatal("Errorf returned nil, want error")
	}
	if got := err.Error(); got != "failed to read file: disk" {
		t.Fatalf("Errorf message = %q, want %q", got, "failed to read file: disk")
	}
	if !errors.Is(err, cause) {
		t.Fatal("Errorf error does not wrap cause")
	}

	err = Errorff("read %s", cause, "config")
	if err == nil {
		t.Fatal("Errorff returned nil, want error")
	}
	if got := err.Error(); got != "failed to read config: disk" {
		t.Fatalf("Errorff message = %q, want %q", got, "failed to read config: disk")
	}
	if !errors.Is(err, cause) {
		t.Fatal("Errorff error does not wrap cause")
	}
}

func TestValidationErrors(t *testing.T) {
	t.Parallel()

	var empty ValidationErrors
	if empty.HasErrors() {
		t.Fatal("empty ValidationErrors has errors")
	}
	if err := empty.Error(); err != nil {
		t.Fatalf("empty ValidationErrors Error() = %v, want nil", err)
	}
	if fields := empty.Fields(); len(fields) != 0 {
		t.Fatalf("empty Fields() length = %d, want 0", len(fields))
	}

	var single ValidationErrors
	single.Add("model", "required", nil, "model is required")
	if !single.HasErrors() {
		t.Fatal("single ValidationErrors has no errors")
	}
	singleErr := single.Error()
	if singleErr == nil {
		t.Fatal("single ValidationErrors Error() returned nil")
	}
	vErr, ok := AsValidationError(singleErr)
	if !ok {
		t.Fatalf("single error type = %T, want ValidationError", singleErr)
	}
	if vErr.Field != "model" || vErr.Constraint != "required" {
		t.Fatalf("single validation error = (%q, %q), want (model, required)", vErr.Field, vErr.Constraint)
	}

	var multi ValidationErrors
	multi.Add("model", "required", nil, "model is required")
	multi.Add("temperature", "range", 3.0, "must be <= 2")
	if got := multi.Fields(); len(got) != 2 || got[0] != "model" || got[1] != "temperature" {
		t.Fatalf("Fields() = %#v, want [model temperature]", got)
	}
	multiErr := multi.Error()
	if multiErr == nil {
		t.Fatal("multi ValidationErrors Error() returned nil")
	}
	if got := multiErr.Error(); got != "VALIDATION_ERROR: validation failed (2 validation errors: model - model: model is required; temperature - temperature: must be <= 2)" {
		t.Fatalf("multi error = %q", got)
	}
}
