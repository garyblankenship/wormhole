package server

import (
	"context"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	wormhole "github.com/garyblankenship/wormhole/pkg/wormhole"
)

// Config holds server configuration.
type Config struct {
	Addr            string
	DefaultProvider string
	WormholeOpts    []wormhole.Option
	ProxyAPIKey     string
	Logger          *slog.Logger
}

type proxy struct {
	wh              *wormhole.Wormhole
	server          *http.Server
	logger          *slog.Logger
	apiKey          string
	defaultProvider string
}

// New creates and wires a new proxy server from the given config.
func New(cfg Config) *proxy {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}

	opts := make([]wormhole.Option, len(cfg.WormholeOpts))
	copy(opts, cfg.WormholeOpts)
	if cfg.DefaultProvider != "" {
		opts = append(opts, wormhole.WithDefaultProvider(cfg.DefaultProvider))
	}

	p := &proxy{
		wh:              wormhole.New(opts...),
		logger:          cfg.Logger,
		apiKey:          cfg.ProxyAPIKey,
		defaultProvider: cfg.DefaultProvider,
	}

	if p.apiKey == "" {
		p.logger.Warn("proxy authentication disabled: WORMHOLE_API_KEY not set; /v1/ endpoints are unauthenticated")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", p.handleChatCompletions)
	mux.HandleFunc("POST /v1/embeddings", p.handleEmbeddings)
	mux.HandleFunc("GET /v1/models", p.handleListModels)
	mux.HandleFunc("GET /health", p.handleHealth)

	p.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           p.auth(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is intentionally unset: the proxy serves long-lived SSE
		// streams (streamChat) that a global write deadline would truncate.
	}

	return p
}

// Start begins listening and serving. Blocks until error or shutdown.
func (p *proxy) Start() error {
	p.logger.Info("starting wormhole proxy", "addr", p.server.Addr)
	return p.server.ListenAndServe()
}

// Shutdown gracefully stops the HTTP server and the wormhole client.
func (p *proxy) Shutdown(ctx context.Context) error {
	serverErr := p.server.Shutdown(ctx)
	_ = p.wh.Shutdown(ctx)
	return serverErr
}

func (p *proxy) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.apiKey != "" && strings.HasPrefix(r.URL.Path, "/v1/") {
			auth := r.Header.Get("Authorization")
			token := strings.TrimPrefix(auth, "Bearer ")
			if token == auth || subtle.ConstantTimeCompare([]byte(token), []byte(p.apiKey)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid_api_key",
					"Invalid or missing API key", "authentication_error")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
