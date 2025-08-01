package types

// Schema represents a structured output schema interface or raw JSON bytes
type Schema interface{}

// SchemaInterface represents the original schema interface
type SchemaInterface interface {
	GetType() string
	GetDescription() string
	Validate(data interface{}) error
}

// BaseSchema provides common schema functionality
type BaseSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func (s *BaseSchema) GetType() string {
	return s.Type
}

func (s *BaseSchema) GetDescription() string {
	return s.Description
}

// ObjectSchema represents an object schema
type ObjectSchema struct {
	BaseSchema
	Properties map[string]SchemaInterface `json:"properties"`
	Required   []string                   `json:"required,omitempty"`
}

func (s *ObjectSchema) Validate(data interface{}) error {
	// TODO: Implement validation
	return nil
}

// ArraySchema represents an array schema
type ArraySchema struct {
	BaseSchema
	Items SchemaInterface `json:"items"`
}

func (s *ArraySchema) Validate(data interface{}) error {
	// TODO: Implement validation
	return nil
}

// StringSchema represents a string schema
type StringSchema struct {
	BaseSchema
	MinLength *int   `json:"minLength,omitempty"`
	MaxLength *int   `json:"maxLength,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
}

func (s *StringSchema) Validate(data interface{}) error {
	// TODO: Implement validation
	return nil
}

// NumberSchema represents a number schema
type NumberSchema struct {
	BaseSchema
	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`
}

func (s *NumberSchema) Validate(data interface{}) error {
	// TODO: Implement validation
	return nil
}

// BooleanSchema represents a boolean schema
type BooleanSchema struct {
	BaseSchema
}

func (s *BooleanSchema) Validate(data interface{}) error {
	// TODO: Implement validation
	return nil
}

// EnumSchema represents an enum schema
type EnumSchema struct {
	BaseSchema
	Enum []interface{} `json:"enum"`
}

func (s *EnumSchema) Validate(data interface{}) error {
	// TODO: Implement validation
	return nil
}
