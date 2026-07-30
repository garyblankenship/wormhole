package anthropic

import "testing"

func BenchmarkAnthropicParseStreamChunk(b *testing.B) {
	p := &Provider{}
	events := [][]byte{
		[]byte(`{"type":"message_start","message":{"id":"msg_1","model":"claude-sonnet-4-5","usage":{"input_tokens":10}}}`),
		[]byte(`{"type":"content_block_start","content_block":{"type":"tool_use","id":"toolu_1","name":"weather"}}`),
		[]byte(`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{\"city\":\"Boston\"}"}}`),
		[]byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn","usage":{"output_tokens":12}}}`),
		[]byte(`{"type":"error","error":{"type":"overloaded_error","message":"try again"}}`),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chunk, err := p.parseStreamChunk(events[i%len(events)])
		if i%len(events) == len(events)-1 {
			if err == nil {
				b.Fatal("in-band error was not returned")
			}
			continue
		}
		if err != nil {
			b.Fatal(err)
		}
		if chunk == nil {
			b.Fatal("recognized event returned nil chunk")
		}
	}
}
