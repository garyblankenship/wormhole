package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

func TestTourScenarios(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("loopback integration examples")
	}
	for _, s := range tourScenarios() {
		t.Run(s.name, func(t *testing.T) {
			t.Parallel()
			if s.name == "factories-env" && os.Getenv("WORMHOLE_SDK_TOUR_BIN") == "" {
				t.Skip("set WORMHOLE_SDK_TOUR_BIN to the built SDK tour for child-process integration")
			}
			if s.name == "proxy" && os.Getenv("WORMHOLE_PROXY_BIN") == "" {
				t.Skip("set WORMHOLE_PROXY_BIN to the built project CLI for process integration")
			}
			if err := runScenario(t.Context(), s); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTourCommand(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(t.Context(), []string{"list"}, &out); err != nil {
		t.Fatal(err)
	}
	names := strings.Fields(out.String())
	if len(names) != len(tourScenarios()) {
		t.Fatal("list omitted a scenario")
	}
	if err := run(t.Context(), []string{"unknown"}, &out); err == nil {
		t.Fatal("unknown scenario succeeded")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := run(ctx, []string{"text"}, &out); err == nil {
		t.Fatal("canceled command succeeded")
	}
}
