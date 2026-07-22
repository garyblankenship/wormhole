package types

import (
	"errors"
	"fmt"
	"strings"
)

// ModelConstraintError represents a model-specific constraint violation
type ModelConstraintError struct {
	*WormholeError
	Constraint string `json:"constraint"`
	Expected   any    `json:"expected"`
	Actual     any    `json:"actual"`
}

// NewModelConstraintError creates a new model constraint error
func NewModelConstraintError(model, constraint string, expected, actual any) *ModelConstraintError {
	baseErr := ErrProviderConstraintError.
		WithModel(model).
		WithDetails(fmt.Sprintf("constraint '%s' violated: expected %v, got %v", constraint, expected, actual))

	return &ModelConstraintError{
		WormholeError: baseErr,
		Constraint:    constraint,
		Expected:      expected,
		Actual:        actual,
	}
}

// ValidationError represents a field-level validation failure with details about
// which field failed and why. Use Validate() on builders to catch these errors
// before calling Generate().
//
// Example:
//
//	if err := builder.Validate(); err != nil {
//	    if vErr, ok := types.AsValidationError(err); ok {
//	        fmt.Printf("Field %s: %s\n", vErr.Field, vErr.Message)
//	    }
//	}
type ValidationError struct {
	*WormholeError
	Field      string `json:"field"`                // The field that failed validation
	Constraint string `json:"constraint,omitempty"` // The constraint that was violated (e.g., "required", "range")
	Value      any    `json:"value,omitempty"`      // The invalid value (if safe to include)
}

// NewValidationError creates a validation error for a specific field.
//
// Example:
//
//	NewValidationError("model", "required", nil, "model is required")
//	NewValidationError("temperature", "range", 3.0, "must be between 0.0 and 2.0")
func NewValidationError(field, constraint string, value any, message string) *ValidationError {
	return &ValidationError{
		WormholeError: ErrValidation.WithDetails(fmt.Sprintf("%s: %s", field, message)),
		Field:         field,
		Constraint:    constraint,
		Value:         value,
	}
}

// Unwrap returns the embedded WormholeError so its classification remains
// available through ValidationError wrappers.
func (e *ValidationError) Unwrap() error {
	return e.WormholeError
}

// AsValidationError extracts a ValidationError from an error if present.
//
// Example:
//
//	if vErr, ok := types.AsValidationError(err); ok {
//	    log.Printf("Validation failed for field: %s", vErr.Field)
//	}
func AsValidationError(err error) (*ValidationError, bool) {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return validationErr, true
	}
	return nil, false
}

// ValidationErrors collects multiple validation errors for batch reporting.
// Use this when validating multiple fields at once.
//
// Example:
//
//	var errs types.ValidationErrors
//	if model == "" {
//	    errs.Add("model", "required", nil, "model is required")
//	}
//	if temp < 0 || temp > 2 {
//	    errs.Add("temperature", "range", temp, "must be between 0.0 and 2.0")
//	}
//	if errs.HasErrors() {
//	    return errs.Error()
//	}
type ValidationErrors struct {
	Errors []*ValidationError `json:"errors"`
}

// Add appends a new validation error.
func (ve *ValidationErrors) Add(field, constraint string, value any, message string) {
	ve.Errors = append(ve.Errors, NewValidationError(field, constraint, value, message))
}

// HasErrors returns true if any validation errors were collected.
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// Error returns a combined error if there are validation errors, nil otherwise.
func (ve *ValidationErrors) Error() error {
	if !ve.HasErrors() {
		return nil
	}
	if len(ve.Errors) == 1 {
		return ve.Errors[0]
	}
	// Combine into summary using strings.Builder for efficiency
	var builder strings.Builder
	fmt.Fprintf(&builder, "%d validation errors: ", len(ve.Errors))
	for i, e := range ve.Errors {
		if i > 0 {
			builder.WriteString("; ")
		}
		builder.WriteString(e.Field)
		builder.WriteString(" - ")
		builder.WriteString(e.Details)
	}
	return ErrValidation.WithDetails(builder.String())
}

// Fields returns a list of fields that failed validation.
func (ve *ValidationErrors) Fields() []string {
	fields := make([]string, len(ve.Errors))
	for i, e := range ve.Errors {
		fields[i] = e.Field
	}
	return fields
}
