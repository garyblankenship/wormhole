package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/middleware"
)

func middlewareExample(ctx context.Context, j *journey) error {
	cache := middleware.NewMemoryCache(j.cfg.Settings.CacheCapacity)
	metrics := middleware.NewTypedMetrics()
	enhanced := middleware.NewEnhancedMetricsCollector(middleware.DefaultEnhancedMetricsConfig())
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	client := j.client(wormhole.WithProviderMiddleware(
		middleware.NewTypedCacheMiddleware(middleware.CacheConfig{Cache: cache, TTL: j.ttl()}),
		middleware.NewTypedTimeoutMiddleware(time.Duration(j.cfg.Settings.TimeoutSeconds)*time.Second),
		middleware.NewTypedRateLimitMiddleware(j.cfg.Settings.RatePerSecond),
		middleware.NewTypedCircuitBreakerMiddleware(j.cfg.Settings.Retries, j.ttl()),
		middleware.NewTypedMetricsMiddleware(metrics),
		middleware.NewTypedEnhancedMetricsMiddleware(enhanced),
		middleware.NewDebugTypedLoggingMiddleware(logger),
	))
	first, err := j.text(client).Generate(ctx)
	if err = j.expectText(first, err); err != nil {
		return err
	}
	first.Text = j.cfg.Prompt
	second, err := j.text(client).Generate(ctx)
	if err = j.expectText(second, err); err != nil {
		return err
	}
	n, failures, _ := metrics.GetTextStats()
	if n != 1 || failures != 0 || len(j.fixture.all()) != 1 {
		return errors.New("middleware cache/metrics mismatch")
	}
	if !strings.Contains(logs.String(), j.cfg.Models["text"]) || !strings.Contains(enhanced.PrometheusExporter(), "wormhole_") {
		return errors.New("logging/metric exports absent")
	}
	metrics.Reset()
	n, _, _ = metrics.GetTextStats()
	return require(n == 0 && enhanced.JSONExporter() != nil, "metrics reset/export failed")
}
func circuitExample(ctx context.Context, j *journey) error {
	client := j.client(wormhole.WithProviderMiddleware(middleware.NewTypedCircuitBreakerMiddleware(j.cfg.Settings.Retries, j.ttl())))
	_, err := client.Text().Model(j.cfg.Models["fail"]).Prompt(j.cfg.Prompt).Generate(ctx)
	if err == nil {
		return errors.New("fixture failure succeeded")
	}
	_, err = j.text(client).Generate(ctx)
	if err == nil || len(j.fixture.all()) != 1 {
		return errors.New("open circuit forwarded request")
	}
	resp, err := j.text(client).Using("backup").Generate(ctx)
	if err = j.expectText(resp, err); err != nil {
		return err
	}
	emb, err := client.Embeddings().Model(j.cfg.Models["embedding"]).Input(j.cfg.Inputs[0]).Generate(ctx)
	if err != nil {
		return err
	}
	return require(equalJSON(emb.Content(), j.cfg.Vectors[0]), "text circuit blocked healthy embedding operation")
}
