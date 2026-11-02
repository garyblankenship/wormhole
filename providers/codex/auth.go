package codex

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Credential is an already issued account-bound access token. It is not an API key.
type Credential struct {
	AccessToken string
	AccountID   string
}

// LoadCredential explicitly reads a Codex CLI auth.json without refreshing or
// writing it. The file and JWT payload are bounded before decoding.
func LoadCredential(path string, now time.Time) (Credential, error) {
	if strings.TrimSpace(path) == "" {
		return Credential{}, errors.New("codex auth path is required")
	}
	file, err := os.Open(path) // #nosec G304 -- the caller explicitly supplies the Codex auth.json path.
	if err != nil {
		return Credential{}, fmt.Errorf("open codex auth: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, 1<<20+1))
	if err != nil {
		return Credential{}, fmt.Errorf("read codex auth: %w", err)
	}
	if len(data) > 1<<20 {
		return Credential{}, errors.New("codex auth exceeds 1 MiB")
	}
	// Decode explicit auth.json field names rather than relying on Go's
	// case-insensitive field matching for access_token.
	var wire struct {
		Tokens struct {
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return Credential{}, errors.New("invalid codex auth JSON")
	}
	return ValidateCredential(Credential{wire.Tokens.AccessToken, wire.Tokens.AccountID}, now)
}

// ValidateCredential checks the required fields and unverified JWT expiry.
// It does not verify a signature or prove subscription eligibility.
func ValidateCredential(c Credential, now time.Time) (Credential, error) {
	c.AccessToken = strings.TrimSpace(c.AccessToken)
	c.AccountID = strings.TrimSpace(c.AccountID)
	if c.AccessToken == "" || strings.ContainsAny(c.AccessToken, " \t\r\n") {
		return Credential{}, errors.New("codex access token is missing or invalid")
	}
	if c.AccountID == "" || strings.ContainsAny(c.AccountID, "\r\n") {
		return Credential{}, errors.New("codex account id is missing or invalid")
	}
	parts := strings.Split(c.AccessToken, ".")
	if len(parts) != 3 || len(parts[1]) > 1<<16 {
		return Credential{}, errors.New("codex access token is not a bounded JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(payload) > 1<<16 {
		return Credential{}, errors.New("codex access token has invalid encoding")
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == "" {
		return Credential{}, errors.New("codex access token has no valid expiry")
	}
	expiry, err := claims.Exp.Int64()
	if err != nil || expiry <= now.Unix() {
		return Credential{}, errors.New("codex access token has invalid or expired expiry")
	}
	return c, nil
}
