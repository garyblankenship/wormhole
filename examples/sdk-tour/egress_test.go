package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFixtureRejectsExternalProxyDestination(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodConnect} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			fixture := &upstream{}
			req := httptest.NewRequest(method, "https://provider.invalid/v1/chat/completions", nil)
			out := httptest.NewRecorder()
			fixture.serve(out, req)
			if out.Code != http.StatusBadRequest || fixture.errors() == nil || len(fixture.all()) != 0 {
				t.Fatal("non-loopback request escaped egress guard")
			}
		})
	}
}
