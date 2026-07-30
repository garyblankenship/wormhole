package providers

import (
	"bytes"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
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

// BenchmarkReadResponseBodyLimitedSizeTiers tracks the bounded response-body
// path at the small and medium tiers used by the retention-candidate gate.
// It deliberately returns every successful buffer so the benchmark observes
// the production ownership contract rather than accumulating benchmark data.
func BenchmarkReadResponseBodyLimitedSizeTiers(b *testing.B) {
	for _, size := range []int{4 << 10, 64 << 10, 1 << 20} {
		b.Run(benchmarkSizeName(size), func(b *testing.B) {
			body := bytes.Repeat([]byte("x"), size)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				got, err := readResponseBodyLimited(bytes.NewReader(body))
				if err != nil {
					b.Fatal(err)
				}
				if len(got) != len(body) {
					b.Fatalf("body length = %d, want %d", len(got), len(body))
				}
				returnResponseBuf(got)
			}
		})
	}
}

// BenchmarkResponseBodyPoolRetentionProbe is an eligibility probe, not a
// throughput result. The Phase-A command runs it as twenty one-iteration
// samples and takes the lower median of retained-after-gc-B.
func BenchmarkResponseBodyPoolRetentionProbe(b *testing.B) {
	if b.N != 1 {
		b.Fatalf("BenchmarkResponseBodyPoolRetentionProbe requires -benchtime=1x; got %dx", b.N)
	}
	responseBodyPool = sync.Pool{
		New: func() any {
			buf := make([]byte, 0, 4<<10)
			return &buf
		},
	}
	large := bytes.Repeat([]byte("x"), 8<<20)
	b.ResetTimer()
	got, err := readResponseBodyLimited(bytes.NewReader(large))
	if err != nil {
		b.Fatal(err)
	}
	returnResponseBuf(got)
	runtime.GC()

	drained := make([]*[]byte, 32)
	maxRetained := 0
	for i := range drained {
		drained[i] = responseBodyPool.Get().(*[]byte)
		if capacity := cap(*drained[i]); capacity > maxRetained {
			maxRetained = capacity
		}
	}
	runtime.KeepAlive(drained)
	b.ReportMetric(float64(maxRetained), "max-retained-after-gc-B")
}

func benchmarkSizeName(size int) string {
	switch size {
	case 4 << 10:
		return "4KiB"
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
