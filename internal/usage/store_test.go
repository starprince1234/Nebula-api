package usage

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
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

func TestApproveKeyBindsProjectBucketMonth(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := NewStore(database)
	teacherID, keyID, projectID, ownerID, organizationID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	month := Month(time.Now())

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT project_id FROM api_keys WHERE id=$1`)).
		WithArgs(keyID).
		WillReturnRows(sqlmock.NewRows([]string{"project_id"}).AddRow(projectID))
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT monthly_credit_quota_milli FROM projects WHERE id=$1 FOR UPDATE`)).
		WithArgs(projectID).
		WillReturnRows(sqlmock.NewRows([]string{"monthly_credit_quota_milli"}).AddRow(int64(20_000_000)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT k.owner_user_id,p.organization_id FROM api_keys k JOIN projects p ON p.id=k.project_id WHERE k.id=$1 AND k.project_id=$2 AND k.status='pending_teacher' FOR UPDATE OF k`)).
		WithArgs(keyID, projectID).
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id", "organization_id"}).AddRow(ownerID, organizationID))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT m.model_id,m.status,m.credit_multiplier_milli,EXISTS(SELECT 1 FROM model_bindings b JOIN providers p ON p.id=b.provider_id WHERE b.model_id=m.id AND b.status='active' AND p.status='active') FROM api_key_models km JOIN models m ON m.id=km.model_id WHERE km.api_key_id=$1 ORDER BY m.id FOR UPDATE OF m`)).
		WithArgs(keyID).
		WillReturnRows(sqlmock.NewRows([]string{"model_id", "status", "credit_multiplier_milli", "routed"}).AddRow("gpt-5.6-sol", "active", int64(1000), true))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO project_month_credit_buckets(id,project_id,month,quota_milli,allocated_milli,charged_milli,pending_milli,created_at,updated_at) SELECT gen_random_uuid(),p.id,$1,p.monthly_credit_quota_milli,COALESCE(sum(k.monthly_credit_quota_milli) FILTER(WHERE k.status IN ('approved','active')),0),0,0,$2,$2 FROM projects p LEFT JOIN api_keys k ON k.project_id=p.id WHERE p.id=$3 GROUP BY p.id ON CONFLICT(project_id,month) DO NOTHING`)).
		WithArgs(month, sqlmock.AnyArg(), projectID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT quota_milli,allocated_milli FROM project_month_credit_buckets WHERE project_id=$1 AND month=$2 FOR UPDATE`)).
		WithArgs(projectID, month).
		WillReturnRows(sqlmock.NewRows([]string{"quota_milli", "allocated_milli"}).AddRow(int64(20_000_000), int64(0)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE api_keys SET monthly_credit_quota_milli=$1,status='approved',updated_at=$2 WHERE id=$3 AND status='pending_teacher'`)).
		WithArgs(int64(500_000), sqlmock.AnyArg(), keyID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE project_month_credit_buckets SET allocated_milli=allocated_milli+$1,updated_at=$2 WHERE project_id=$3 AND month=$4`)).
		WithArgs(int64(500_000), sqlmock.AnyArg(), projectID, month).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO api_key_month_credit_buckets(id,api_key_id,project_id,month,quota_milli,allocation_active,charged_milli,pending_milli,created_at,updated_at) VALUES(gen_random_uuid(),$1,$2,$3,$4,true,0,0,$5,$5) ON CONFLICT(api_key_id,month) DO UPDATE SET quota_milli=EXCLUDED.quota_milli,allocation_active=true,updated_at=EXCLUDED.updated_at`)).
		WithArgs(keyID, projectID, month, int64(500_000), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO organization_members(id,organization_id,user_id,created_at) VALUES(gen_random_uuid(),$1,$2,$3) ON CONFLICT(organization_id,user_id) DO NOTHING`)).
		WithArgs(organizationID, ownerID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO project_members(id,project_id,user_id,created_at) VALUES(gen_random_uuid(),$1,$2,$3) ON CONFLICT(project_id,user_id) DO NOTHING`)).
		WithArgs(projectID, ownerID, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO api_key_audits(id,api_key_id,actor_user_id,action,comment,created_at) VALUES(gen_random_uuid(),$1,$2,'teacher_approved',NULLIF($3,''),$4)`)).
		WithArgs(keyID, teacherID, "", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	approvedOwnerID, err := store.ApproveKey(context.Background(), teacherID, keyID, 500_000, "")
	if err != nil {
		t.Fatalf("approve key: %v", err)
	}
	if approvedOwnerID != ownerID {
		t.Fatalf("approved owner = %s, want %s", approvedOwnerID, ownerID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunMaintenanceUsesTypedHeartbeatParameters(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := NewStore(database)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT pg_try_advisory_lock($1)`)).
		WithArgs(int64(7_823_409_118)).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO project_month_credit_buckets`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`INSERT INTO api_key_month_credit_buckets`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()
	mock.ExpectQuery(`SELECT id,request_id,month,organization_id,project_id,user_id,api_key_id,model_id,organization_name,project_name,user_name,api_key_name,model_name,multiplier_milli FROM gateway_calls`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "request_id", "month", "organization_id", "project_id", "user_id", "api_key_id", "model_id", "organization_name", "project_name", "user_name", "api_key_name", "model_name", "multiplier_milli"}))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO maintenance_state(id,name,last_success_at,last_error,created_at,updated_at) VALUES(gen_random_uuid(),'credit-maintenance',CASE WHEN $1::text='' THEN $2::timestamptz ELSE NULL END,NULLIF($1::text,''),$2::timestamptz,$2::timestamptz) ON CONFLICT(name) DO UPDATE SET last_success_at=CASE WHEN $1::text='' THEN $2::timestamptz ELSE maintenance_state.last_success_at END,last_error=NULLIF($1::text,''),updated_at=$2::timestamptz`)).
		WithArgs("", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_unlock($1)`)).
		WithArgs(int64(7_823_409_118)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.RunMaintenance(context.Background(), false); err != nil {
		t.Fatalf("run maintenance: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
