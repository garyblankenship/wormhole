package config

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// WithMinVersion returns a copy of the TLSConfig with the specified minimum version.
func (c TLSConfig) WithMinVersion(version uint16) TLSConfig {
	c.MinVersion = version
	return c
}

// WithMaxVersion returns a copy of the TLSConfig with the specified maximum version.
func (c TLSConfig) WithMaxVersion(version uint16) TLSConfig {
	c.MaxVersion = version
	return c
}

// WithCipherSuites returns a copy of the TLSConfig with the specified cipher suites.
func (c TLSConfig) WithCipherSuites(cipherSuites []uint16) TLSConfig {
	c.CipherSuites = cipherSuites
	return c
}

// WithInsecureSkipVerify returns a copy of the TLSConfig with InsecureSkipVerify set.
// WARNING: This disables certificate verification and makes connections vulnerable to MITM attacks.
func (c TLSConfig) WithInsecureSkipVerify(skip bool) TLSConfig {
	c.InsecureSkipVerify = skip
	return c
}

// WithAllowInsecure returns a copy of the TLSConfig with insecure TLS settings
// explicitly allowed. This is intended for local development and legacy systems.
func (c TLSConfig) WithAllowInsecure(allow bool) TLSConfig {
	c.AllowInsecure = allow
	return c
}

// WithRootCAs returns a copy of the TLSConfig with custom root CAs.
func (c TLSConfig) WithRootCAs(rootCAs *x509.CertPool) TLSConfig {
	c.RootCAs = rootCAs
	return c
}

// WithServerName returns a copy of the TLSConfig with a specific server name.
func (c TLSConfig) WithServerName(serverName string) TLSConfig {
	c.ServerName = serverName
	return c
}

// WithHandshakeTimeout returns a copy of the TLSConfig with a specific handshake timeout.
func (c TLSConfig) WithHandshakeTimeout(timeout time.Duration) TLSConfig {
	c.HandshakeTimeout = timeout
	return c
}

// Fingerprint returns a string that uniquely identifies the TLS configuration.
// Used for caching transports based on TLS settings.
func (c TLSConfig) Fingerprint() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d|%d|%v|%v|rootca:%s|%s|",
		c.MinVersion, c.MaxVersion, c.InsecureSkipVerify, c.AllowInsecure, rootCAPoolFingerprint(c.RootCAs), c.ServerName)
	for _, cs := range c.CipherSuites {
		fmt.Fprintf(&b, "%d,", cs)
	}
	return b.String()
}

func rootCAPoolFingerprint(pool *x509.CertPool) string {
	if pool == nil {
		return "system"
	}
	// CertPool has no other public deterministic iteration surface suitable for
	// deriving a transport-cache identity.
	subjects := pool.Subjects() //nolint:staticcheck
	if len(subjects) == 0 {
		return "empty"
	}
	// x509.CertPool exposes Clone and Equal in current Go, but still does not
	// expose a stable iterable view of the inserted certificates beyond Subjects.
	// Without reflection into unexported fields, the subject DER set remains the
	// only public, deterministic identity surface available for cache keys.
	sort.Slice(subjects, func(i, j int) bool {
		return bytes.Compare(subjects[i], subjects[j]) < 0
	})
	h := sha256.New()
	for _, subject := range subjects {
		_, _ = fmt.Fprintf(h, "%d:", len(subject))
		_, _ = h.Write(subject)
	}
	return hex.EncodeToString(h.Sum(nil))
}
