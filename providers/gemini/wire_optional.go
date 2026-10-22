package gemini

import "encoding/json"

// Optional annotations never make an otherwise usable generation undecodable.
func (c *candidate) UnmarshalJSON(data []byte) error {
	type plain candidate
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	grounding, citations := fields["groundingMetadata"], fields["citationMetadata"]
	delete(fields, "groundingMetadata")
	delete(fields, "citationMetadata")
	body, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, (*plain)(c)); err != nil {
		return err
	}
	decode := func(raw json.RawMessage) map[string]any {
		if len(raw) == 0 || string(raw) == "null" {
			return nil
		}
		var result map[string]any
		if json.Unmarshal(raw, &result) == nil {
			return result
		}
		var original any
		_ = json.Unmarshal(raw, &original)
		return map[string]any{"malformed": original}
	}
	c.GroundingMetadata, c.CitationMetadata = decode(grounding), decode(citations)
	return nil
}
func (u *usageMetadata) UnmarshalJSON(data []byte) error {
	type plain usageMetadata
	if err := json.Unmarshal(data, (*plain)(u)); err != nil {
		return err
	}
	return json.Unmarshal(data, &u.Raw)
}
