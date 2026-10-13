package main

func tourScenarios() []scenario {
	out := []scenario{
		{"text", textExample},
		{"conversation-vision", conversationExample},
		{"sampling-reasoning", controlsExample},
		{"stream", streamExample},
		{"accumulate", accumulateExample},
		{"stream-error", streamErrorExample},
		{"stream-idle", idleExample},
		{"incomplete-output", incompleteExample},
		{"structured-default", structuredExample},
		{"structured-json", structuredExample},
		{"structured-tools", structuredExample},
		{"structured-strict", structuredExample},
		{"schema-helpers", schemaExample},
		{"embeddings", embeddingExample},
		{"embeddings-base64", embeddingExample},
		{"embeddings-batched", embeddingExample},
		{"rerank", rerankExample},
		{"image", imageExample},
		{"speech-to-text", sttExample},
		{"text-to-speech", ttsExample},
		{"tools-manual", manualToolExample},
		{"tools-auto", toolExample},
		{"agent", agentExample},
		{"tool-safety", toolSafetyExample},
		{"fallback", fallbackExample},
		{"provider-fallback", fallbackExample},
		{"retry", retryExample},
		{"key-rotation", retryExample},
		{"batch", batchExample},
		{"batch-collect", batchExample},
		{"batch-first", batchExample},
		{"idempotency", idempotencyExample},
		{"middleware", middlewareExample},
		{"circuit-isolation", circuitExample},
		{"cancel-timeout", cancellationExample},
		{"handles-shutdown", handlesExample},
		{"health", healthExample},
		{"catalog", catalogExample},
		{"model-validation", validationExample},
		{"profiles-local", profilesExample},
		{"custom-testing", customExample},
		{"responses", responsesExample},
		{"responses-stream", responsesExample},
		{"responses-structured", responsesExample},
	}
	for _, provider := range []string{"anthropic", "gemini", "ollama"} {
		for _, operation := range []string{"text", "stream", "structured"} {
			out = append(out, scenario{"native-" + provider + "-" + operation, nativeExample})
		}
		if provider != "anthropic" {
			out = append(out, scenario{"native-" + provider + "-embeddings", nativeExample})
		}
	}
	out = append(out, scenario{"native-gemini-image", nativeExample}, scenario{"factories-env", environmentExample}, scenario{"proxy", proxyExample})
	return out
}
