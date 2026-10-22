package types

// CloneTextResponse returns a detached response, including JSON-like metadata
// and tool arguments. Opaque values follow CloneValue's ownership conventions.
func CloneTextResponse(src *TextResponse) *TextResponse {
	if src == nil {
		return nil
	}
	dst := *src
	dst.ToolCalls = CloneToolCalls(src.ToolCalls)
	dst.Metadata = CloneMap(src.Metadata)
	dst.Thinking = cloneThinking(src.Thinking)
	dst.Usage = CloneUsage(src.Usage)
	dst.ResponseMetadata = CloneResponseMetadata(src.ResponseMetadata)
	dst.Sources = CloneSources(src.Sources)
	dst.Citations = CloneCitations(src.Citations)
	dst.Warnings = CloneWarnings(src.Warnings)
	return &dst
}

// CloneTextChunk returns a detached streaming chunk. Error identity is retained.
func CloneTextChunk(src TextChunk) TextChunk {
	dst := src
	dst.ToolCalls = CloneToolCalls(src.ToolCalls)
	if src.ToolCall != nil {
		call := CloneToolCall(*src.ToolCall)
		dst.ToolCall = &call
	}
	dst.Thinking = cloneThinking(src.Thinking)
	dst.Usage = CloneUsage(src.Usage)
	dst.Metadata = CloneMap(src.Metadata)
	dst.ResponseMetadata = CloneResponseMetadata(src.ResponseMetadata)
	dst.Sources = CloneSources(src.Sources)
	dst.Citations = CloneCitations(src.Citations)
	dst.Warnings = CloneWarnings(src.Warnings)
	if src.FinishReason != nil {
		reason := *src.FinishReason
		dst.FinishReason = &reason
	}
	if src.Delta != nil {
		delta := *src.Delta
		delta.Thinking = cloneThinking(src.Delta.Thinking)
		delta.ToolCalls = CloneToolCalls(src.Delta.ToolCalls)
		dst.Delta = &delta
	}
	return dst
}

func cloneThinking(src *Thinking) *Thinking {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}
