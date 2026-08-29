package usage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageJSONContractUsesSnakeCase(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(ProjectUsage{ProjectID: "project", ProjectName: "Project", Quota: "20.000", Members: []MemberUsage{{UserID: "user", UserName: "User", Keys: []KeyMemberUsage{{ID: "key", Quota: "2.000"}}}}, Models: []UsageSlice{}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{`"project_id"`, `"project_name"`, `"quota"`, `"members"`, `"user_id"`, `"keys"`, `"models"`} {
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
