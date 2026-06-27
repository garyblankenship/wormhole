package openai

import "encoding/json"

// OpenAI API response types

type chatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage usage `json:"usage"`
}

type message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
}

type toolCall struct {
	// Index keys a streaming tool-call fragment. OpenAI streams
	// tool_calls[].function.arguments in fragments across chunks, all sharing
	// the same index; nil on non-streaming responses. Pointer so a real index
	// of 0 is distinguishable from "absent".
	Index    *int   `json:"index,omitempty"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type usage struct {
	PromptTokens          int                 `json:"prompt_tokens"`
	CompletionTokens      int                 `json:"completion_tokens"`
	TotalTokens           int                 `json:"total_tokens"`
	PromptCacheHitTokens  int                 `json:"prompt_cache_hit_tokens,omitempty"`
	PromptTokensDetails   *promptTokensDetail `json:"prompt_tokens_details,omitempty"`
}

type promptTokensDetail struct {
	CachedTokens int `json:"cached_tokens"`
}

type responsesResponse struct {
	ID                string                `json:"id"`
	Object            string                `json:"object"`
	CreatedAt         int64                 `json:"created_at"`
	Model             string                `json:"model"`
	Status            string                `json:"status"`
	Output            []responsesOutputItem `json:"output"`
	OutputText        string                `json:"output_text,omitempty"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details,omitempty"`
	Usage responsesUsage `json:"usage"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type responsesOutputItem struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status,omitempty"`
	Role      string                 `json:"role,omitempty"`
	Content   []responsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
	Raw       map[string]any         `json:"-"`
}

func (i *responsesOutputItem) UnmarshalJSON(data []byte) error {
	type alias responsesOutputItem
	var item alias
	if err := json.Unmarshal(data, &item); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err == nil {
		item.Raw = raw
	}
	*i = responsesOutputItem(item)
	return nil
}

type responsesContentPart struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

type responsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type responsesStreamEvent struct {
	Type     string               `json:"type"`
	Response *responsesResponse   `json:"response,omitempty"`
	Delta    string               `json:"delta,omitempty"`
	ItemID   string               `json:"item_id,omitempty"`
	Item     *responsesOutputItem `json:"item,omitempty"`
}

func (e responsesStreamEvent) responseModel() string {
	if e.Response == nil {
		return ""
	}
	return e.Response.Model
}

type streamChoice struct {
	Index        int          `json:"index"`
	Delta        messageDelta `json:"delta"`
	FinishReason string       `json:"finish_reason,omitempty"`
}

type messageDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []toolCall `json:"tool_calls,omitempty"`
}

type streamResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []streamChoice `json:"choices"`
	Usage   *usage         `json:"usage,omitempty"`
}

type embeddingsResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage usage  `json:"usage"`
}

type rerankResponse struct {
	ID       string `json:"id"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model"`
	Results  []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Document       struct {
			Text  string `json:"text,omitempty"`
			Image string `json:"image,omitempty"`
		} `json:"document"`
	} `json:"results"`
	Usage struct {
		SearchUnits int     `json:"search_units"`
		TotalTokens int     `json:"total_tokens"`
		Cost        float64 `json:"cost"`
	} `json:"usage"`
}

type imageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL     string `json:"url,omitempty"`
		B64JSON string `json:"b64_json,omitempty"`
	} `json:"data"`
}
