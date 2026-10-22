package types

import (
	"fmt"
	"strings"
)

// ResponseMetadata describes one upstream HTTP response. Headers contains only
// filtered header values; it never retains the HTTP response or its body.
type ResponseMetadata struct {
	Provider   string              `json:"provider,omitempty"`
	RequestID  string              `json:"request_id,omitempty"`
	StatusCode int                 `json:"status_code,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
}

// ResponseWarning describes an encountered, non-fatal normalization limitation.
type ResponseWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

const (
	WarningUnsupportedNormalization = "unsupported_normalization"
	WarningMalformedCitation        = "malformed_citation"
	WarningAmbiguousReasoning       = "ambiguous_legacy_reasoning"
)

// Source is a retrieved reference scoped to one response. ProviderID preserves
// its native identity when present; equal URLs alone do not establish identity.
type Source struct {
	ID         string         `json:"id"`
	ProviderID string         `json:"provider_id,omitempty"`
	Kind       string         `json:"kind,omitempty"`
	Title      string         `json:"title,omitempty"`
	URL        string         `json:"url,omitempty"`
	Raw        map[string]any `json:"raw,omitempty"`
}

// TextRange is a half-open range of UTF-8 byte offsets in final response text.
type TextRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SourceLocation carries provider-native source coordinates. Character offsets
// retain the provider's unit; they are not offsets into generated output text.
type SourceLocation struct {
	StartPage *int `json:"start_page,omitempty"`
	EndPage   *int `json:"end_page,omitempty"`
	StartChar *int `json:"start_char,omitempty"`
	EndChar   *int `json:"end_char,omitempty"`
}

// Citation links generated output to a source. Absent OutputRange means the
// provider did not report offsets in a unit Wormhole can safely normalize.
type Citation struct {
	SourceID       string          `json:"source_id"`
	OutputRange    *TextRange      `json:"output_range,omitempty"`
	SourceLocation *SourceLocation `json:"source_location,omitempty"`
	Raw            map[string]any  `json:"raw,omitempty"`
}

// FileReferenceMedia identifies a file already managed by a registered provider
// or profile. Wormhole performs no file upload, fetch, or existence probing.
type FileReferenceMedia struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	ID       string `json:"id,omitempty"`
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

func (m *FileReferenceMedia) GetType() string { return m.Kind }

// Validate checks provider-independent invariants. Adapters additionally enforce
// their supported kinds, reference representations, and MIME requirements.
func (m *FileReferenceMedia) Validate() error {
	if m == nil {
		return ErrInvalidRequest.WithDetails("file reference must not be nil")
	}
	if strings.TrimSpace(m.Provider) == "" {
		return ErrInvalidRequest.WithDetails("file reference requires a registered provider/profile")
	}
	if (strings.TrimSpace(m.ID) == "") == (strings.TrimSpace(m.URI) == "") {
		return ErrInvalidRequest.WithDetails("file reference requires exactly one of ID or URI")
	}
	switch m.Kind {
	case "image", "document", "audio", "video":
	default:
		return ErrInvalidRequest.WithDetails(fmt.Sprintf("unsupported file reference kind %q", m.Kind))
	}
	return nil
}

// ToolCallsRequiredError reports a successful application-tool turn that must
// be completed before a structured result can be decoded.
type ToolCallsRequiredError struct{ Response *StructuredResponse }

func (e *ToolCallsRequiredError) Error() string {
	return "application tool calls must be completed before decoding structured output"
}

// WithResponseMetadata returns an error carrying a detached metadata snapshot.
func (e *WormholeError) WithResponseMetadata(metadata *ResponseMetadata) *WormholeError {
	cloned := *e
	cloned.ResponseMetadata = CloneResponseMetadata(metadata)
	return &cloned
}

// CloneResponseMetadata returns metadata with detached header slices.
func CloneResponseMetadata(src *ResponseMetadata) *ResponseMetadata {
	if src == nil {
		return nil
	}
	return CloneValue(src).(*ResponseMetadata)
}

// CloneUsage returns usage with a detached cache-write breakdown.
func CloneUsage(src *Usage) *Usage {
	if src == nil {
		return nil
	}
	return CloneValue(src).(*Usage)
}

// CloneSources returns sources with detached native data.
func CloneSources(src []Source) []Source {
	if src == nil {
		return nil
	}
	return CloneValue(src).([]Source)
}

// CloneCitations returns citations with detached coordinates and native data.
func CloneCitations(src []Citation) []Citation {
	if src == nil {
		return nil
	}
	return CloneValue(src).([]Citation)
}

// CloneWarnings returns a detached warning slice.
func CloneWarnings(src []ResponseWarning) []ResponseWarning {
	return append([]ResponseWarning(nil), src...)
}

// CloneStructuredResponse returns a fully detached structured response.
func CloneStructuredResponse(src *StructuredResponse) *StructuredResponse {
	if src == nil {
		return nil
	}
	return CloneValue(src).(*StructuredResponse)
}
