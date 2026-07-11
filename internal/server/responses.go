package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/garyblankenship/wormhole/pkg/types"
	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
)

type responsesRequest struct {
	Model              string              `json:"model"`
	Instructions       string              `json:"instructions,omitempty"`
	Input              responsesInput      `json:"input"`
	Tools              []responsesTool     `json:"tools,omitempty"`
	ToolChoice         json.RawMessage     `json:"tool_choice,omitempty"`
	Stream             bool                `json:"stream,omitempty"`
	Store              bool                `json:"store,omitempty"`
	PreviousResponseID string              `json:"previous_response_id,omitempty"`
	Temperature        *float64            `json:"temperature,omitempty"`
	TopP               *float64            `json:"top_p,omitempty"`
	MaxOutputTokens    *int                `json:"max_output_tokens,omitempty"`
	Reasoning          *responsesReasoning `json:"reasoning,omitempty"`
}

type responsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type responsesInput struct {
	Text  string
	Items []responsesInputItem
}

func (i *responsesInput) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &i.Text); err == nil {
		i.Items = nil
		return nil
	}
	if err := json.Unmarshal(data, &i.Items); err != nil {
		return fmt.Errorf("input must be a string or array of response input items")
	}
	return nil
}

type responsesInputItem struct {
	Type        string          `json:"type"`
	Role        string          `json:"role,omitempty"`
	Content     json.RawMessage `json:"content,omitempty"`
	Name        string          `json:"name,omitempty"`
	Arguments   string          `json:"arguments,omitempty"`
	CustomInput string          `json:"input,omitempty"`
	CallID      string          `json:"call_id,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
}

type responsesContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type responsesExecution struct {
	builder     *wormhole.TextRequestBuilder
	model       string
	customTools map[string]bool
}

type responsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokenDetails  any `json:"input_tokens_details"`
	OutputTokenDetails any `json:"output_tokens_details"`
}

type responsesOutputItem struct {
	ID        string                `json:"id"`
	Type      string                `json:"type"`
	Status    string                `json:"status,omitempty"`
	Role      string                `json:"role,omitempty"`
	Content   []responsesOutputText `json:"content"`
	CallID    string                `json:"call_id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Arguments string                `json:"arguments,omitempty"`
	Input     string                `json:"input,omitempty"`
}

type responsesOutputText struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	Annotations []any  `json:"annotations"`
}

type responsesEnvelope struct {
	ID                string                `json:"id"`
	Object            string                `json:"object"`
	CreatedAt         int64                 `json:"created_at"`
	Status            string                `json:"status"`
	Model             string                `json:"model"`
	Output            []responsesOutputItem `json:"output"`
	Usage             *responsesUsage       `json:"usage,omitempty"`
	Error             any                   `json:"error"`
	IncompleteDetails any                   `json:"incomplete_details"`
}

type responsesEvent struct {
	Type         string               `json:"type"`
	Response     *responsesEnvelope   `json:"response,omitempty"`
	OutputIndex  *int                 `json:"output_index,omitempty"`
	ContentIndex *int                 `json:"content_index,omitempty"`
	ItemID       string               `json:"item_id,omitempty"`
	Delta        string               `json:"delta,omitempty"`
	Arguments    string               `json:"arguments,omitempty"`
	Input        string               `json:"input,omitempty"`
	Item         *responsesOutputItem `json:"item,omitempty"`
}

type responsesToolChoiceSelection struct {
	choice       *types.ToolChoice
	allowedTools map[string]bool
}

func (p *proxy) handleResponses(w http.ResponseWriter, r *http.Request) {
	var req responsesRequest
	if err := decodeRequestBody(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Failed to parse request body: "+err.Error(), "invalid_request_error")
		return
	}
	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model_required", "model is required", "invalid_request_error")
		return
	}
	if req.Input.Text == "" && len(req.Input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "input_required", "input is required", "invalid_request_error")
		return
	}
	if req.Store || req.PreviousResponseID != "" {
		writeError(w, http.StatusBadRequest, "unsupported_state", "store and previous_response_id are not supported by the stateless proxy", "invalid_request_error")
		return
	}

	execution, err := p.responsesBuilder(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), "invalid_request_error")
		return
	}
	if req.Stream {
		p.streamResponses(w, r, execution)
		return
	}

	resp, err := execution.builder.Generate(r.Context())
	if err != nil {
		p.logger.Error("responses generation failed", "error", err, "model", req.Model)
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}
	writeJSON(w, http.StatusOK, completedResponsesEnvelope(resp, execution.model, execution.customTools))
}

