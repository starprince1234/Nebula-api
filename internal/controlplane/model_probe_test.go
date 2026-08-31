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

func TestModelCatalogProbeUsesModelsEndpoint(t *testing.T) {
	definition := modelCatalogProbeRequest()
	if definition.Method != "GET" || definition.Path != "/v1/models" || len(definition.Body) != 0 {
		t.Fatalf("unexpected model catalog probe: %#v", definition)
	}
}

func TestExtractProbeConfigurationMatchesModelAndSuggestsMetadata(t *testing.T) {
	configuration := extractProbeConfiguration([]byte(`{"data":[{"id":"other"},{"id":"GLM-5.1","description":"<p>GLM reasoning model</p>","model_type":"文本","tags":"思考,工具","supported_endpoint_types":["openai-response"],"context_window":128000,"max_output_tokens":8192}]}`), "glm-5.1")
	if configuration.Description != "GLM reasoning model" || configuration.Category != "text" || configuration.ContextWindow == nil || *configuration.ContextWindow != 128000 || configuration.MaxInputTokens == nil || *configuration.MaxInputTokens != 128000 || configuration.MaxOutputTokens == nil || *configuration.MaxOutputTokens != 8192 {
		t.Fatalf("unexpected configuration: %#v", configuration)
	}
	if len(configuration.InputModalities) != 1 || configuration.InputModalities[0] != "text" || len(configuration.OutputModalities) != 1 || configuration.OutputModalities[0] != "text" {
		t.Fatalf("unexpected modalities: %#v", configuration)
	}
	want := map[string]bool{"reasoning": false, "tool_calling": false}
	for _, capability := range configuration.Capabilities {
		if _, ok := want[capability]; ok {
			want[capability] = true
		}
	}
	for capability, found := range want {
		if !found {
			t.Fatalf("missing capability %s in %#v", capability, configuration.Capabilities)
		}
	}
}

func TestProbeResponseDoesNotExposeUpstreamBody(t *testing.T) {
	raw, err := json.Marshal(ProbeResponse{Results: []ProbeResult{{Endpoint: "models", Path: "/v1/models", HTTPStatus: 200}}})
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	results, ok := payload["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("unexpected results: %s", raw)
	}
	result, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected result: %s", raw)
	}
	if _, exposed := result["response"]; exposed {
		t.Fatalf("upstream response body was exposed: %s", raw)
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
