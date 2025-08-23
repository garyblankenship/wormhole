package types

import (
	"encoding/json"
)

// Role represents the role of a message in a conversation
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in a conversation
type Message interface {
	GetRole() Role
	GetContent() any
}

// BaseMessage provides common message functionality
type BaseMessage struct {
	Role    Role        `json:"role"`
	Content any `json:"content"`
}

func (m BaseMessage) GetRole() Role {
	return m.Role
}

func (m BaseMessage) GetContent() any {
	return m.Content
}

func (m BaseMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Role    Role        `json:"role"`
		Content any `json:"content"`
	}{
		Role:    m.Role,
		Content: m.Content,
	})
}

// SystemMessage represents a system message
type SystemMessage struct {
	Content string `json:"content"`
}

func (m *SystemMessage) GetRole() Role {
	return RoleSystem
}

func (m *SystemMessage) GetContent() any {
	return m.Content
}

func (m *SystemMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Role    Role   `json:"role"`
		Content string `json:"content"`
	}{
		Role:    RoleSystem,
		Content: m.Content,
	})
}

// NewSystemMessage creates a new system message
func NewSystemMessage(content string) *SystemMessage {
	return &SystemMessage{
		Content: content,
	}
}

// UserMessage represents a user message
type UserMessage struct {
	Content string  `json:"content"`
	Media   []Media `json:"media,omitempty"`
}

func (m *UserMessage) GetRole() Role {
	return RoleUser
}

func (m *UserMessage) GetContent() any {
	return m.Content
}

func (m *UserMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Role    Role    `json:"role"`
		Content string  `json:"content"`
		Media   []Media `json:"media,omitempty"`
	}{
		Role:    RoleUser,
		Content: m.Content,
		Media:   m.Media,
	})
}

// NewUserMessage creates a new user message
func NewUserMessage(content string) *UserMessage {
	return &UserMessage{
		Content: content,
	}
}

// AssistantMessage represents an assistant message
type AssistantMessage struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

func (m *AssistantMessage) GetRole() Role {
	return RoleAssistant
}

func (m *AssistantMessage) GetContent() any {
	return m.Content
}

func (m *AssistantMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Role      Role       `json:"role"`
		Content   string     `json:"content"`
		ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	}{
		Role:      RoleAssistant,
		Content:   m.Content,
		ToolCalls: m.ToolCalls,
	})
}

// NewAssistantMessage creates a new assistant message
func NewAssistantMessage(content string) *AssistantMessage {
	return &AssistantMessage{
		Content: content,
	}
}

// ToolMessage represents a tool result message (alias for ToolResultMessage)
type ToolMessage = ToolResultMessage

// ToolResultMessage represents a tool result message
type ToolResultMessage struct {
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}

func (m *ToolResultMessage) GetRole() Role {
	return RoleTool
}

func (m *ToolResultMessage) GetContent() any {
	return m.Content
}

func (m *ToolResultMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Role       Role   `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
	}{
		Role:       RoleTool,
		Content:    m.Content,
		ToolCallID: m.ToolCallID,
	})
}

// NewToolResultMessage creates a new tool result message
func NewToolResultMessage(toolCallID string, content string) *ToolResultMessage {
	return &ToolResultMessage{
		Content:    content,
		ToolCallID: toolCallID,
	}
}

// MessagePart represents a part of a multi-modal message
type MessagePart struct {
	Type string      `json:"type"`
	Text string      `json:"text,omitempty"`
	Data any `json:"data,omitempty"`
}

// TextPart creates a text message part
func TextPart(text string) MessagePart {
	return MessagePart{
		Type: "text",
		Text: text,
	}
}

// ImagePart creates an image message part
func ImagePart(data any) MessagePart {
	return MessagePart{
		Type: "image",
		Data: data,
	}
}

// Media represents media content in a message
type Media interface {
	GetType() string
}

// ImageMedia represents an image in a message
type ImageMedia struct {
	URL        string `json:"url,omitempty"`
	Data       []byte `json:"data,omitempty"`
	Base64Data string `json:"base64_data,omitempty"`
	MimeType   string `json:"mime_type"`
}

func (m *ImageMedia) GetType() string {
	return "image"
}

// DocumentMedia represents a document in a message
type DocumentMedia struct {
	URL      string `json:"url,omitempty"`
	Data     []byte `json:"data,omitempty"`
	MimeType string `json:"mime_type"`
}

func (m *DocumentMedia) GetType() string {
	return "document"
}
