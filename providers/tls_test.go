package providers

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/v2/config"
	"github.com/garyblankenship/wormhole/v2/types"
)

func TestNewSecureHTTPClient(t *testing.T) {
	t.Parallel()
	// Test 1: Default secure client
	client := NewSecureHTTPClient(30*time.Second, nil, nil, "")
	if client == nil {
		t.Fatal("NewSecureHTTPClient returned nil")
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", client.Timeout)
	}

	// Test 2: Client with custom TLS config
	tlsConfig := config.DefaultTLSConfig().WithMinVersion(tls.VersionTLS13)
	client2 := NewSecureHTTPClient(60*time.Second, &tlsConfig, nil, "")
	if client2 == nil {
		t.Fatal("NewSecureHTTPClient with custom TLS returned nil")
	}

	// Test 3: Insecure client
	client3 := NewInsecureHTTPClient(30*time.Second, false)
	if client3 == nil {
		t.Fatal("NewInsecureHTTPClient returned nil")
	}

	// Test 4: Strict client
	client4 := NewStrictHTTPClient(30 * time.Second)
	if client4 == nil {
		t.Fatal("NewStrictHTTPClient returned nil")
	}
}

func TestHTTPTransportConfig(t *testing.T) {
	t.Parallel()
	config := DefaultHTTPTransportConfig()

	// Validate default values
	if config.MaxIdleConns != 100 {
		t.Errorf("Expected MaxIdleConns 100, got %d", config.MaxIdleConns)
	}
	if config.MaxIdleConnsPerHost != 10 {
		t.Errorf("Expected MaxIdleConnsPerHost 10, got %d", config.MaxIdleConnsPerHost)
	}
	if config.IdleConnTimeout != 90*time.Second {
		t.Errorf("Expected IdleConnTimeout 90s, got %v", config.IdleConnTimeout)
	}
	if config.TLSConfig == nil {
		t.Fatal("TLSConfig should not be nil")
	}

	// Test validation
	if err := config.Validate(); err != nil {
		t.Errorf("Default config validation failed: %v", err)
	}

	// Test invalid config
	invalidConfig := config.WithConnectionPooling(-1, -1, -1, -1*time.Second)
	if err := invalidConfig.Validate(); err == nil {
		t.Error("Expected validation error for invalid config")
	}
}

func TestExtractTLSConfigFromProviderConfig(t *testing.T) {
	t.Parallel()
	// Test 1: ProviderConfig without TLS config
	config1 := types.NewProviderConfig("test-key")
	tlsConfig1 := ExtractTLSConfigFromProviderConfig(config1)
	if tlsConfig1 != nil {
		t.Error("Expected nil TLS config for ProviderConfig without TLS params")
	}

	// Test 2: ProviderConfig with TLS config
	config2 := types.NewProviderConfig("test-key").
		WithTLSConfigParam("min_version", uint16(tls.VersionTLS12)).
		WithTLSConfigParam("insecure_skip_verify", true)
	tlsConfig2 := ExtractTLSConfigFromProviderConfig(config2)
	if tlsConfig2 == nil {
		t.Fatal("Expected TLS config for ProviderConfig with TLS params")
	}
	if tlsConfig2.MinVersion != tls.VersionTLS12 {
		t.Errorf("Expected MinVersion TLS 1.2, got %d", tlsConfig2.MinVersion)
	}
	if !tlsConfig2.InsecureSkipVerify {
		t.Error("Expected InsecureSkipVerify true")
	}
	if tlsConfig2.AllowInsecure {
		t.Error("Raw TLS params should not implicitly allow insecure TLS")
	}

	// Test 3: ProviderConfig with complex TLS config
	config3 := types.NewProviderConfig("test-key").
		WithTLSConfigParam("min_version", uint16(tls.VersionTLS13)).
		WithTLSConfigParam("server_name", "example.com").
		WithTLSConfigParam("handshake_timeout", float64(5)) // 5 seconds
	tlsConfig3 := ExtractTLSConfigFromProviderConfig(config3)
	if tlsConfig3 == nil {
		t.Fatal("Expected TLS config for complex ProviderConfig")
	}
	if tlsConfig3.MinVersion != tls.VersionTLS13 {
		t.Errorf("Expected MinVersion TLS 1.3, got %d", tlsConfig3.MinVersion)
	}
	if tlsConfig3.ServerName != "example.com" {
		t.Errorf("Expected ServerName 'example.com', got %s", tlsConfig3.ServerName)
	}
	if tlsConfig3.HandshakeTimeout != 5*time.Second {
		t.Errorf("Expected HandshakeTimeout 5s, got %v", tlsConfig3.HandshakeTimeout)
	}
}

