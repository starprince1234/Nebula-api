package controlplane

import (
	"testing"

	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/model"
)

func TestNormalizeRequestedModels(t *testing.T) {
	contextWindow := 128000
	ids, requested, err := normalizeRequestedModels([]string{"gpt-4.1"}, []RequestedModelInput{{
		ModelID: " Lab-Model ", DisplayName: "Lab Model", Category: "multimodal",
		Capabilities: []string{"vision", "Vision"}, InputModalities: []string{"text", "image"},
		OutputModalities: []string{"text"}, ContextWindow: &contextWindow,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[1] != "Lab-Model" {
		t.Fatalf("unexpected model IDs: %#v", ids)
	}
	item := requested["lab-model"]
	if item.Category != string(model.CategoryMultimodal) || len(item.Capabilities) != 1 {
		t.Fatalf("unexpected requested model: %#v", item)
	}
}

func TestNormalizeRequestedModelsRequiresCompleteMetadata(t *testing.T) {
	_, _, err := normalizeRequestedModels(nil, []RequestedModelInput{{ModelID: "lab-model", DisplayName: "Lab", Category: "text"}})
	if err == nil {
		t.Fatal("expected incomplete requested model to fail")
	}
}

func TestProgressForTeacherRejection(t *testing.T) {
	progress := progressForStatus("rejected", []AuditView{{Action: "teacher_rejected"}})
	if progress.Current != "rejected_teacher" || len(progress.CompletedSteps) != 2 {
		t.Fatalf("unexpected progress: %#v", progress)
	}
}
