package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

type citationAccumulator struct {
	sources   []types.Source
	citations []types.Citation
	warnings  []types.ResponseWarning
	sourceIDs map[string]string
	seen      map[string]bool
}

func newCitationAccumulator() *citationAccumulator {
	return &citationAccumulator{sourceIDs: make(map[string]string), seen: make(map[string]bool)}
}

func (a *citationAccumulator) add(data json.RawMessage) {
	if len(data) == 0 || a.seen[string(data)] {
		return
	}
	a.seen[string(data)] = true
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Malformed Anthropic citation", Field: "citations"})
		a.sources = append(a.sources, types.Source{ID: fmt.Sprintf("source_%d", len(a.sources)+1), Kind: "unknown", Raw: map[string]any{"annotation": string(data)}})
		return
	}
	typ, _ := raw["type"].(string)
	key := ""
	if index, ok := raw["document_index"].(float64); ok {
		key = fmt.Sprintf("document:%v", index)
	}
	if id, ok := raw["encrypted_index"].(string); ok && id != "" {
		key = "web:" + id
	}
	if index, ok := raw["search_result_index"].(float64); ok {
		key = fmt.Sprintf("search:%v", index)
	}
	if key == "" {
		a.sources = append(a.sources, types.Source{ID: fmt.Sprintf("source_%d", len(a.sources)+1), Kind: "unknown", Raw: raw})
		a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Anthropic citation lacks a source identifier", Field: "citations"})
		return
	}
	id, ok := a.sourceIDs[key]
	if !ok {
		id = fmt.Sprintf("source_%d", len(a.sources)+1)
		a.sourceIDs[key] = id
		title, _ := raw["document_title"].(string)
		if title == "" {
			title, _ = raw["title"].(string)
		}
		url, _ := raw["url"].(string)
		native, _ := raw["encrypted_index"].(string)
		kind := "document"
		if url != "" {
			kind = "url"
		}
		a.sources = append(a.sources, types.Source{ID: id, ProviderID: native, Kind: kind, Title: title, URL: url, Raw: raw})
	}
	location := &types.SourceLocation{}
	switch typ {
	case "char_location":
		location.StartChar = citationInt(raw, "start_char_index")
		location.EndChar = citationInt(raw, "end_char_index")
	case "page_location":
		location.StartPage = citationInt(raw, "start_page_number")
		location.EndPage = citationInt(raw, "end_page_number")
	case "web_search_result_location", "search_result_location", "content_block_location":
		location = nil
	default:
		a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningUnsupportedNormalization, Message: "Unsupported Anthropic citation type", Field: "citations"})
		return
	}
	if (typ == "char_location" && (location.StartChar == nil || location.EndChar == nil)) || (typ == "page_location" && (location.StartPage == nil || location.EndPage == nil)) {
		a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Incomplete Anthropic source location", Field: "citations"})
		return
	}
	if location != nil && ((location.StartChar != nil && location.EndChar != nil && *location.EndChar < *location.StartChar) || (location.StartPage != nil && location.EndPage != nil && *location.EndPage < *location.StartPage)) {
		a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Invalid Anthropic source location", Field: "citations"})
		return
	}
	// Anthropic supplies source positions, never offsets into generated output.
	a.citations = append(a.citations, types.Citation{SourceID: id, SourceLocation: location, Raw: raw})
}

func citationInt(raw map[string]any, key string) *int {
	v, ok := raw[key].(float64)
	if !ok || v < 0 || v != float64(int(v)) {
		return nil
	}
	n := int(v)
	return &n
}
