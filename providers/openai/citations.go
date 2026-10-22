package openai

import (
	"encoding/json"
	"fmt"

	"github.com/garyblankenship/wormhole/v3/types"
)

// State belongs to one response, never the provider shared by concurrent calls.
type responsesAnnotationState struct {
	sources map[string]string
	seen    map[string]bool
	// A terminal snapshot reuses IDs while including every source once.
	snapshotSources map[string]bool
}

func newResponsesAnnotationState() *responsesAnnotationState {
	return &responsesAnnotationState{sources: map[string]string{}, seen: map[string]bool{}}
}

// normalizeResponse retains native annotation offsets: Responses offsets have
// no guaranteed Unicode unit.
func (s *responsesAnnotationState) normalizeResponse(response *responsesResponse, result *types.TextResponse) {
	for outputIndex, item := range response.Output {
		for contentIndex, part := range item.Content {
			if part.Annotations == nil {
				continue
			}
			// Decode optional data independently so malformed annotations cannot
			// reject otherwise usable text or tool output.
			raw, _ := json.Marshal(part.Annotations)
			var annotations []any
			if err := json.Unmarshal(raw, &annotations); err != nil {
				s.warn(part.Annotations, result, "malformed_citation")
				continue
			}
			for index, annotation := range annotations {
				s.normalize(annotation, outputIndex, contentIndex, index, result)
			}
		}
	}
}

func (s *responsesAnnotationState) warn(raw any, result *types.TextResponse, code string) {
	result.Warnings = append(result.Warnings, types.ResponseWarning{Code: code, Message: "provider annotation could not be normalized", Field: "annotations"})
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	values, _ := result.Metadata["unparsed_annotations"].([]any)
	result.Metadata["unparsed_annotations"] = append(values, raw)
}

func (s *responsesAnnotationState) normalize(value any, outputIndex, contentIndex, annotationIndex int, result *types.TextResponse) {
	raw, _ := json.Marshal(value)
	// Annotation indices are local to a content part. Include all native
	// coordinates and payload so reused indices cannot alias distinct sources.
	identity := fmt.Sprintf("%d:%d:%d:%s", outputIndex, contentIndex, annotationIndex, raw)
	if s.seen[identity] {
		return
	}
	s.seen[identity] = true
	annotation, ok := value.(map[string]any)
	if !ok {
		s.warn(value, result, "malformed_citation")
		return
	}
	kind, _ := annotation["type"].(string)
	nativeID, _ := annotation["file_id"].(string)
	url, _ := annotation["url"].(string)
	title, _ := annotation["title"].(string)
	if kind == "file_citation" && title == "" {
		title, _ = annotation["filename"].(string)
	}
	if kind == "" || (kind == "url_citation" && url == "") || (kind == "file_citation" && nativeID == "") {
		s.warn(value, result, "malformed_citation")
		return
	}
	if kind != "url_citation" && kind != "file_citation" {
		s.warn(value, result, "unsupported_normalization")
		return
	}
	key := identity
	if nativeID != "" {
		key = kind + ":" + nativeID
	}
	id, exists := s.sources[key]
	if !exists {
		id = fmt.Sprintf("source_%d", len(s.sources)+1)
		s.sources[key] = id
	}
	if !exists || (s.snapshotSources != nil && !s.snapshotSources[id]) {
		if s.snapshotSources != nil {
			s.snapshotSources[id] = true
		}
		result.Sources = append(result.Sources, types.Source{ID: id, ProviderID: nativeID, Kind: kind, URL: url, Title: title, Raw: annotation})
	}
	result.Citations = append(result.Citations, types.Citation{SourceID: id, Raw: annotation})
}
