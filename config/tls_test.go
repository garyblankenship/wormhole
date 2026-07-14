package config

import (
	"crypto/tls"
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLSConfigConstructors(t *testing.T) {
	t.Parallel()
	t.Run("default is secure", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultTLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
		assert.False(t, cfg.InsecureSkipVerify)
		assert.Equal(t, 10*time.Second, cfg.HandshakeTimeout)
		assert.NotEmpty(t, cfg.CipherSuites)
		assert.True(t, cfg.IsSecure())
	})

	t.Run("strict uses only TLS 1.3 ciphers", func(t *testing.T) {
		t.Parallel()
		cfg := StrictTLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS13), cfg.MinVersion)
		assert.Equal(t, TLS13CipherSuites(), cfg.CipherSuites)
		assert.True(t, cfg.IsSecure())
	})

	t.Run("insecure allows TLS 1.0 but still verifies by default", func(t *testing.T) {
		t.Parallel()
		cfg := InsecureTLSConfig()
		assert.Equal(t, uint16(tls.VersionTLS10), cfg.MinVersion)
		assert.False(t, cfg.InsecureSkipVerify)
		assert.False(t, cfg.IsSecure())
		assert.Contains(t, cfg.CipherSuites, uint16(tls.TLS_RSA_WITH_AES_128_GCM_SHA256))
	})
}

func TestTLSConfigIsSecure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  TLSConfig
		want bool
	}{
		{
			name: "tls 1.2 with modern cipher",
			cfg: TLSConfig{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			},
			want: true,
		},
		{
			name: "tls 1.0",
			cfg: TLSConfig{
				MinVersion: tls.VersionTLS10,
			},
			want: false,
		},
		{
			name: "insecure skip verify",
			cfg: TLSConfig{
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true,
			},
			want: false,
		},
		{
			name: "weak cipher",
			cfg: TLSConfig{
				MinVersion:   tls.VersionTLS12,
				CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_CBC_SHA},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.cfg.IsSecure())
		})
	}
}

func TestTLSConfigApplyAndWithers(t *testing.T) {
	t.Parallel()
	rootCAs := x509.NewCertPool()
	cfg := DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS12).
		WithMaxVersion(tls.VersionTLS13).
		WithCipherSuites([]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}).
		WithInsecureSkipVerify(true).
		WithRootCAs(rootCAs).
		WithServerName("api.example.test").
		WithHandshakeTimeout(3 * time.Second)

	applied := cfg.ApplyToTLSConfig(&tls.Config{
		MinVersion: tls.VersionTLS10,
		ServerName: "old.example.test",
	})

	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), cfg.MaxVersion)
	assert.Equal(t, 3*time.Second, cfg.HandshakeTimeout)
	assert.Equal(t, uint16(tls.VersionTLS12), applied.MinVersion)
	assert.Equal(t, uint16(tls.VersionTLS13), applied.MaxVersion)
	assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, applied.CipherSuites)
	assert.True(t, applied.InsecureSkipVerify)
	assert.Same(t, rootCAs, applied.RootCAs)
	assert.Equal(t, "api.example.test", applied.ServerName)
}

func TestTLSConfigApplyToNilBase(t *testing.T) {
	t.Parallel()
	applied := TLSConfig{}.ApplyToTLSConfig(nil)
	require.NotNil(t, applied)
	assert.Equal(t, uint16(tls.VersionTLS13), applied.MinVersion)
}

func TestTLSConfigFingerprint(t *testing.T) {
	t.Parallel()
	base := DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS12).
		WithServerName("api.example.test")

	same := DefaultTLSConfig().
		WithMinVersion(tls.VersionTLS12).
		WithServerName("api.example.test")
	different := base.WithInsecureSkipVerify(true)
	withRoots := base.WithRootCAs(certPoolWithSubjects("root-a"))
	withOtherRoots := base.WithRootCAs(certPoolWithSubjects("root-b"))
	withSameRootsDifferentOrder := base.WithRootCAs(certPoolWithSubjects("root-b", "root-a"))
	withSameRoots := base.WithRootCAs(certPoolWithSubjects("root-a", "root-b"))

	assert.Equal(t, base.Fingerprint(), same.Fingerprint())
	assert.NotEqual(t, base.Fingerprint(), different.Fingerprint())
	assert.NotEqual(t, base.Fingerprint(), withRoots.Fingerprint())
	assert.NotEqual(t, withRoots.Fingerprint(), withOtherRoots.Fingerprint())
	assert.Equal(t, withSameRoots.Fingerprint(), withSameRootsDifferentOrder.Fingerprint())
	assert.Contains(t, base.Fingerprint(), "api.example.test")
}

func certPoolWithSubjects(subjects ...string) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, subject := range subjects {
		pool.AddCert(&x509.Certificate{Raw: []byte(subject), RawSubject: []byte(subject)})
	}
	return pool
}