func TestBaseProviderTLSIntegration(t *testing.T) {
	t.Parallel()
	// Test 1: Default BaseProvider (should use secure TLS)
	providerConfig := types.NewProviderConfig("test-key")
	bp := NewBaseProvider("test-provider", providerConfig)
	if bp == nil {
		t.Fatal("NewBaseProvider returned nil")
	}

	client := bp.GetHTTPClient()
	if client == nil {
		t.Fatal("GetHTTPClient returned nil")
	}

	// Test 2: BaseProvider with custom TLS config
	tlsConfig := config.DefaultTLSConfig().WithMinVersion(tls.VersionTLS13)
	bp2 := NewBaseProviderWithTLS("test-provider", providerConfig, &tlsConfig)
	if bp2 == nil {
		t.Fatal("NewBaseProviderWithTLS returned nil")
	}

	// Test 3: Insecure BaseProvider
	bp3 := NewInsecureBaseProvider("test-provider", providerConfig, true)
	if bp3 == nil {
		t.Fatal("NewInsecureBaseProvider returned nil")
	}

	// Verify insecure config
	if bp3.tlsConfig == nil {
		t.Fatal("Insecure BaseProvider should have TLS config")
	}
	if !bp3.tlsConfig.InsecureSkipVerify {
		t.Error("Insecure BaseProvider should have InsecureSkipVerify true")
	}
}

func TestProviderConfigTLSBuilderMethods(t *testing.T) {
	t.Parallel()
	// Test WithTLSConfigParam
	config := types.NewProviderConfig("test-key").
		WithTLSConfigParam("min_version", uint16(tls.VersionTLS12)).
		WithTLSConfigParam("cipher_suites", []uint16{tls.TLS_AES_128_GCM_SHA256})

	if !config.HasTLSConfig() {
		t.Error("Expected HasTLSConfig to return true")
	}

	// Test WithInsecureTLS
	insecureConfig := types.NewProviderConfig("test-key").WithInsecureTLS(true)
	if !insecureConfig.HasTLSConfig() {
		t.Error("WithInsecureTLS should add TLS config")
	}

	// Extract and verify
	tlsConfig := ExtractTLSConfigFromProviderConfig(insecureConfig)
	if tlsConfig == nil {
		t.Fatal("Expected TLS config from insecure config")
	}
	if tlsConfig.MinVersion != tls.VersionTLS10 {
		t.Errorf("Expected MinVersion TLS 1.0 for insecure config, got %d", tlsConfig.MinVersion)
	}
	if !tlsConfig.InsecureSkipVerify {
		t.Error("Expected InsecureSkipVerify true for insecure config")
	}
	if !tlsConfig.AllowInsecure {
		t.Error("Expected AllowInsecure true for WithInsecureTLS")
	}
}

func TestTLSConfigSecurity(t *testing.T) {
	t.Parallel()
	// Test default config is secure
	defaultTLS := config.DefaultTLSConfig()
	if !defaultTLS.IsSecure() {
		t.Error("Default TLS config should be secure")
	}

	// Test insecure config is not secure
	insecureTLS := config.InsecureTLSConfig()
	if insecureTLS.IsSecure() {
		t.Error("Insecure TLS config should not be secure")
	}

	// Test strict config is secure
	strictTLS := config.StrictTLSConfig()
	if !strictTLS.IsSecure() {
		t.Error("Strict TLS config should be secure")
	}

	// Test custom secure config
	customTLS := config.DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS12).
		WithCipherSuites(config.ModernCipherSuites())
	if !customTLS.IsSecure() {
		t.Error("Custom TLS config with TLS 1.2 and modern ciphers should be secure")
	}

	// Test custom insecure config
	customInsecureTLS := config.DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS10).
		WithInsecureSkipVerify(true)
	if customInsecureTLS.IsSecure() {
		t.Error("Custom TLS config with TLS 1.0 and skip verify should not be secure")
	}
}