func (p *proxy) responsesBuilder(req responsesRequest) (responsesExecution, error) {
	messages, err := responsesMessages(req)
	if err != nil {
		return responsesExecution{}, err
	}
	configuredProviders := p.wh.ConfiguredProviders()
	effDefaultProvider := effectiveDefaultProvider(p.defaultProvider, configuredProviders)
	provider, model := parseModelRoute(req.Model, effDefaultProvider, configuredProviders)

	builder := p.wh.Text().Model(model).Messages(messages...)
	customTools := make(map[string]bool)
	availableTools := make(map[string]bool)
	toolSelection, err := parseResponsesToolChoice(req.ToolChoice)
	if err != nil {
		return responsesExecution{}, err
	}
	if provider != "" {
		builder = builder.Using(provider)
	}
	if req.Temperature != nil {
		builder = builder.Temperature(float32(*req.Temperature))
	}
	if req.TopP != nil {
		builder = builder.TopP(float32(*req.TopP))
	}
	if req.MaxOutputTokens != nil {
		builder = builder.MaxTokens(*req.MaxOutputTokens)
	}
	if len(req.Tools) > 0 {
		tools := make([]types.Tool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			switch tool.Type {
			case "function":
				if tool.Name == "" {
					return responsesExecution{}, fmt.Errorf("function tool name is required")
				}
				if toolSelection.allowedTools != nil && !toolSelection.allowedTools[tool.Name] {
					continue
				}
				tools = append(tools, *types.NewTool(tool.Name, tool.Description, tool.Parameters))
				availableTools[tool.Name] = true
			case "custom":
				if tool.Name == "" {
					return responsesExecution{}, fmt.Errorf("custom tool name is required")
				}
				if toolSelection.allowedTools != nil && !toolSelection.allowedTools[tool.Name] {
					continue
				}
				customTools[tool.Name] = true
				schema := map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"input": map[string]any{"type": "string", "description": "Raw custom tool input"}},
					"required":             []string{"input"},
					"additionalProperties": false,
				}
				tools = append(tools, *types.NewTool(tool.Name, tool.Description, schema))
				availableTools[tool.Name] = true
			case "namespace", "web_search":
				// Chat Completions has no namespace-tool equivalent. Namespaces are
				// optional capability groupings, so omit them rather than rejecting
				// an otherwise portable coding request.
				continue
			default:
				return responsesExecution{}, fmt.Errorf("unsupported tool type %q with name %q", tool.Type, tool.Name)
			}
		}
		if len(tools) > 0 {
			builder = builder.Tools(tools...)
		}
	}
	if toolSelection.allowedTools != nil {
		for name := range toolSelection.allowedTools {
			if !availableTools[name] {
				return responsesExecution{}, fmt.Errorf("allowed tool %q has no Chat Completions equivalent", name)
			}
		}
	}
	if choice := toolSelection.choice; choice != nil {
		if choice.Type == types.ToolChoiceTypeAny && len(availableTools) == 0 {
			return responsesExecution{}, fmt.Errorf("tool_choice requires a tool, but no portable tools remain after translation")
		}
		if choice.Type == types.ToolChoiceTypeSpecific && !availableTools[choice.ToolName] {
			return responsesExecution{}, fmt.Errorf("selected tool %q has no Chat Completions equivalent", choice.ToolName)
		}
		builder = builder.ToolChoice(choice)
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		targetProvider := provider
		if targetProvider == "" {
			targetProvider = effDefaultProvider
		}
		if targetProvider == "zai" {
			thinkingType := "enabled"
			if req.Reasoning.Effort == "none" {
				thinkingType = "disabled"
			}
			builder = builder.ProviderOptions(map[string]any{"thinking": map[string]any{"type": thinkingType}})
		} else if req.Reasoning.Effort != "none" {
			builder = builder.Reasoning(types.Reasoning{Effort: types.ReasoningEffort(req.Reasoning.Effort)})
		}
	}
	return responsesExecution{builder: builder, model: model, customTools: customTools}, nil
}

