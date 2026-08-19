package httpapi

import (
	"encoding/json"
	"testing"
)

func TestNullableIntPayload(t *testing.T) {
	var omitted modelPatchPayload
	if err := json.Unmarshal([]byte(`{"display_name":"x"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.ContextWindow.Set {
		t.Fatal("omitted value must remain unchanged")
	}
	var cleared modelPatchPayload
	if err := json.Unmarshal([]byte(`{"context_window":null}`), &cleared); err != nil {
		t.Fatal(err)
	}
	if !cleared.ContextWindow.Set || cleared.ContextWindow.Value != nil {
		t.Fatal("null must clear the value")
	}
	var updated modelPatchPayload
	if err := json.Unmarshal([]byte(`{"context_window":8192}`), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.ContextWindow.Set || updated.ContextWindow.Value == nil || *updated.ContextWindow.Value != 8192 {
		t.Fatal("positive integer must update the value")
	}
}