func TestNewSecureHTTPClientFloorsUnapprovedInsecureTLS(t *testing.T) {
	t.Parallel()

	rootCAs := x509.NewCertPool()
	tlsConfig := config.DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS10).
		WithCipherSuites([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}).
		WithRootCAs(rootCAs).
		WithServerName("api.example.test").
		WithInsecureSkipVerify(true)
	client := NewSecureHTTPClient(30*time.Second, &tlsConfig, nil, "")

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("unapproved insecure TLS disabled certificate verification")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("unapproved insecure TLS was not floored to default MinVersion: %d", transport.TLSClientConfig.MinVersion)
	}
	if transport.TLSClientConfig.RootCAs != rootCAs {
		t.Fatal("unapproved insecure TLS lost custom RootCAs")
	}
	if transport.TLSClientConfig.ServerName != "api.example.test" {
		t.Fatalf("unapproved insecure TLS lost ServerName: %q", transport.TLSClientConfig.ServerName)
	}
	if got := transport.TLSClientConfig.CipherSuites; len(got) != 1 || got[0] != tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 {
		t.Fatalf("unapproved insecure TLS lost custom CipherSuites: %v", got)
	}
}

func TestNewSecureHTTPClientPreservesExplicitlyAllowedInsecureTLS(t *testing.T) {
	t.Parallel()

	tlsConfig := config.DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS10).
		WithInsecureSkipVerify(true).
		WithAllowInsecure(true)
	client := NewSecureHTTPClient(30*time.Second, &tlsConfig, nil, "")

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("explicitly allowed insecure TLS did not preserve InsecureSkipVerify")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS10 {
		t.Fatalf("explicitly allowed insecure TLS did not preserve MinVersion: %d", transport.TLSClientConfig.MinVersion)
	}
}

func TestProviderConfigRawInsecureTLSIsFlooredUnlessAllowed(t *testing.T) {
	t.Parallel()

	rawInsecure := types.NewProviderConfig("test-key").
		WithTLSConfigParam("min_version", uint16(tls.VersionTLS10)).
		WithTLSConfigParam("insecure_skip_verify", true)
	rawProvider := NewBaseProvider("test-provider", rawInsecure)
	rawTransport, ok := rawProvider.GetHTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}
	if rawTransport.TLSClientConfig.InsecureSkipVerify || rawTransport.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("raw insecure provider TLS was not floored: min=%d skip=%v",
			rawTransport.TLSClientConfig.MinVersion, rawTransport.TLSClientConfig.InsecureSkipVerify)
	}

	allowed := types.NewProviderConfig("test-key").WithInsecureTLS(true)
	allowedProvider := NewBaseProvider("test-provider", allowed)
	allowedTransport, ok := allowedProvider.GetHTTPClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}
	if !allowedTransport.TLSClientConfig.InsecureSkipVerify || allowedTransport.TLSClientConfig.MinVersion != tls.VersionTLS10 {
		t.Fatalf("explicit insecure provider TLS was not preserved: min=%d skip=%v",
			allowedTransport.TLSClientConfig.MinVersion, allowedTransport.TLSClientConfig.InsecureSkipVerify)
	}
}

