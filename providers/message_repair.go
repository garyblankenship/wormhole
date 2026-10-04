package providers

import (
	"github.com/garyblankenship/wormhole/v3/types"
)

// ValidateMessageSequence runs the same orphan/stranded detection as
// PrepareMessages but returns the defects as warning strings rather than
// repairing them. Use it to surface sequence problems (for logging or metrics)
// before — or instead of — calling PrepareMessages. It does not mutate the input slice.
//
// The returned warnings use the same wording PrepareMessages emits for the
// corresponding repairs. The error return mirrors PrepareMessages' hard
// constraint: a duplicate normalized tool-call ID within one assistant message.
func ValidateMessageSequence(messages []types.Message) ([]string, error) {
	// Use precisely the same occurrence matching and normalization on detached
	// copies so validation reports the repairs without changing caller history.
	_, warnings, err := PrepareMessages(messages)
	return warnings, err
}
