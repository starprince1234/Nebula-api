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
	suggestion := SuggestConfiguration(Item{ModelTypes: []string{"文本"}, SupportedEndpointTypes: []string{"openai-response"}, Tags: []string{"对话", "工具", "识图", "思考"}})
	if suggestion.Category != "multimodal" || !contains(suggestion.Capabilities, "reasoning") || !contains(suggestion.Capabilities, "vision") || !contains(suggestion.Capabilities, "tool_calling") {
		t.Fatalf("unexpected suggestion: %#v", suggestion)
	}
}
