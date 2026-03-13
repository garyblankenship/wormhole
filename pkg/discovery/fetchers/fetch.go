package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/garyblankenship/wormhole/pkg/providers"
)

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

func fetchJSON(ctx context.Context, req *http.Request, out any) error {
	resp, err := getDefaultClient().Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch models: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func newGetRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return req, nil
}