func responsesMessages(req responsesRequest) ([]types.Message, error) {
	messages := make([]types.Message, 0, len(req.Input.Items)+2)
	if req.Instructions != "" {
		messages = append(messages, types.NewSystemMessage(req.Instructions))
	}
	if req.Input.Text != "" {
		return append(messages, types.NewUserMessage(req.Input.Text)), nil
	}
	for _, item := range req.Input.Items {
		switch item.Type {
		case "message":
			text, media, err := responsesContent(item.Content)
			if err != nil {
				return nil, err
			}
			switch item.Role {
			case "developer", "system":
				if len(media) > 0 {
					return nil, fmt.Errorf("image content is only supported on user messages")
				}
				messages = append(messages, types.NewSystemMessage(text))
			case "user":
				messages = append(messages, &types.UserMessage{Content: text, Media: media})
			case "assistant":
				if len(media) > 0 {
					return nil, fmt.Errorf("image content is only supported on user messages")
				}
				messages = append(messages, types.NewAssistantMessage(text))
			default:
				return nil, fmt.Errorf("unsupported message role %q", item.Role)
			}
		case "function_call":
			if item.CallID == "" || item.Name == "" {
				return nil, fmt.Errorf("function_call requires call_id and name")
			}
			messages = appendAssistantToolCall(messages, ChatToolCall{
				ID: item.CallID, Type: "function", Function: ChatToolCallFunction{Name: item.Name, Arguments: item.Arguments},
			})
		case "custom_tool_call":
			if item.CallID == "" || item.Name == "" {
				return nil, fmt.Errorf("custom_tool_call requires call_id and name")
			}
			arguments, err := json.Marshal(map[string]string{"input": item.CustomInput})
			if err != nil {
				return nil, fmt.Errorf("encode custom tool input: %w", err)
			}
			messages = appendAssistantToolCall(messages, ChatToolCall{
				ID: item.CallID, Type: "function", Function: ChatToolCallFunction{Name: item.Name, Arguments: string(arguments)},
			})
		case "function_call_output", "custom_tool_call_output":
			if item.CallID == "" {
				return nil, fmt.Errorf("%s requires call_id", item.Type)
			}
			text, media, err := responsesContent(item.Output)
			if err != nil {
				return nil, err
			}
			if len(media) > 0 {
				return nil, fmt.Errorf("image content is not supported in tool output")
			}
			messages = append(messages, types.NewToolResultMessage(item.CallID, text))
		case "reasoning":
			// Provider reasoning artifacts are not portable to Chat Completions.
		default:
			return nil, fmt.Errorf("unsupported response input item %q", item.Type)
		}
	}
	return messages, nil
}

func appendAssistantToolCall(messages []types.Message, call ChatToolCall) []types.Message {
	toolCall := toWormholeToolCalls([]ChatToolCall{call})[0]
	if len(messages) > 0 {
		if assistant, ok := messages[len(messages)-1].(*types.AssistantMessage); ok {
			assistant.ToolCalls = append(assistant.ToolCalls, toolCall)
			return messages
		}
	}
	assistant := types.NewAssistantMessage("")
	assistant.ToolCalls = []types.ToolCall{toolCall}
	return append(messages, assistant)
}

func responsesContent(raw json.RawMessage) (string, []types.Media, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil, nil
	}
	var parts []responsesContentPart
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", nil, fmt.Errorf("response content must be a string or array of content parts")
	}
	var out strings.Builder
	var media []types.Media
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			out.WriteString(part.Text)
		case "input_image":
			image, err := parseImageURLPart(part.ImageURL)
			if err != nil {
				return "", nil, err
			}
			media = append(media, image)
		default:
			return "", nil, fmt.Errorf("unsupported response content part %q", part.Type)
		}
	}
	return out.String(), media, nil
}

func parseResponsesToolChoice(raw json.RawMessage) (responsesToolChoiceSelection, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return responsesToolChoiceSelection{}, nil
	}
	if choice := parseToolChoice(raw); choice != nil {
		return responsesToolChoiceSelection{choice: choice}, nil
	}
	var item struct {
		Type  string `json:"type"`
		Name  string `json:"name"`
		Mode  string `json:"mode"`
		Tools []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return responsesToolChoiceSelection{}, fmt.Errorf("invalid tool_choice: %w", err)
	}
	switch item.Type {
	case "function", "custom":
		if item.Name == "" {
			return responsesToolChoiceSelection{}, fmt.Errorf("tool_choice %q requires name", item.Type)
		}
		return responsesToolChoiceSelection{choice: &types.ToolChoice{Type: types.ToolChoiceTypeSpecific, ToolName: item.Name}}, nil
	case "allowed_tools":
		if len(item.Tools) == 0 {
			return responsesToolChoiceSelection{}, fmt.Errorf("allowed_tools requires at least one tool")
		}
		allowed := make(map[string]bool, len(item.Tools))
		for _, tool := range item.Tools {
			if (tool.Type != "function" && tool.Type != "custom") || tool.Name == "" {
				return responsesToolChoiceSelection{}, fmt.Errorf("allowed tool type %q with name %q has no Chat Completions equivalent", tool.Type, tool.Name)
			}
			allowed[tool.Name] = true
		}
		var choice types.ToolChoice
		switch item.Mode {
		case "auto":
			choice.Type = types.ToolChoiceTypeAuto
		case "required":
			choice.Type = types.ToolChoiceTypeAny
		default:
			return responsesToolChoiceSelection{}, fmt.Errorf("unsupported allowed_tools mode %q", item.Mode)
		}
		return responsesToolChoiceSelection{choice: &choice, allowedTools: allowed}, nil
	default:
		return responsesToolChoiceSelection{}, fmt.Errorf("unsupported tool_choice type %q", item.Type)
	}
}

