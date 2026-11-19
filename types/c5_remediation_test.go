package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotImplementedError_NotRetryable(t *testing.T) {
	t.Parallel()

	err := NotImplementedError("ollama", "Images")
	we, ok := AsWormholeError(err)
	require.True(t, ok)
	assert.Equal(t, ErrorCodeProvider, we.Code)
	assert.Equal(t, "ollama", we.Provider)
	assert.False(t, we.IsRetryable(), "unsupported operations must not be retryable")
	assert.Contains(t, we.Message, "does not support Images")
}

func TestIsNotSupportedError_DistinguishesProviderErrors(t *testing.T) {
	t.Parallel()

	assert.False(t, IsNotSupportedError(nil))
	assert.True(t, IsNotSupportedError(NotImplementedError("ollama", "Images")))
	assert.True(t, IsNotSupportedError(NewBaseProvider("base").NotImplementedError("Text")))
	assert.True(t, IsNotSupportedError(errors.New("custom provider does not support streaming")))
	assert.True(t, IsNotSupportedError(errors.New("endpoint not implemented")))

	assert.False(t, IsNotSupportedError(ProviderError("openai", "HTTP 500: upstream failure")))
	assert.False(t, IsNotSupportedError(ErrProviderNotFound))
	assert.False(t, IsNotSupportedError(ErrProviderUnavailable))
	assert.False(t, IsNotSupportedError(ErrProviderConstraintError))
	assert.False(t, IsNotSupportedError(errors.New("transient socket reset")))
}

func TestObjectSchemaValidate_SkipsUnexportedAndJSONIgnoredFields(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name   string `json:"name"`
		Secret string `json:"-"`
		hidden string
	}

	schema := &ObjectSchema{
		BaseSchema: BaseSchema{Type: "object"},
		Properties: map[string]SchemaInterface{
			"name": &StringSchema{BaseSchema: BaseSchema{Type: "string"}},
		},
		Required: []string{"name"},
	}

	// Must not panic on unexported field `hidden` and must ignore `Secret`.
	require.NoError(t, schema.Validate(payload{Name: "Ada", Secret: "shh", hidden: "internal"}))

	// A required field matching a json:"-" field must be treated as missing.
	schemaRequiringIgnored := &ObjectSchema{
		BaseSchema: BaseSchema{Type: "object"},
		Properties: map[string]SchemaInterface{
			"Secret": &StringSchema{BaseSchema: BaseSchema{Type: "string"}},
		},
		Required: []string{"Secret"},
	}
	require.Error(t, schemaRequiringIgnored.Validate(payload{Name: "Ada", Secret: "shh"}))
}

type trackingProvider struct {
	*BaseProvider
	caps   []ModelCapability
	closed bool
}

func (p *trackingProvider) SupportedCapabilities() []ModelCapability {
	return p.caps
}

func (p *trackingProvider) Close() error {
	p.closed = true
	return nil
}

func TestProviderWrapper_ImplementsProviderAndDelegatesLifecycle(t *testing.T) {
	t.Parallel()

	inner := &trackingProvider{
		BaseProvider: NewBaseProvider("tracking"),
		caps:         []ModelCapability{CapabilityText, CapabilityStream},
	}
	var p Provider = NewProviderWrapper(inner)

	assert.Equal(t, []ModelCapability{CapabilityText, CapabilityStream}, p.SupportedCapabilities())
	require.NoError(t, p.Close())
	assert.True(t, inner.closed)
}

func TestProviderConfigBuilders_DoNotMutateSharedMaps(t *testing.T) {
	t.Parallel()

	base := NewProviderConfig("key").
		WithHeader("X-Base", "1").
		WithParam("base_param", "v1").
		WithTLSConfigParam("server_name", "base.example")

	branchA := base.
		WithHeader("X-Branch", "A").
		WithHeaders(map[string]string{"X-Base": "A"}).
		WithParam("branch_param", "A").
		WithParams(map[string]any{"base_param": "A"}).
		WithTLSConfigParam("server_name", "a.example").
		WithTLSConfigParam("insecure_skip_verify", true)

	branchB := base.
		WithHeader("X-Branch", "B").
		WithParam("branch_param", "B").
		WithTLSConfigParam("server_name", "b.example")

	// Base must remain untouched
	assert.Equal(t, map[string]string{"X-Base": "1"}, base.Headers)
	assert.Equal(t, "v1", base.Params["base_param"])
	assert.NotContains(t, base.Params, "branch_param")
	baseTLS := base.Params[TLSConfigParamKey].(map[string]any)
	assert.Equal(t, "base.example", baseTLS["server_name"])
	assert.NotContains(t, baseTLS, "insecure_skip_verify")

	// Branches must remain isolated from each other
	assert.Equal(t, "A", branchA.Headers["X-Branch"])
	assert.Equal(t, "B", branchB.Headers["X-Branch"])
	assert.Equal(t, "A", branchA.Params["branch_param"])
	assert.Equal(t, "B", branchB.Params["branch_param"])
	assert.Equal(t, "a.example", branchA.Params[TLSConfigParamKey].(map[string]any)["server_name"])
	assert.Equal(t, "b.example", branchB.Params[TLSConfigParamKey].(map[string]any)["server_name"])
}

func TestHTTPStatusToError_ForbiddenIsAuthError(t *testing.T) {
	t.Parallel()

	err := HTTPStatusToError(http.StatusForbidden, "access denied")
	assert.Equal(t, ErrorCodeAuth, err.Code)
	assert.False(t, err.Retryable)
	assert.Equal(t, http.StatusForbidden, err.StatusCode)
	assert.Equal(t, "access denied", err.Details)
}
