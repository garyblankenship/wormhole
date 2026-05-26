package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garyblankenship/wormhole/internal/server"
	"github.com/garyblankenship/wormhole/pkg/types"
	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) < 1 {
		printUsage(stdout)
		return 0
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:], stdout, stderr, getenv)
	case "version":
		_, _ = fmt.Fprintln(stdout, "wormhole v1.9.0")
	case "help", "--help", "-h":
		printUsage(stdout)
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, `wormhole - OpenAI-compatible LLM proxy

Commands:
  serve     Start the proxy server
  version   Print version
  help      Show this help

Run "wormhole serve --help" for serve options.`)
}

func runServe(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", ":8080", "Listen address")
	defaultProvider := fs.String("default-provider", "", "Default provider when model has no prefix")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 1
	}

	logger := slog.New(slog.NewJSONHandler(stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	var opts []wormhole.Option
	opts = append(opts, wormhole.WithAllProvidersFromEnv())

	// Ollama often has no API key, just a base URL
	if ollamaURL := getenv("OLLAMA_BASE_URL"); ollamaURL != "" {
		opts = append(opts, wormhole.WithOllama(types.ProviderConfig{
			BaseURL: ollamaURL,
		}))
	}

	cfg := server.Config{
		Addr:            *addr,
		DefaultProvider: *defaultProvider,
		WormholeOpts:    opts,
		ProxyAPIKey:     getenv("WORMHOLE_API_KEY"),
		Logger:          logger,
	}

	srv := server.New(cfg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Info("shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logger.Error("shutdown error", "error", err)
		}
	}()

	if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server error", "error", err)
		return 1
	}
	return 0
}
