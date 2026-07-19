package types

// Error types
type WormholeProviderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
	Param   string `json:"param,omitempty"`
}

func (e WormholeProviderError) Error() string {
	return e.Message
}

// OCRResponse represents an OCR (Optical Character Recognition) response
type OCRResponse struct {
	ID       string         `json:"id"`
	Model    string         `json:"model"`
	Text     string         `json:"text"`
	Created  int64          `json:"created"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
