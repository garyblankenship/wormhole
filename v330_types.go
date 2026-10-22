package wormhole

import (
	"github.com/garyblankenship/wormhole/v3/internal/structured"
	"github.com/garyblankenship/wormhole/v3/types"
)

// ResponseMetadata describes a filtered upstream response snapshot.
type ResponseMetadata = types.ResponseMetadata

// ResponseWarning describes a non-fatal provider normalization limitation.
type ResponseWarning = types.ResponseWarning

// Source is a response-scoped provider reference.
type Source = types.Source

// Citation links generated output to a source.
type Citation = types.Citation

// TextRange contains half-open UTF-8 byte offsets.
type TextRange = types.TextRange

// SourceLocation contains native source coordinates.
type SourceLocation = types.SourceLocation

// FileReferenceMedia references a file already stored with a provider.
type FileReferenceMedia = types.FileReferenceMedia

// ToolCallsRequiredError reports a structured application-tool turn.
type ToolCallsRequiredError = types.ToolCallsRequiredError

// StructuredResponseError retains a malformed or mixed structured provider response.
type StructuredResponseError = structured.ResponseError