func TestHTTPClientCreationWithTransportConfig(t *testing.T) {
	t.Parallel()
	// Test with custom transport config
	transportConfig := DefaultHTTPTransportConfig().
		WithConnectionPooling(50, 5, 20, 60*time.Second).
		WithTimeouts(
			15*time.Second,
			20*time.Second,
			5*time.Second,
			2*time.Second,
			0,
		)

	client := NewSecureHTTPClient(30*time.Second, nil, &transportConfig, "")
	if client == nil {
		t.Fatal("NewSecureHTTPClient with custom transport config returned nil")
	}

	// Verify transport settings
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Expected *http.Transport")
	}

	if transport.MaxIdleConns != 50 {
		t.Errorf("Expected MaxIdleConns 50, got %d", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost != 5 {
		t.Errorf("Expected MaxIdleConnsPerHost 5, got %d", transport.MaxIdleConnsPerHost)
	}
	if transport.IdleConnTimeout != 60*time.Second {
		t.Errorf("Expected IdleConnTimeout 60s, got %v", transport.IdleConnTimeout)
	}
}

func TestTransportCacheMetrics(t *testing.T) {
	t.Parallel()
	tc := NewTransportCache()

	// Create a unique transport config
	configA := DefaultHTTPTransportConfig()
	configA.MaxIdleConns = 9999
	configA.MaxIdleConnsPerHost = 1111

	// First call with configA — miss (fresh cache)
	client1 := tc.newSecureHTTPClient(30*time.Second, nil, &configA, "")
	if client1 == nil {
		t.Fatal("newSecureHTTPClient returned nil")
	}

	metrics1 := tc.Metrics()
	if metrics1.Misses != 1 {
		t.Errorf("Expected 1 miss after first call, got %d", metrics1.Misses)
	}
	if metrics1.Hits != 0 {
		t.Errorf("Expected 0 hits after first call, got %d", metrics1.Hits)
	}

	// Second call with same configA — hit
	client2 := tc.newSecureHTTPClient(30*time.Second, nil, &configA, "")
	if client2 == nil {
		t.Fatal("newSecureHTTPClient returned nil")
	}

	metrics2 := tc.Metrics()
	if metrics2.Hits != 1 {
		t.Errorf("Expected 1 hit after second call with same config, got %d", metrics2.Hits)
	}

	// Third call with different config — miss
	configB := DefaultHTTPTransportConfig()
	configB.MaxIdleConns = 8888
	configB.MaxIdleConnsPerHost = 2222
	client3 := tc.newSecureHTTPClient(30*time.Second, nil, &configB, "")
	if client3 == nil {
		t.Fatal("newSecureHTTPClient returned nil")
	}

	metrics3 := tc.Metrics()
	if metrics3.Misses != 2 {
		t.Errorf("Expected 2 misses after different config, got %d", metrics3.Misses)
	}
	if metrics3.Size < 2 {
		t.Errorf("Expected cache size >= 2 with two distinct configs, got %d", metrics3.Size)
	}
}

func TestTransportCacheEvictionBounded(t *testing.T) {
	t.Parallel()
	tc := NewTransportCache()

	firstConfig := DefaultHTTPTransportConfig()
	firstConfig.MaxIdleConns = 1000
	firstKey := firstConfig.CacheKey("https://host-0.example")
	// Seed the first (oldest) entry explicitly so we can assert it gets evicted.
	tc.newSecureHTTPClient(30*time.Second, nil, &firstConfig, "https://host-0.example")

	for i := 0; i < maxCachedTransports+8; i++ {
		cfg := DefaultHTTPTransportConfig()
		cfg.MaxIdleConns = 1000 + i
		client := tc.newSecureHTTPClient(30*time.Second, nil, &cfg, "https://host-"+string(rune('a'+(i%26)))+".example/"+time.Duration(i).String())
		if client == nil {
			t.Fatal("newSecureHTTPClient returned nil")
		}
	}

	metrics := tc.Metrics()
	if metrics.Size != maxCachedTransports {
		t.Fatalf("expected cache size %d, got %d", maxCachedTransports, metrics.Size)
	}

	tc.mu.RLock()
	_, exists := tc.transports[firstKey]
	tc.mu.RUnlock()
	if exists {
		t.Fatalf("expected oldest transport %q to be evicted", firstKey)
	}
}

func TestTransportCacheInstancesAreIsolated(t *testing.T) {
	t.Parallel()
	cfg := DefaultHTTPTransportConfig()
	cfg.MaxIdleConns = 4242

	tcA := NewTransportCache()
	tcB := NewTransportCache()

	// Populate cache A only.
	tcA.newSecureHTTPClient(30*time.Second, nil, &cfg, "https://isolated.example")
	tcA.newSecureHTTPClient(30*time.Second, nil, &cfg, "https://isolated.example") // hit

	mA := tcA.Metrics()
	if mA.Size == 0 {
		t.Fatalf("cache A expected to hold at least one transport, got size 0")
	}
	if mA.Hits == 0 {
		t.Fatalf("cache A expected at least one hit on repeated identical config")
	}

	mB := tcB.Metrics()
	if mB.Size != 0 || mB.Hits != 0 || mB.Misses != 0 {
		t.Fatalf("cache B must be untouched by activity on cache A: %+v", mB)
	}
}
