package controlplane

import (
	"encoding/json"
	"testing"
)

func TestProbeRequestDefinitions(t *testing.T) {
	for _, endpoint := range []string{"chat_completions", "responses", "messages", "embeddings", "rerank"} {
		definition, err := buildProbeRequest(endpoint, "upstream-model")
		if err != nil || definition.Path == "" || len(definition.Body) == 0 {
			t.Fatalf("invalid %s definition: %#v, err=%v", endpoint, definition, err)
		}
		var payload map[string]any
		if err := json.Unmarshal(definition.Body, &payload); err != nil || payload["model"] != "upstream-model" {
			t.Fatalf("invalid %s body: %s", endpoint, definition.Body)
		}
	}
}

func TestExtractProbeLimitsIgnoresUsageAndByteFields(t *testing.T) {
	limits := extractProbeLimits([]byte(`{"context_window":128000,"max_input_tokens":120000,"max_output_tokens":8000,"usage":{"input_tokens":10},"max_bytes":999999}`))
	if limits.ContextWindow == nil || *limits.ContextWindow != 128000 || limits.MaxInputTokens == nil || *limits.MaxInputTokens != 120000 || limits.MaxOutputTokens == nil || *limits.MaxOutputTokens != 8000 {
		t.Fatalf("unexpected limits: %#v", limits)
	}
}

func TestPublicAddressRejectsPrivateRanges(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1"} {
		if isPublicIP(value) {
			t.Fatalf("private address %s was accepted", value)
		}
	}
}

func TestProbeTargetDoesNotDuplicateVersionPrefix(t *testing.T) {
	target, err := probeTarget("https://example.com/v1", "/v1/responses")
	if err != nil || target != "https://example.com/v1/responses" {
		t.Fatalf("unexpected target %q, err=%v", target, err)
	}
}
