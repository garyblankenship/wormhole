package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net"
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
		// Default to loopback: an unauthenticated proxy bound to all interfaces
		// would let anyone on the network spend the operator's provider credits.
		cfg.Addr = "127.0.0.1:8080"
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
	// Fail closed: never expose an unauthenticated proxy on a non-loopback
	// interface. Anyone who could reach it would spend the operator's credits.
	if p.apiKey == "" && !isLoopbackAddr(p.server.Addr) {
		return fmt.Errorf("refusing to bind %q without authentication: set WORMHOLE_API_KEY, or bind to localhost", p.server.Addr)
	}
	p.logger.Info("starting wormhole proxy", "addr", p.server.Addr)
	return p.server.ListenAndServe()
}

// isLoopbackAddr reports whether addr binds only the loopback interface.
// An empty host (e.g. ":8080") binds all interfaces and is NOT loopback.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
