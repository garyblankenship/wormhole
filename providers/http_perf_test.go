package providers

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/garyblankenship/wormhole/v2/types"
)

var (
	benchmarkHTTPBytes    []byte
	benchmarkHTTPResponse *http.Response
	benchmarkHTTPRequest  *http.Request
)

type benchmarkRequestBody struct {
	Data string `json:"data"`
}

func BenchmarkHTTPClientWrapperMarshalRequestBody(b *testing.B) {
	wrapper := NewHTTPClientWrapper("benchmark", types.ProviderConfig{}, nil, &NoAuthStrategy{}, nil)

	for _, size := range []int{1 << 10, 64 << 10, 1 << 20} {
		b.Run(benchmarkSizeName(size), func(b *testing.B) {
			body := benchmarkRequestBody{Data: strings.Repeat("x", size)}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				payload, err := wrapper.marshalRequestBody(body)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkHTTPBytes = payload
			}
		})
	}
}

type successfulBenchmarkHTTPClient struct {
	response http.Response
}

func (c *successfulBenchmarkHTTPClient) Do(req *http.Request) (*http.Response, error) {
	benchmarkHTTPRequest = req
	c.response.Request = req
	return &c.response, nil
}

func BenchmarkRetryableHTTPClientSuccessfulFirstAttempt(b *testing.B) {
	transport := &successfulBenchmarkHTTPClient{response: http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}}
	client := newRetryableHTTPClient(transport, retryConfig{})

	for _, size := range []int{-1, 1 << 10, 1 << 20} {
		name := "nil"
		if size >= 0 {
			name = benchmarkSizeName(size)
		}
		b.Run(name, func(b *testing.B) {
			var reader io.Reader
			if size >= 0 {
				reader = bytes.NewReader(bytes.Repeat([]byte("x"), size))
			}
			req, err := http.NewRequest(http.MethodPost, "https://example.test", reader)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				resp, err := client.Do(req)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkHTTPResponse = resp
			}
		})
	}
}

func benchmarkSizeName(size int) string {
	switch size {
	case 1 << 10:
		return "1KiB"
	case 64 << 10:
		return "64KiB"
	case 1 << 20:
		return "1MiB"
	default:
		return "unknown"
	}
}
