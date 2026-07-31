package openai

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/garyblankenship/wormhole/v3/providers"
	"github.com/garyblankenship/wormhole/v3/types"
)

func BenchmarkPrepareMessagesProviderPayload(b *testing.B) {
	provider := New(types.ProviderConfig{})
	for _, count := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("%dMessages", count), func(b *testing.B) {
			messages := benchmarkPrepareMessages(count)
			request := types.TextRequest{
				BaseRequest: types.BaseRequest{Model: "benchmark"},
				Messages:    messages,
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				prepared, _, err := providers.PrepareMessages(messages)
				if err != nil {
					b.Fatal(err)
				}
				payload := provider.buildChatPayload(&request, prepared)
				runtime.KeepAlive(payload)
			}
		})
	}
}

func benchmarkPrepareMessages(count int) []types.Message {
	messages := make([]types.Message, 0, count)
	for i := range count {
		messages = append(messages, types.NewUserMessage(fmt.Sprintf("request %d", i)))
	}
	return messages
}
