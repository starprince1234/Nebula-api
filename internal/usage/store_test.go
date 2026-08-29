package usage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageJSONContractUsesSnakeCase(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(ProjectUsage{ProjectID: "project", ProjectName: "Project", Quota: "20.000", Members: []MemberUsage{{UserID: "user", UserName: "User", Keys: []KeyMemberUsage{{ID: "key", Quota: "2.000", Models: []UsageSlice{{ID: "model", Name: "Free"}}}}, FreeModels: []UsageSlice{{ID: "model", Name: "Free", Credits: "0.000", Calls: 3}}}}, Models: []UsageSlice{}, FreeModels: []UsageSlice{{ID: "model", Name: "Free", Credits: "0.000", Calls: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{`"project_id"`, `"project_name"`, `"quota"`, `"members"`, `"user_id"`, `"keys"`, `"models"`, `"free_models"`, `"calls":3`} {
		if !strings.Contains(text, field) {
			t.Fatalf("missing %s in %s", field, text)
		}
	}
	for _, legacy := range []string{`"ProjectID"`, `"Quota"`, `"Members"`, `"Models"`} {
		if strings.Contains(text, legacy) {
			t.Fatalf("legacy field %s leaked in %s", legacy, text)
		}
	}
}

func TestCallAndInputJSONContractsUseSnakeCase(t *testing.T) {
	t.Parallel()
	for _, value := range []any{CallLog{RequestID: "request", APIKeyID: "key", BillingState: "charged"}, InputMonitorItem{CallID: "call", ContentBytes: 42}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		if strings.Contains(text, `"RequestID"`) || strings.Contains(text, `"CallID"`) || strings.Contains(text, `"ContentBytes"`) {
			t.Fatalf("Go field name leaked in %s", text)
		}
	}
}

func TestSummarizeMemberFreeModelsEqualsMemberSums(t *testing.T) {
	t.Parallel()
	members := []MemberUsage{
		{FreeModels: []UsageSlice{{ID: "model-a", Name: "A", Credits: "0.000", Calls: 2}, {ID: "model-b", Name: "B", Credits: "0.000", Calls: 1}}},
		{FreeModels: []UsageSlice{{ID: "model-a", Name: "A", Credits: "0.000", Calls: 3}}},
	}
	totals := summarizeMemberFreeModels(members)
	if len(totals) != 2 || totals[0].ID != "model-a" || totals[0].Calls != 5 || totals[1].ID != "model-b" || totals[1].Calls != 1 {
		t.Fatalf("unexpected free model totals: %#v", totals)
	}
}
