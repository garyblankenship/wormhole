package validation

import (
	"testing"
)

func TestValidateAgainstSchema(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]any
		schema      map[string]any
		shouldError bool
	}{
		{
			name: "simple object validation passes",
			data: map[string]any{
				"name": "John",
				"age":  30,
			},
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"age": map[string]any{
						"type": "number",
					},
				},
				"required": []any{"name", "age"},
			},
			shouldError: false,
		},
		{
			name: "missing required field fails",
			data: map[string]any{
				"name": "John",
			},
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"age": map[string]any{
						"type": "number",
					},
				},
				"required": []any{"name", "age"},
			},
			shouldError: true,
		},
		{
			name: "string length validation passes",
			data: map[string]any{
				"password": "secret123",
			},
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"password": map[string]any{
						"type":      "string",
						"minLength": 8,
						"maxLength": 20,
					},
				},
			},
			shouldError: false,
		},
		{
			name: "string too short fails",
			data: map[string]any{
				"password": "short",
			},
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"password": map[string]any{
						"type":      "string",
						"minLength": 8,
						"maxLength": 20,
					},
				},
			},
			shouldError: true,
		},
		{
			name: "enum validation passes",
			data: map[string]any{
				"status": "active",
			},
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type": "enum",
						"enum": []any{"active", "inactive", "pending"},
					},
				},
			},
			shouldError: false,
		},
		{
			name: "invalid enum value fails",
			data: map[string]any{
				"status": "invalid",
			},
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type": "enum",
						"enum": []any{"active", "inactive", "pending"},
					},
				},
			},
			shouldError: true,
		},
		{
			name:        "nil schema passes validation",
			data:        map[string]any{"test": "value"},
			schema:      nil,
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgainstSchema(tt.data, tt.schema)
			if tt.shouldError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}