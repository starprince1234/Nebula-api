package controlplane

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/modelbinding"
	"github.com/starprince1234/Nebula-api/internal/usage"
)

func TestMentorCandidateCursorRoundTrip(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	encoded := encodeMentorCandidateCursor("导师甲", id)
	decoded, err := DecodeMentorCandidateCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Name != "导师甲" || decoded.ID != id {
		t.Fatalf("unexpected cursor: %#v", decoded)
	}
}

func TestValidBindingAdapters(t *testing.T) {
	t.Parallel()
	for _, adapter := range []modelbinding.Adapter{
		modelbinding.AdapterOpenaiCompatible,
		modelbinding.AdapterOpenaiResponses,
		modelbinding.AdapterOpenaiEmbeddings,
		modelbinding.AdapterOpenaiImages,
		modelbinding.AdapterOpenaiAudio,
		modelbinding.AdapterOpenaiVideo,
		modelbinding.AdapterOpenaiRealtime,
		modelbinding.AdapterOpenaiModerations,
		modelbinding.AdapterAnthropic,
		modelbinding.AdapterCohereRerankV2,
		modelbinding.AdapterGoogleGeminiV1beta,
	} {
		if !validBindingAdapter(string(adapter)) {
			t.Fatalf("expected adapter %q to be valid", adapter)
		}
	}
	if validBindingAdapter("unknown") {
		t.Fatal("unknown adapter was accepted")
	}
}

func TestMultiplierChangeReasonOnlyRequiredForExistingChangedMultiplier(t *testing.T) {
	old := int64(1000)
	same := int64(1000)
	changed := int64(1250)
	tests := []struct {
		name           string
		current, next  *int64
		requiresReason bool
	}{
		{name: "first configuration", current: nil, next: &same, requiresReason: false},
		{name: "unchanged", current: &old, next: &same, requiresReason: false},
		{name: "changed", current: &old, next: &changed, requiresReason: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := multiplierChangeRequiresReason(tt.current, tt.next); got != tt.requiresReason {
				t.Fatalf("requires reason = %v, want %v", got, tt.requiresReason)
			}
		})
	}
}

func TestMentorCandidateCursorRejectsInvalidInput(t *testing.T) {
	if _, err := DecodeMentorCandidateCursor("not-a-cursor"); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}

func TestTeacherApprovalDependencyErrorDescribesRollback(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	keyID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT project_id FROM api_keys WHERE id=$1`)).
		WithArgs(keyID).
		WillReturnError(errors.New("database unavailable"))

	service := &Service{usage: usage.NewStore(database)}
	err = service.ReviewKeyAsTeacher(context.Background(), uuid.New(), keyID, true, "", 500_000)
	var appErr *domain.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("expected domain error, got %v", err)
	}
	if appErr.Code != domain.CodeDependencyUnavailable {
		t.Fatalf("code = %s, want %s", appErr.Code, domain.CodeDependencyUnavailable)
	}
	details, ok := appErr.Details.(map[string]any)
	if !ok || details["operation"] != "api_key_approval" || details["state_changed"] != false {
		t.Fatalf("unexpected details: %#v", appErr.Details)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
