package main

import (
	"errors"
	"io"
	"testing"
)

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestTourReportsOutputFailureAfterCleanup(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"list", "text"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(t.Context(), []string{name}, failedWriter{}); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("output error lost: %v", err)
			}
		})
	}
}