func completedResponsesEnvelope(resp *types.TextResponse, model string, customTools map[string]bool) responsesEnvelope {
	outputs := make([]responsesOutputItem, 0, 1+len(resp.ToolCalls))
	if resp.Text != "" {
		outputs = append(outputs, completedMessageOutput("msg_"+resp.ID, resp.Text))
	}
	for _, call := range resp.ToolCalls {
		outputs = append(outputs, completedToolOutput(call, len(outputs), customTools[call.Name]))
	}
	status, incompleteDetails := responsesStatus(resp.FinishReason)
	return responsesEnvelope{ID: "resp_" + resp.ID, Object: "response", CreatedAt: resp.Created.Unix(), Status: status, Model: model,
		Output: outputs, Usage: toResponsesUsage(resp.Usage), Error: nil, IncompleteDetails: incompleteDetails}
}

func responsesStatus(reason types.FinishReason) (string, any) {
	switch reason {
	case types.FinishReasonLength:
		return "incomplete", map[string]string{"reason": "max_output_tokens"}
	case types.FinishReasonContentFilter:
		return "incomplete", map[string]string{"reason": "content_filter"}
	default:
		return "completed", nil
	}
}

func completedMessageOutput(id, text string) responsesOutputItem {
	return responsesOutputItem{ID: id, Type: "message", Status: "completed", Role: "assistant", Content: []responsesOutputText{{Type: "output_text", Text: text, Annotations: []any{}}}}
}

func completedToolOutput(call types.ToolCall, index int, custom bool) responsesOutputItem {
	arguments := ""
	if call.Function != nil {
		arguments = call.Function.Arguments
	}
	if arguments == "" && call.Arguments != nil {
		if encoded, err := json.Marshal(call.Arguments); err == nil {
			arguments = string(encoded)
		}
	}
	callID := call.ID
	if callID == "" {
		callID = fmt.Sprintf("call_%d", index)
	}
	if custom {
		var payload struct {
			Input string `json:"input"`
		}
		_ = json.Unmarshal([]byte(arguments), &payload)
		return responsesOutputItem{ID: fmt.Sprintf("ctc_%d", index), Type: "custom_tool_call", Status: "completed", CallID: callID, Name: call.Name, Input: payload.Input}
	}
	return responsesOutputItem{ID: fmt.Sprintf("fc_%d", index), Type: "function_call", Status: "completed", CallID: callID, Name: call.Name, Arguments: arguments}
}

func toResponsesUsage(usage *types.Usage) *responsesUsage {
	if usage == nil {
		return nil
	}
	return &responsesUsage{InputTokens: usage.PromptTokens, OutputTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens}
}

