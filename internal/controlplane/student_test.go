package controlplane

import "testing"

func TestNormalizeRequestedModels(t *testing.T) {
	models, err := normalizeRequestedModels([]RequestedModelInput{{ModelID: "gpt-4.1"}, {ModelID: " Lab-Model ", DisplayName: "Lab Model"}, {ModelID: "lab-model"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[1].ModelID != "Lab-Model" || models[1].DisplayName != "Lab Model" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestNormalizeRequestedModelsDefaultsDisplayName(t *testing.T) {
	models, err := normalizeRequestedModels([]RequestedModelInput{{ModelID: "new-model"}})
	if err != nil || len(models) != 1 || models[0].DisplayName != "new-model" {
		t.Fatalf("unexpected models: %#v, err=%v", models, err)
	}
}

func TestProgressForTeacherRejection(t *testing.T) {
	progress := progressForStatus("rejected", []AuditView{{Action: "teacher_rejected"}})
	if progress.Current != "rejected_teacher" || len(progress.CompletedSteps) != 2 {
		t.Fatalf("unexpected progress: %#v", progress)
	}
}
