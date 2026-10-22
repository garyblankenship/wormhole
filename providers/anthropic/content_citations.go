package anthropic

import (
	"bytes"
	"encoding/json"
)

// Optional citation shapes must not prevent decoding the generation itself.
func (c *contentPart) UnmarshalJSON(data []byte) error {
	type plain contentPart
	var decoded plain
	fields := struct {
		*plain
		Citations json.RawMessage `json:"citations"`
	}{plain: &decoded}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if raw := bytes.TrimSpace(fields.Citations); len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
		if err := json.Unmarshal(raw, &decoded.Citations); err != nil {
			// The normalizer retains malformed records and emits a warning.
			decoded.Citations = []json.RawMessage{append(json.RawMessage(nil), raw...)}
		}
	}
	*c = contentPart(decoded)
	return nil
}
