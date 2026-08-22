package controlplane

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/user"
)

func TestBootstrapTestAccountsCreatesMissingUser(t *testing.T) {
	service, mock, closeDatabase := newBootstrapTestService(t)
	defer closeDatabase()

	mock.ExpectQuery(`SELECT .* FROM "users" WHERE "users"\."email" = \$1 LIMIT 2`).
		WithArgs("student.one@example.com").
		WillReturnRows(sqlmock.NewRows(user.Columns))
	mock.ExpectExec(`INSERT INTO "users"`).
		WithArgs(
			sqlmock.AnyArg(), sqlmock.AnyArg(), "Student One", "student.one@example.com",
			sqlmock.AnyArg(), user.RoleStudent, user.StatusActive, sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := service.BootstrapTestAccounts(context.Background(), []BootstrapAccount{{
		Role: string(user.RoleStudent), Name: "Student One", Email: "student.one@example.com", Password: "student-password-one",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBootstrapTestAccountsLeavesMatchingExistingUserUnchanged(t *testing.T) {
	service, mock, closeDatabase := newBootstrapTestService(t)
	defer closeDatabase()

	mock.ExpectQuery(`SELECT .* FROM "users" WHERE "users"\."email" = \$1 LIMIT 2`).
		WithArgs("mentor.one@example.com").
		WillReturnRows(sqlmock.NewRows(user.Columns).AddRow(
			uuid.Must(uuid.NewV7()), time.Now(), time.Now(), "Existing Mentor", "mentor.one@example.com",
			"existing-password-hash", user.RoleMentor, user.StatusActive,
		))

	err := service.BootstrapTestAccounts(context.Background(), []BootstrapAccount{{
		Role: string(user.RoleMentor), Name: "Mentor One", Email: "mentor.one@example.com", Password: "mentor-password-one",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareBootstrapAccountNormalizesConfiguredUser(t *testing.T) {
	account, role, err := prepareBootstrapAccount(BootstrapAccount{
		Role: "student", Name: " Student One ", Email: " STUDENT.ONE@EXAMPLE.COM ", Password: "student-password-one",
	})
	if err != nil {
		t.Fatal(err)
	}
	if role != user.RoleStudent || account.Name != "Student One" || account.Email != "student.one@example.com" {
		t.Fatalf("unexpected prepared bootstrap account: %#v, role=%q", account, role)
	}
}

func TestPrepareBootstrapAccountRejectsUnsupportedRole(t *testing.T) {
	_, _, err := prepareBootstrapAccount(BootstrapAccount{
		Role: "teacher", Name: "Teacher", Email: "teacher@example.com", Password: "teacher-password-one",
	})
	if err == nil {
		t.Fatal("expected unsupported bootstrap role to fail")
	}
}

func TestValidateExistingBootstrapAccountRejectsRoleConflict(t *testing.T) {
	err := validateExistingBootstrapAccount(&ent.User{Role: user.RoleMentor}, user.RoleStudent)
	if err == nil {
		t.Fatal("expected role conflict to fail")
	}
}

func TestValidateExistingBootstrapAccountAcceptsMatchingRole(t *testing.T) {
	if err := validateExistingBootstrapAccount(&ent.User{Role: user.RoleMentor}, user.RoleMentor); err != nil {
		t.Fatal(err)
	}
}

func newBootstrapTestService(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
	t.Helper()
	database, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	driver := entsql.OpenDB(dialect.Postgres, database)
	client := ent.NewClient(ent.Driver(driver))
	return &Service{db: client}, mock, func() {
		_ = client.Close()
	}
}
