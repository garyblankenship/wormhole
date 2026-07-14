package anthropic

import (
	"encoding/json"
	"fmt"
)

func lenientUnmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

func unmarshalToolArgs(args string, value any) error {
	if args == "" {
		return fmt.Errorf("empty tool arguments")
	}
	if err := json.Unmarshal([]byte(args), value); err != nil {
		return fmt.Errorf("failed to parse Anthropic tool arguments (may contain regex or escaped patterns): %w", err)
	}
	return nil
}
