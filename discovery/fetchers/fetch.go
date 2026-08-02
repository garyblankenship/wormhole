package fetchers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

const maxDiscoveryResponseBodyBytes = 32 << 20

var (
	defaultClient     *http.Client
	defaultClientOnce sync.Once
)

func getDefaultClient() *http.Client {
	defaultClientOnce.Do(func() {
		defaultClient = providers.NewSecureHTTPClient(30*time.Second, nil, nil, "")
	})
	return defaultClient
}

func fetchJSON(req *http.Request, out any) error {
	resp, err := getDefaultClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch models: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiscoveryResponseBodyBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read model discovery response: %w", err)
	}
	if len(body) > maxDiscoveryResponseBodyBytes {
		return types.ErrRequestTooLarge.WithDetails(
			fmt.Sprintf("model discovery response body exceeded %d bytes", maxDiscoveryResponseBodyBytes),
		)
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(out)
}

func newGetRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return req, nil
}
