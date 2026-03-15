package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/garyblankenship/wormhole/internal/server"
	"github.com/garyblankenship/wormhole/pkg/types"
	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "version":
		fmt.Println("wormhole v0.1.0")
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`wormhole - OpenAI-compatible LLM proxy

Commands:
  serve     Start the proxy server
  version   Print version
  help      Show this help

Run "wormhole serve --help" for serve options.`)
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "Listen address")
	defaultProvider := fs.String("default-provider", "", "Default provider when model has no prefix")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	var opts []wormhole.Option
	opts = append(opts, wormhole.WithAllProvidersFromEnv())

	// Ollama often has no API key, just a base URL
	if ollamaURL := os.Getenv("OLLAMA_BASE_URL"); ollamaURL != "" {
		opts = append(opts, wormhole.WithOllama(types.ProviderConfig{
			BaseURL: ollamaURL,
		}))
	}

	cfg := server.Config{
		Addr:            *addr,
		DefaultProvider: *defaultProvider,
		WormholeOpts:    opts,
		ProxyAPIKey:     os.Getenv("WORMHOLE_API_KEY"),
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

	if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}
