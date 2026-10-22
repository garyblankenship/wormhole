package server

import (
	"strconv"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v3/types"
)

func TestChatCompletionResponseIdentity(t *testing.T) {
	t.Parallel()
	now := time.Unix(2000, 123)
	cases := []struct {
		name        string
		response    types.TextResponse
		wantID      string
		wantCreated int64
	}{
		{name: "native fields absent", wantID: "wh-" + strconv.FormatInt(now.UnixNano(), 10), wantCreated: now.Unix()},
		{name: "native fields supplied", response: types.TextResponse{ID: "upstream-id", Created: time.Unix(1000, 0)}, wantID: "wh-upstream-id", wantCreated: 1000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := tc.response
			got := chatCompletionResponse(&input, "fixture-model", nil, now)
			if got.ID != tc.wantID || got.Created != tc.wantCreated {
				t.Fatalf("identity = %q / %d; want %q / %d", got.ID, got.Created, tc.wantID, tc.wantCreated)
			}
			if input.ID != tc.response.ID || !input.Created.Equal(tc.response.Created) {
				t.Fatal("conversion mutated the native response")
			}
		})
	}
}
