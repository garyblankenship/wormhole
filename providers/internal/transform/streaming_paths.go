package transform

import "strings"

// pathSegment is one precompiled step of a JSON extraction path: either a map
// key or an array index. Paths come from immutable StreamingConfig strings, so
// they are compiled once per transformer; per-chunk extraction then walks the
// segments without re-splitting path strings or allocating parse errors.
type pathSegment struct {
	key     string
	index   int
	isIndex bool
}

// streamingPaths holds the compiled form of every configured extraction path.
type streamingPaths struct {
	id                []pathSegment
	model             []pathSegment
	textField         []pathSegment
	toolCallField     []pathSegment
	thinking          []pathSegment
	refusal           []pathSegment
	finishReason      []pathSegment
	extraFinishReason []pathSegment
	usage             []pathSegment
}

// compileStreamingPaths compiles every configured path. Empty paths compile to
// a nil slice so extraction gating can rely on segment count alone.
func compileStreamingPaths(config StreamingConfig) streamingPaths {
	return streamingPaths{
		id:                compilePath(config.IDPath),
		model:             compilePath(config.ModelPath),
		textField:         compilePath(config.TextFieldPath),
		toolCallField:     compilePath(config.ToolCallFieldPath),
		thinking:          compilePath(config.ThinkingPath),
		refusal:           compilePath(config.RefusalPath),
		finishReason:      compilePath(config.FinishReasonPath),
		extraFinishReason: compilePath(config.ExtraFinishReasonPath),
		usage:             compilePath(config.UsagePath),
	}
}

// compilePath splits a dot-separated path ("choices.0.delta.content") into
// segments. An all-digit segment is an array index, mirroring the original
// runtime path parser (including the empty segment, which behaves as index 0).
func compilePath(path string) []pathSegment {
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	segments := make([]pathSegment, len(parts))
	for i, part := range parts {
		if idx, ok := parseArrayIndex(part); ok {
			segments[i] = pathSegment{index: idx, isIndex: true}
		} else {
			segments[i] = pathSegment{key: part}
		}
	}
	return segments
}

// parseArrayIndex reports whether s consists only of digits and its value.
// It is allocation-free and runs only at transformer construction.
func parseArrayIndex(s string) (int, bool) {
	result := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		result = result*10 + int(r-'0')
	}
	return result, true
}

// getField walks precompiled segments through decoded JSON data, returning nil
// as soon as a step misses. Semantics match the previous runtime path walker:
// index bounds require >= 0, and map misses or type mismatches terminate with
// nil.
func getField(current any, segments []pathSegment) any {
	for _, seg := range segments {
		if current == nil {
			return nil
		}
		if seg.isIndex {
			arr, ok := current.([]any)
			if !ok || seg.index < 0 || seg.index >= len(arr) {
				return nil
			}
			current = arr[seg.index]
		} else {
			m, ok := current.(map[string]any)
			if !ok {
				return nil
			}
			current = m[seg.key]
		}
	}
	return current
}
