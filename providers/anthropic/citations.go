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

// citationProblem describes a citation frame that cannot be normalized.
type citationProblem struct {
	code    string
	message string
}

func (a *citationAccumulator) add(data json.RawMessage) {
	if len(data) == 0 || a.seen[string(data)] {
		return
	}
	a.seen[string(data)] = true
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		a.warnUnparsedSource(data)
		return
	}
	key := anthropicSourceKey(raw)
	if key == "" {
		a.unidentifiedSource(raw)
		return
	}
	id := a.ensureSource(raw, key)
	location, problem := anthropicCitationLocation(raw)
	if problem.code != "" {
		a.warnings = append(a.warnings, types.ResponseWarning{Code: problem.code, Message: problem.message, Field: "citations"})
		return
	}
	a.citations = append(a.citations, types.Citation{SourceID: id, SourceLocation: location, Raw: raw})
}

// anthropicSourceKey derives the stable source identity for a citation frame,
// preserving wire precedence: search-result, encrypted web, then document.
func anthropicSourceKey(raw map[string]any) string {
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
	return key
}

func (a *citationAccumulator) nextSourceID() string {
	return fmt.Sprintf("source_%d", len(a.sources)+1)
}

func (a *citationAccumulator) warnUnparsedSource(data json.RawMessage) {
	a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Malformed Anthropic citation", Field: "citations"})
	a.sources = append(a.sources, types.Source{ID: a.nextSourceID(), Kind: "unknown", Raw: map[string]any{"annotation": string(data)}})
}

func (a *citationAccumulator) unidentifiedSource(raw map[string]any) {
	a.sources = append(a.sources, types.Source{ID: a.nextSourceID(), Kind: "unknown", Raw: raw})
	a.warnings = append(a.warnings, types.ResponseWarning{Code: types.WarningMalformedCitation, Message: "Anthropic citation lacks a source identifier", Field: "citations"})
}

// ensureSource registers the citation's source once and returns its stable ID.
func (a *citationAccumulator) ensureSource(raw map[string]any, key string) string {
	if id, ok := a.sourceIDs[key]; ok {
		return id
	}
	id := a.nextSourceID()
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
	return id
}

// anthropicCitationLocation normalizes supported location types and reports
// the first applicable warning problem in place of a location.
func anthropicCitationLocation(raw map[string]any) (*types.SourceLocation, citationProblem) {
	typ, _ := raw["type"].(string)
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
		return nil, citationProblem{types.WarningUnsupportedNormalization, "Unsupported Anthropic citation type"}
	}
	if (typ == "char_location" && (location.StartChar == nil || location.EndChar == nil)) || (typ == "page_location" && (location.StartPage == nil || location.EndPage == nil)) {
		return nil, citationProblem{types.WarningMalformedCitation, "Incomplete Anthropic source location"}
	}
	if location != nil && ((location.StartChar != nil && location.EndChar != nil && *location.EndChar < *location.StartChar) || (location.StartPage != nil && location.EndPage != nil && *location.EndPage < *location.StartPage)) {
		return nil, citationProblem{types.WarningMalformedCitation, "Invalid Anthropic source location"}
	}
	// Anthropic supplies source positions, never offsets into generated output.
	return location, citationProblem{}
}

func citationInt(raw map[string]any, key string) *int {
	v, ok := raw[key].(float64)
	if !ok || v < 0 || v != float64(int(v)) {
		return nil
	}
	n := int(v)
	return &n
}
