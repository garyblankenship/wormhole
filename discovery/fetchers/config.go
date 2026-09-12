package fetchers

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"

	"github.com/garyblankenship/wormhole/v3/types"
)

// configuredFetcherConfig is the discovery subset of ProviderConfig. It keeps
// discovery request construction aligned with provider requests without
// retaining a caller-owned headers map.
type configuredFetcherConfig struct {
	apiKey  string
	baseURL string
	headers map[string]string
	noAuth  bool
	scope   string
}

func newConfiguredFetcherConfig(defaultBaseURL string, config types.ProviderConfig) configuredFetcherConfig {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	headers := canonicalHeaders(config.Headers)
	apiKey := config.EffectiveAPIKey()
	return configuredFetcherConfig{
		apiKey:  apiKey,
		baseURL: baseURL,
		headers: headers,
		noAuth:  config.NoAuth,
		scope:   configuredAccountDiscriminator(baseURL, config.NoAuth, apiKey, headers),
	}
}

func canonicalHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	canonical := make(map[string]string, len(headers))
	for _, key := range keys {
		canonical[http.CanonicalHeaderKey(key)] = headers[key]
	}
	return canonical
}

func configuredAccountDiscriminator(baseURL string, noAuth bool, apiKey string, headers map[string]string) string {
	hash := sha256.New()
	writeScopePart(hash, baseURL)
	if noAuth {
		writeScopePart(hash, "true")
	} else {
		writeScopePart(hash, "false")
	}
	writeScopePart(hash, apiKey)

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		writeScopePart(hash, key)
		writeScopePart(hash, headers[key])
	}

	return hex.EncodeToString(hash.Sum(nil))
}

func writeScopePart(hash interface{ Write([]byte) (int, error) }, value string) {
	_, _ = hash.Write([]byte(value))
	_, _ = hash.Write([]byte{0})
}

func applyHeaders(req *http.Request, headers map[string]string) {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		req.Header.Set(key, headers[key])
	}
}
