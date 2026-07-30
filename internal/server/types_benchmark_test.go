package server

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkParseImageDataURL(b *testing.B) {
	for _, size := range []int{1 << 20, 10 << 20, 20 << 20} {
		b.Run(fmt.Sprintf("%dMiB", size>>20), func(b *testing.B) {
			data := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", size)))
			rawURL := "data:image/png;base64," + data
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				image, err := parseImageURLPart(rawURL)
				if err != nil {
					b.Fatal(err)
				}
				if image.Base64Data != data {
					b.Fatal("base64 payload was not preserved")
				}
			}
		})
	}
}
