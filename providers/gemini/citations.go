package gemini

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"github.com/garyblankenship/wormhole/v3/types"
)

// citationSet is scoped to one response, including all frames of a stream.
type citationSet struct {
	sources      []types.Source
	citations    []types.Citation
	warnings     []types.ResponseWarning
	sourceKeys   map[string]string
	citationKeys map[string]bool
}

func (s *citationSet) source(raw map[string]any) string {
	encoded, _ := json.Marshal(raw)
	key := string(encoded)
	if s.sourceKeys == nil {
		s.sourceKeys = map[string]string{}
	}
	if id := s.sourceKeys[key]; id != "" {
		return id
	}
	id := fmt.Sprintf("source-%d", len(s.sources)+1)
	detail := raw
	kind := "document"
	if web, ok := raw["web"].(map[string]any); ok {
		detail = web
		kind = "web"
	}
	if retrieved, ok := raw["retrievedContext"].(map[string]any); ok {
		detail = retrieved
	}
	url, _ := detail["uri"].(string)
	title, _ := detail["title"].(string)
	native, _ := detail["id"].(string)
	s.sources = append(s.sources, types.Source{ID: id, ProviderID: native, Kind: kind, URL: url, Title: title, Raw: raw})
	s.sourceKeys[key] = id
	return id
}
func (s *citationSet) malformed(field string) {
	s.warnings = append(s.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Malformed optional Gemini citation data was retained", Field: field})
}
func (s *citationSet) add(c types.Citation) {
	encoded, _ := json.Marshal(c)
	if s.citationKeys == nil {
		s.citationKeys = map[string]bool{}
	}
	if !s.citationKeys[string(encoded)] {
		s.citations = append(s.citations, c)
		s.citationKeys[string(encoded)] = true
	}
}
func integer(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), n == float64(int(n))
	case int:
		return n, true
	}
	return 0, false
}
func (s *citationSet) consume(c candidate, text string) {
	grounding := c.GroundingMetadata
	if grounding["malformed"] != nil {
		s.malformed("groundingMetadata")
	}
	if c.CitationMetadata["malformed"] != nil {
		s.malformed("citationMetadata")
	}
	ids := s.consumeGroundingChunks(grounding)
	s.consumeGroundingSupports(grounding, ids, text)
	s.consumeNativeCitations(c.CitationMetadata)
}

// groundingChunkSupported reports whether a grounding chunk carries a source
// form this adapter normalizes.
func groundingChunkSupported(raw map[string]any) bool {
	if _, web := raw["web"].(map[string]any); web {
		return true
	}
	_, retrieved := raw["retrievedContext"].(map[string]any)
	return retrieved
}

func (s *citationSet) consumeGroundingChunks(grounding map[string]any) []string {
	chunks, chunksOK := grounding["groundingChunks"].([]any)
	if grounding["groundingChunks"] != nil && !chunksOK {
		s.malformed("groundingChunks")
	}
	ids := make([]string, len(chunks))
	for i, value := range chunks {
		raw, ok := value.(map[string]any)
		if !ok {
			s.malformed("groundingChunks")
			continue
		}
		if !groundingChunkSupported(raw) {
			s.warnings = append(s.warnings, types.ResponseWarning{Code: types.WarningUnsupportedNormalization, Message: "Unsupported Gemini grounding source data was retained", Field: "groundingChunks"})
			continue
		}
		ids[i] = s.source(raw)
	}
	return ids
}

func (s *citationSet) consumeGroundingSupports(grounding map[string]any, ids []string, text string) {
	supports, supportsOK := grounding["groundingSupports"].([]any)
	if grounding["groundingSupports"] != nil && !supportsOK {
		s.malformed("groundingSupports")
	}
	for _, value := range supports {
		raw, ok := value.(map[string]any)
		if !ok {
			s.malformed("groundingSupports")
			continue
		}
		s.consumeGroundingSupport(raw, ids, text)
	}
}

func (s *citationSet) consumeGroundingSupport(raw map[string]any, ids []string, text string) {
	segment, segmentOK := raw["segment"].(map[string]any)
	if raw["segment"] != nil && !segmentOK {
		s.malformed("groundingSupports.segment")
	}
	output := s.segmentOutputRange(segment, text)
	indexes, indexesOK := raw["groundingChunkIndices"].([]any)
	if !indexesOK {
		s.malformed("groundingChunkIndices")
	}
	for _, value := range indexes {
		index, valid := integer(value)
		if !valid || index < 0 || index >= len(ids) || ids[index] == "" {
			s.malformed("groundingChunkIndices")
			continue
		}
		s.add(types.Citation{SourceID: ids[index], OutputRange: output, Raw: raw})
	}
}

// segmentOutputRange validates UTF-8 byte offsets against the response text.
func (s *citationSet) segmentOutputRange(segment map[string]any, text string) *types.TextRange {
	start, hasStart := integer(segment["startIndex"])
	end, hasEnd := integer(segment["endIndex"])
	// Gemini grounding segment offsets are UTF-8 byte offsets. Omitted startIndex is zero.
	if !hasStart {
		start = 0
	}
	if hasEnd && start >= 0 && end >= start && end <= len(text) && utf8.ValidString(text[:start]) && utf8.ValidString(text[:end]) {
		return &types.TextRange{Start: start, End: end}
	}
	if hasEnd || hasStart {
		s.malformed("groundingSupports.segment")
	}
	return nil
}

func (s *citationSet) consumeNativeCitations(metadata map[string]any) {
	native, nativeOK := metadata["citations"].([]any)
	if metadata["citations"] != nil && !nativeOK {
		s.malformed("citationMetadata.citations")
	}
	// Older generateContent responses use citationSources instead.
	if native == nil {
		native, _ = metadata["citationSources"].([]any)
	}
	for _, value := range native {
		raw, ok := value.(map[string]any)
		if !ok {
			s.malformed("citationMetadata")
			continue
		}
		uri, _ := raw["uri"].(string)
		if uri == "" {
			s.malformed("citationMetadata.uri")
			continue
		}
		id := s.source(raw)
		// The citation metadata index unit is not guaranteed; preserve native offsets only.
		s.add(types.Citation{SourceID: id, Raw: raw})
		if raw["startIndex"] != nil || raw["endIndex"] != nil {
			s.warnings = append(s.warnings, types.ResponseWarning{Code: types.WarningUnsupportedNormalization, Message: "Gemini citation metadata offsets have no guaranteed unit", Field: "citationMetadata"})
		}
	}
}
