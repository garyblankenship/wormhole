package providers

import (
	"crypto/tls"
	"net/http"
	"testing"
	"time"

	"github.com/garyblankenship/wormhole/pkg/config"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func TestNewSecureHTTPClient(t *testing.T) {
	// Test 1: Default secure client
	client := NewSecureHTTPClient(30*time.Second, nil, nil)
	if client == nil {
		t.Fatal("NewSecureHTTPClient returned nil")
	}
	if client.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", client.Timeout)
	}

	// Test 2: Client with custom TLS config
	tlsConfig := config.DefaultTLSConfig().WithMinVersion(tls.VersionTLS13)
	client2 := NewSecureHTTPClient(60*time.Second, &tlsConfig, nil)
	if client2 == nil {
		t.Fatal("NewSecureHTTPClient with custom TLS returned nil")
	}

	// Test 3: Insecure client
	client3 := NewInsecureHTTPClient(30*time.Second, false)
	if client3 == nil {
		t.Fatal("NewInsecureHTTPClient returned nil")
	}

	// Test 4: Strict client
	client4 := NewStrictHTTPClient(30*time.Second)
	if client4 == nil {
		t.Fatal("NewStrictHTTPClient returned nil")
	}
}

func TestHTTPTransportConfig(t *testing.T) {
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
}

func TestTLSConfigSecurity(t *testing.T) {
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

func TestHTTPClientCreationWithTransportConfig(t *testing.T) {
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

	client := NewSecureHTTPClient(30*time.Second, nil, &transportConfig)
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
	// Get initial metrics
	initial := GetTransportCacheMetrics()

	// Create a unique transport config that likely hasn't been cached before
	configA := DefaultHTTPTransportConfig()
	configA.MaxIdleConns = 9999
	configA.MaxIdleConnsPerHost = 1111

	// Create first client with configA - likely a miss (new transport)
	client1 := NewSecureHTTPClient(30*time.Second, nil, &configA)
	if client1 == nil {
		t.Fatal("NewSecureHTTPClient returned nil")
	}

	metrics1 := GetTransportCacheMetrics()
	// We can't guarantee miss increase if configA was already cached from previous tests
	// But we can verify that hits increased if it was a hit, or misses increased if it was a miss
	// Just record the delta
	missDelta := metrics1.Misses - initial.Misses
	hitDelta := metrics1.Hits - initial.Hits

	// Create second client with same configA - should be a hit (cached transport)
	client2 := NewSecureHTTPClient(30*time.Second, nil, &configA)
	if client2 == nil {
		t.Fatal("NewSecureHTTPClient returned nil")
	}

	metrics2 := GetTransportCacheMetrics()
	if metrics2.Hits <= metrics1.Hits {
		t.Errorf("Expected hit count to increase for same config, got %d (previous %d)", metrics2.Hits, metrics1.Hits)
	}

	// Create a different transport config - should be a miss (different fingerprint)
	configB := DefaultHTTPTransportConfig()
	configB.MaxIdleConns = 8888
	configB.MaxIdleConnsPerHost = 2222
	client3 := NewSecureHTTPClient(30*time.Second, nil, &configB)
	if client3 == nil {
		t.Fatal("NewSecureHTTPClient returned nil")
	}

	metrics3 := GetTransportCacheMetrics()
	if metrics3.Misses <= metrics2.Misses {
		t.Errorf("Expected miss count to increase with different config, got %d (previous %d)", metrics3.Misses, metrics2.Misses)
	}

	// Size should increase with new transports (unless configB was already cached)
	sizeIncreased := metrics3.Size > metrics2.Size
	if !sizeIncreased && metrics3.Misses > metrics2.Misses {
		t.Errorf("Miss count increased but cache size didn't: size %d (previous %d)", metrics3.Size, metrics2.Size)
	}

	// Verify that at least one hit occurred (second client)
	if metrics2.Hits == initial.Hits && missDelta == 0 && hitDelta == 0 {
		t.Log("Note: All transports were already cached from previous tests")
	}
}