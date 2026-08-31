package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSyncUsesAtomicSnapshot(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","success":true,"data":[{"id":"model-a","owned_by":"provider","supported_endpoint_types":["openai"],"model_type":"文本","description":"desc","tags":"对话"}]}`))
	}))
	defer server.Close()
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE matrix_model_catalog SET status='inactive',synced_at=$1,updated_at=$1`)).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO matrix_model_catalog`).WithArgs(sqlmock.AnyArg(), "model-a", "desc", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := (&Syncer{DB: database, APIKey: "secret", URL: server.URL}).Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFailureDoesNotStartSnapshotTransaction(t *testing.T) {
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "failed", http.StatusBadGateway) }))
	defer server.Close()
	if err := (&Syncer{DB: database, APIKey: "secret", URL: server.URL}).Sync(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
