package gemini

import "github.com/prism-php/prism-go/pkg/types"

// Gemini API response types
type geminiTextResponse struct {
	Candidates     []candidate     `json:"candidates"`
	UsageMetadata  *usageMetadata  `json:"usageMetadata,omitempty"`
	PromptFeedback *promptFeedback `json:"promptFeedback,omitempty"`
	Error          *geminiError    `json:"error,omitempty"`
}

type candidate struct {
	Content           content            `json:"content"`
	FinishReason      string             `json:"finishReason,omitempty"`
	SafetyRatings     []safetyRating     `json:"safetyRatings,omitempty"`
	CitationMetadata  *citationMetadata  `json:"citationMetadata,omitempty"`
	TokenCount        int                `json:"tokenCount,omitempty"`
	GroundingMetadata *groundingMetadata `json:"groundingMetadata,omitempty"`
}

type content struct {
	Parts []part `json:"parts"`
	Role  string `json:"role"`
}

type part struct {
	Text             string            `json:"text,omitempty"`
	InlineData       *inlineData       `json:"inlineData,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type inlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type functionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

type functionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
}

type usageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type safetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
	Blocked     bool   `json:"blocked,omitempty"`
}

type promptFeedback struct {
	BlockReason   string         `json:"blockReason,omitempty"`
	SafetyRatings []safetyRating `json:"safetyRatings,omitempty"`
}

type citationMetadata struct {
	Citations []citation `json:"citations"`
}

type citation struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	URI        string `json:"uri"`
	Title      string `json:"title,omitempty"`
	License    string `json:"license,omitempty"`
}

type groundingMetadata struct {
	WebSearchQueries      []string               `json:"webSearchQueries,omitempty"`
	SearchEntryPoint      *searchEntryPoint      `json:"searchEntryPoint,omitempty"`
	GroundingAttributions []groundingAttribution `json:"groundingAttributions,omitempty"`
}

type searchEntryPoint struct {
	RenderedContent string `json:"renderedContent"`
}

type groundingAttribution struct {
	SourceID        *sourceID        `json:"sourceId"`
	Content         string           `json:"content"`
	CitationSources []citationSource `json:"citationSources,omitempty"`
}

type sourceID struct {
	GroundingPassage       *groundingPassage       `json:"groundingPassage,omitempty"`
	SemanticRetrieverChunk *semanticRetrieverChunk `json:"semanticRetrieverChunk,omitempty"`
}

type groundingPassage struct {
	PassageID string `json:"passageId"`
	PartIndex int    `json:"partIndex"`
}

type semanticRetrieverChunk struct {
	Source string `json:"source"`
	Chunk  string `json:"chunk"`
}

type citationSource struct {
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	URI        string `json:"uri"`
	Title      string `json:"title,omitempty"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Embeddings response types
type geminiEmbeddingsResponse struct {
	Embeddings []embedding `json:"embeddings"`
}

type embedding struct {
	Values []float64 `json:"values"`
}

// Tool types
type geminiTool struct {
	FunctionDeclarations []functionDeclaration `json:"functionDeclarations"`
}

type functionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// Schema types for structured output
type geminiSchema struct {
	Type        string                 `json:"type"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Items       *geminiSchema          `json:"items,omitempty"`
	Enum        []interface{}          `json:"enum,omitempty"`
	Description string                 `json:"description,omitempty"`
}

// Finish reason mappings
var finishReasonMap = map[string]types.FinishReason{
	"STOP":                      types.FinishReasonStop,
	"MAX_TOKENS":                types.FinishReasonLength,
	"SAFETY":                    types.FinishReasonContentFilter,
	"RECITATION":                types.FinishReasonContentFilter,
	"OTHER":                     types.FinishReasonOther,
	"FINISH_REASON_UNSPECIFIED": types.FinishReasonOther,
}
