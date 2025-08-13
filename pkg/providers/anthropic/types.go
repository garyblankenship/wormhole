package anthropic

// Anthropic API types

type messageResponse struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Content    []contentPart `json:"content"`
	Model      string        `json:"model"`
	StopReason string        `json:"stop_reason"`
	Usage      messageUsage  `json:"usage"`
}

type contentPart struct {
	Type  string    `json:"type"`
	Text  string    `json:"text,omitempty"`
	ID    string    `json:"id,omitempty"`
	Name  string    `json:"name,omitempty"`
	Input toolInput `json:"input,omitempty"`
}

type toolInput map[string]interface{}

type messageUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type streamEvent struct {
	Type string `json:"type"`
}

type messageStartEvent struct {
	Type    string          `json:"type"`
	Message messageResponse `json:"message"`
}

type contentBlockDeltaEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

type messageDeltaEvent struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason string       `json:"stop_reason,omitempty"`
		Usage      messageUsage `json:"usage,omitempty"`
	} `json:"delta"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (e anthropicError) Error() string {
	return e.Message
}
