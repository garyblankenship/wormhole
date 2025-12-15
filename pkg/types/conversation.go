package types

// Conversation provides a fluent builder for multi-turn conversations.
// This simplifies the construction of message arrays for chat-like interactions.
//
// Example:
//
//	conv := types.NewConversation().
//	    System("You are a helpful coding assistant.").
//	    User("What is Go?").
//	    Assistant("Go is a statically typed, compiled programming language.").
//	    User("What makes it good for servers?")
//
//	response, _ := client.Text().Conversation(conv).Generate(ctx)
type Conversation struct {
	messages []Message
}

// NewConversation creates a new empty conversation.
func NewConversation() *Conversation {
	return &Conversation{
		messages: make([]Message, 0),
	}
}

// System adds a system message to the conversation.
// System messages set the behavior and context for the assistant.
// Typically placed at the beginning of a conversation.
func (c *Conversation) System(content string) *Conversation {
	c.messages = append(c.messages, NewSystemMessage(content))
	return c
}

// User adds a user message to the conversation.
func (c *Conversation) User(content string) *Conversation {
	c.messages = append(c.messages, NewUserMessage(content))
	return c
}

// Assistant adds an assistant message to the conversation.
// Useful for few-shot prompting or continuing from a previous response.
func (c *Conversation) Assistant(content string) *Conversation {
	c.messages = append(c.messages, NewAssistantMessage(content))
	return c
}

// Add appends a raw Message to the conversation.
// Use this when you need to add messages with additional fields (e.g., tool calls).
func (c *Conversation) Add(msg Message) *Conversation {
	c.messages = append(c.messages, msg)
	return c
}

// AddAll appends multiple messages to the conversation.
func (c *Conversation) AddAll(msgs ...Message) *Conversation {
	c.messages = append(c.messages, msgs...)
	return c
}

// Messages returns the underlying message slice.
// This is useful for passing to APIs that expect []Message.
func (c *Conversation) Messages() []Message {
	return c.messages
}

// Len returns the number of messages in the conversation.
func (c *Conversation) Len() int {
	return len(c.messages)
}

// IsEmpty returns true if the conversation has no messages.
func (c *Conversation) IsEmpty() bool {
	return len(c.messages) == 0
}

// Clear removes all messages from the conversation.
func (c *Conversation) Clear() *Conversation {
	c.messages = make([]Message, 0)
	return c
}

// Clone creates a deep copy of the conversation.
// This is useful for branching conversations or testing variations.
func (c *Conversation) Clone() *Conversation {
	cloned := &Conversation{
		messages: make([]Message, len(c.messages)),
	}
	copy(cloned.messages, c.messages)
	return cloned
}

// Last returns the last message in the conversation, or nil if empty.
func (c *Conversation) Last() Message {
	if len(c.messages) == 0 {
		return nil
	}
	return c.messages[len(c.messages)-1]
}

// FirstUserMessage returns the first user message, or nil if none.
func (c *Conversation) FirstUserMessage() Message {
	for _, msg := range c.messages {
		if msg.GetRole() == RoleUser {
			return msg
		}
	}
	return nil
}

// SystemMessage returns the first system message, or nil if none.
func (c *Conversation) SystemMessage() Message {
	for _, msg := range c.messages {
		if msg.GetRole() == RoleSystem {
			return msg
		}
	}
	return nil
}

// WithoutSystem returns a new conversation with system messages removed.
// Useful when the provider doesn't support system messages natively.
func (c *Conversation) WithoutSystem() *Conversation {
	filtered := NewConversation()
	for _, msg := range c.messages {
		if msg.GetRole() != RoleSystem {
			filtered.messages = append(filtered.messages, msg)
		}
	}
	return filtered
}

// FromMessages creates a Conversation from an existing message slice.
func FromMessages(msgs []Message) *Conversation {
	return &Conversation{
		messages: msgs,
	}
}

// FewShot is a convenience constructor for few-shot prompting.
// It creates a conversation with examples of user/assistant exchanges.
//
// Example:
//
//	conv := types.FewShot(
//	    "You are a translator.",
//	    []types.ExamplePair{
//	        {User: "Hello", Assistant: "Hola"},
//	        {User: "Goodbye", Assistant: "Adiós"},
//	    },
//	).User("How are you?")
func FewShot(systemPrompt string, examples []ExamplePair) *Conversation {
	c := NewConversation()
	if systemPrompt != "" {
		c.System(systemPrompt)
	}
	for _, ex := range examples {
		c.User(ex.User).Assistant(ex.Assistant)
	}
	return c
}

// ExamplePair represents a user/assistant exchange for few-shot prompting.
type ExamplePair struct {
	User      string
	Assistant string
}