func (p *proxy) streamResponses(w http.ResponseWriter, r *http.Request, execution responsesExecution) {
	model := execution.model
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported", "api_error")
		return
	}
	stream, err := execution.builder.Stream(r.Context())
	if err != nil {
		status, errType, clientMsg := upstreamErrorStatus(err)
		writeError(w, status, "upstream_error", clientMsg, errType)
		return
	}

	responseID := fmt.Sprintf("resp_wh-%d", time.Now().UnixNano())
	messageID := fmt.Sprintf("msg_wh-%d", time.Now().UnixNano())
	createdAt := time.Now().Unix()
	outputIndex := 0
	messageOpened := false
	var text strings.Builder
	toolDeltas := newStreamToolState()
	tools := map[int]ChatToolCall{}
	var usage *types.Usage
	var finishReason types.FinishReason

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	writeResponsesSSE(w, responsesEvent{Type: "response.created", Response: &responsesEnvelope{ID: responseID, Object: "response", CreatedAt: createdAt, Status: "in_progress", Model: model, Output: []responsesOutputItem{}, Error: nil, IncompleteDetails: nil}})
	flusher.Flush()

	for chunk := range stream {
		if chunk.Error != nil {
			writeResponsesFailure(w, responseID, model, createdAt, chunk.Error)
			flusher.Flush()
			return
		}
		if content := chunk.Content(); content != "" {
			if !messageOpened {
				messageOpened = true
				index := outputIndex
				item := responsesOutputItem{ID: messageID, Type: "message", Status: "in_progress", Role: "assistant", Content: []responsesOutputText{}}
				writeResponsesSSE(w, responsesEvent{Type: "response.output_item.added", OutputIndex: &index, Item: &item})
				outputIndex++
			}
			text.WriteString(content)
			index, contentIndex := 0, 0
			writeResponsesSSE(w, responsesEvent{Type: "response.output_text.delta", OutputIndex: &index, ContentIndex: &contentIndex, ItemID: messageID, Delta: content})
		}
		for _, delta := range toolDeltas.delta(chunk) {
			if delta.Index == nil {
				continue
			}
			current := tools[*delta.Index]
			if delta.ID != "" {
				current.ID = delta.ID
			}
			if delta.Function.Name != "" {
				current.Function.Name = delta.Function.Name
			}
			current.Function.Arguments += delta.Function.Arguments
			tools[*delta.Index] = current
		}
		if chunk.Usage != nil {
			usage = chunk.Usage
		}
		if chunk.FinishReason != nil {
			finishReason = *chunk.FinishReason
		}
		flusher.Flush()
	}

	outputs := make([]responsesOutputItem, 0, outputIndex+len(tools))
	if messageOpened {
		index := 0
		item := completedMessageOutput(messageID, text.String())
		outputs = append(outputs, item)
		writeResponsesSSE(w, responsesEvent{Type: "response.output_item.done", OutputIndex: &index, Item: &item})
	}
	for index := 0; index < len(tools); index++ {
		call := tools[index]
		item := completedToolOutput(types.ToolCall{ID: call.ID, Name: call.Function.Name, Function: &types.ToolCallFunction{Name: call.Function.Name, Arguments: call.Function.Arguments}}, outputIndex, execution.customTools[call.Function.Name])
		idx := outputIndex
		outputs = append(outputs, item)
		added := item
		added.Status = "in_progress"
		added.Arguments = ""
		added.Input = ""
		writeResponsesSSE(w, responsesEvent{Type: "response.output_item.added", OutputIndex: &idx, Item: &added})
		if item.Type == "custom_tool_call" {
			writeResponsesSSE(w, responsesEvent{Type: "response.custom_tool_call_input.delta", OutputIndex: &idx, ItemID: item.ID, Delta: item.Input})
			writeResponsesSSE(w, responsesEvent{Type: "response.custom_tool_call_input.done", OutputIndex: &idx, ItemID: item.ID, Input: item.Input})
		} else {
			writeResponsesSSE(w, responsesEvent{Type: "response.function_call_arguments.delta", OutputIndex: &idx, ItemID: item.ID, Delta: item.Arguments})
			writeResponsesSSE(w, responsesEvent{Type: "response.function_call_arguments.done", OutputIndex: &idx, ItemID: item.ID, Arguments: item.Arguments})
		}
		writeResponsesSSE(w, responsesEvent{Type: "response.output_item.done", OutputIndex: &idx, Item: &item})
		outputIndex++
	}
	status, incompleteDetails := responsesStatus(finishReason)
	completed := responsesEnvelope{ID: responseID, Object: "response", CreatedAt: createdAt, Status: status, Model: model, Output: outputs, Usage: toResponsesUsage(usage), Error: nil, IncompleteDetails: incompleteDetails}
	eventType := "response.completed"
	if status == "incomplete" {
		eventType = "response.incomplete"
	}
	writeResponsesSSE(w, responsesEvent{Type: eventType, Response: &completed})
	flusher.Flush()
}

func writeResponsesSSE(w http.ResponseWriter, event responsesEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
}

func writeResponsesFailure(w http.ResponseWriter, responseID, model string, createdAt int64, err error) {
	_, errType, clientMsg := upstreamErrorStatus(err)
	code := responsesErrorCode(err)
	event := map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id": responseID, "object": "response", "created_at": createdAt, "status": "failed", "model": model,
			"output": []any{}, "error": map[string]any{"code": code, "message": clientMsg, "type": errType},
		},
	}
	data, marshalErr := json.Marshal(event)
	if marshalErr == nil {
		_, _ = fmt.Fprintf(w, "event: response.failed\ndata: %s\n\n", data)
	}
}

func responsesErrorCode(err error) string {
	whErr, ok := types.AsWormholeError(err)
	if !ok {
		return "upstream_error"
	}
	if whErr.Code == types.ErrorCodeProvider && validResponsesErrorCode(whErr.Details) {
		return whErr.Details
	}
	if whErr.Code != "" {
		return string(whErr.Code)
	}
	return "upstream_error"
}

func validResponsesErrorCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
