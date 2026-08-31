package catalog

import "testing"

func TestMergeMatrixEntriesCombinesDuplicateModelIDs(t *testing.T) {
	items := MergeEntries([]MatrixEntry{
		{ID: "MiniMax-M2.5", OwnedBy: "custom", Description: "short"},
		{ID: "minimax-m2.5", OwnedBy: "minimax", ModelType: "文本", Description: "longer description", SupportedEndpointTypes: []string{"openai"}, Tags: "对话,工具"},
	})
	if len(items) != 1 || items[0].ModelID != "MiniMax-M2.5" || items[0].Description != "longer description" {
		t.Fatalf("unexpected merge: %#v", items)
	}
	if len(items[0].OwnedBy) != 2 || len(items[0].SupportedEndpointTypes) != 1 || len(items[0].Tags) != 2 || len(items[0].RawEntries) != 2 {
		t.Fatalf("metadata was not merged: %#v", items[0])
	}
}

func TestSuggestedConfigurationUsesDeterministicMetadata(t *testing.T) {
	suggestion := SuggestConfiguration(Item{ModelID: "qwen3-vl-think", Description: "支持视觉理解、联网搜索和 structured_output", ModelTypes: []string{"文本"}, SupportedEndpointTypes: []string{"openai-response"}, Tags: []string{"对话", "工具", "识图", "思考"}})
	if suggestion.Category != "multimodal" || !contains(suggestion.Capabilities, "reasoning") || !contains(suggestion.Capabilities, "vision") || !contains(suggestion.Capabilities, "tool_calling") {
		t.Fatalf("unexpected suggestion: %#v", suggestion)
	}
	if !contains(suggestion.Capabilities, "structured_output") || !contains(suggestion.Capabilities, "web_search") || suggestion.ContextWindow != 128000 || suggestion.MaxInputTokens != 128000 || suggestion.MaxOutputTokens != 8000 {
		t.Fatalf("metadata was not fully mapped: %#v", suggestion)
	}
}

func TestSuggestedConfigurationExtractsDescriptionCapacitiesBeforeFamilyFallback(t *testing.T) {
	suggestion := SuggestConfiguration(Item{ModelID: "claude-sonnet-5", Description: "支持 100 万令牌上下文窗口，最大输出 128K", ModelTypes: []string{"文本"}, SupportedEndpointTypes: []string{"anthropic"}, Tags: []string{"对话", "识图", "工具"}})
	if suggestion.ContextWindow != 1000000 || suggestion.MaxInputTokens != 1000000 || suggestion.MaxOutputTokens != 128000 {
		t.Fatalf("description capacities were not extracted: %#v", suggestion)
	}
}

func TestSuggestedConfigurationKeepsSpecificFamilyRulesAheadOfGenericRules(t *testing.T) {
	tests := []struct {
		modelID                  string
		contextWindow, maxOutput int
	}{
		{modelID: "glm-5.1", contextWindow: 1000000, maxOutput: 16000},
		{modelID: "claude-sonnet-4-5-20250929", contextWindow: 200000, maxOutput: 16000},
		{modelID: "text-embedding-3-small", contextWindow: 8192, maxOutput: 4000},
	}
	for _, test := range tests {
		suggestion := SuggestConfiguration(Item{ModelID: test.modelID})
		if suggestion.ContextWindow != test.contextWindow || suggestion.MaxOutputTokens != test.maxOutput {
			t.Fatalf("unexpected capacities for %s: %#v", test.modelID, suggestion)
		}
	}
}
