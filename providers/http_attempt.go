package providers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

// observeHTTPAttempt surrounds exactly one underlying Do. Completion is emitted
// as soon as Do returns, before retry classification, response-body consumption,
// or any backoff. Callback execution is synchronous and intentionally not recovered.
func (r *retryableHTTPClient) observeHTTPAttempt(req *http.Request, attempt int) (*http.Response, error) {
	callback := r.OnHTTPAttempt
	if callback == nil {
		return r.Client.Do(req)
	}
	ordinal := attempt + 1
	callback(types.HTTPAttemptEvent{Phase: types.HTTPAttemptStart, Attempt: ordinal})
	start := time.Now()
	resp, err := r.Client.Do(req)
	event := types.HTTPAttemptEvent{
		Phase:    types.HTTPAttemptComplete,
		Attempt:  ordinal,
		Duration: time.Since(start),
		Outcome:  types.HTTPAttemptResponse,
	}
	if resp != nil {
		event.StatusCode = resp.StatusCode
	}
	if err != nil {
		event.Outcome = types.HTTPAttemptTransportError
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			event.Outcome = types.HTTPAttemptCanceled
		}
	}
	callback(event)
	return resp, err
}
