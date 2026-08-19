package controlplane

import (
	"context"
	"fmt"
	netmail "net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/cache"
	security "github.com/starprince1234/Nebula-api/internal/infrastructure/crypto"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikey"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikeyaudit"
	inframail "github.com/starprince1234/Nebula-api/internal/infrastructure/mail"
)

type Config struct {
	AccessTTL       time.Duration
	RefreshTTL      time.Duration
	VerificationTTL time.Duration
	SendCooldown    time.Duration
	MaxAttempts     int
	Lockout         time.Duration
	InvitationTTL   time.Duration
	PublicAppURL    string
}

type Service struct {
	db       *ent.Client
	cache    *cache.Store
	security *security.Manager
	mailer   inframail.Sender
	config   Config
	now      func() time.Time
}

type Identity struct {
	UserID uuid.UUID
	Role   string
}

func NewService(db *ent.Client, cacheStore *cache.Store, securityManager *security.Manager, mailer inframail.Sender, cfg Config) *Service {
	return &Service{
		db: db, cache: cacheStore, security: securityManager, mailer: mailer,
		config: cfg, now: time.Now,
	}
}

func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := netmail.ParseAddress(value)
	if err != nil || address.Address != value || len(value) > 254 {
		return "", domain.NewError(domain.CodeValidation, "invalid email")
	}
	return value, nil
}

func ValidateName(value string, maxLength int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > maxLength {
		return "", domain.NewError(domain.CodeValidation, "invalid name")
	}
	return value, nil
}

func ValidateComment(action, value string) (string, error) {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 1000 {
		return "", domain.NewError(domain.CodeValidation, "comment is too long")
	}
	if err := domain.ValidateAuditComment(action, value); err != nil {
		return "", err
	}
	return value, nil
}

func ValidateBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", domain.NewError(domain.CodeValidation, "invalid provider base URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", domain.NewError(domain.CodeValidation, "provider base URL must use http or https")
	}
	return value, nil
}

func UUID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, domain.NewError(domain.CodeValidation, "invalid resource ID")
	}
	return id, nil
}

func (s *Service) keyView(ctx context.Context, query *ent.APIKeyQuery) (APIKeyView, error) {
	key, err := query.
		WithOwner().
		WithProject(func(projectQuery *ent.ProjectQuery) {
			projectQuery.WithOrganization()
		}).
		WithModels(func(modelLinkQuery *ent.APIKeyModelQuery) {
			modelLinkQuery.WithModel()
		}).
		WithAudits(func(auditQuery *ent.APIKeyAuditQuery) {
			auditQuery.Order(ent.Asc(apikeyaudit.FieldCreatedAt)).WithActor()
		}).
		Only(ctx)
	if ent.IsNotFound(err) {
		return APIKeyView{}, domain.NewError(domain.CodeNotFound, "API key not found")
	}
	if err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query API key", err)
	}
	return makeAPIKeyView(key), nil
}

func makeAPIKeyView(key *ent.APIKey) APIKeyView {
	view := APIKeyView{
		ID: key.ID.String(), Name: key.Name, Status: string(key.Status),
		KeyPrefix: key.KeyPrefix, ClaimedAt: key.ClaimedAt,
		CreatedAt: key.CreatedAt, UpdatedAt: key.UpdatedAt,
	}
	if key.Edges.Owner != nil {
		view.Applicant = userView(key.Edges.Owner)
	}
	if key.Edges.Project != nil {
		view.Project = projectView(key.Edges.Project, false)
		if key.Edges.Project.Edges.Organization != nil {
			view.Organization = organizationView(key.Edges.Project.Edges.Organization)
		}
	}
	for _, link := range key.Edges.Models {
		if link.Edges.Model != nil {
			view.Models = append(view.Models, modelView(link.Edges.Model))
		}
	}
	for _, audit := range key.Edges.Audits {
		item := AuditView{
			Action: string(audit.Action), Comment: audit.Comment, CreatedAt: audit.CreatedAt,
		}
		if audit.Edges.Actor != nil {
			item.ActorRole = string(audit.Edges.Actor.Role)
		}
		view.Audits = append(view.Audits, item)
	}
	view.Progress = progressForStatus(view.Status, view.Audits)
	return view
}

func progressForStatus(status string, audits []AuditView) ProgressView {
	switch status {
	case domain.KeyPendingMentor:
		return ProgressView{Current: "mentor_review", CompletedSteps: []string{"submitted"}}
	case domain.KeyPendingTeacher:
		return ProgressView{Current: "teacher_review", CompletedSteps: []string{"submitted", "mentor_review"}}
	case domain.KeyApproved:
		return ProgressView{Current: "claim", CompletedSteps: []string{"submitted", "mentor_review", "teacher_review"}}
	case domain.KeyActive:
		return ProgressView{Current: "active", CompletedSteps: []string{"submitted", "mentor_review", "teacher_review", "claimed"}}
	case domain.KeyRejected:
		for _, audit := range audits {
			if audit.Action == string(apikeyaudit.ActionTeacherRejected) {
				return ProgressView{Current: "rejected_teacher", CompletedSteps: []string{"submitted", "mentor_review"}}
			}
		}
		return ProgressView{Current: "rejected", CompletedSteps: []string{"submitted"}}
	case domain.KeyRevoked:
		return ProgressView{Current: "revoked", CompletedSteps: []string{"submitted", "mentor_review", "teacher_review", "claimed"}}
	default:
		return ProgressView{Current: status}
	}
}

func (s *Service) publishKeyStatus(ctx context.Context, ownerID uuid.UUID, keyID uuid.UUID, status string, updatedAt time.Time) {
	_ = s.cache.PublishUserEvent(ctx, ownerID.String(), "api_key.status_changed", map[string]any{
		"api_key_id": keyID.String(),
		"status":     status,
		"updated_at": updatedAt.UTC(),
	})
}

func keyStatus(status string) (apikey.Status, error) {
	switch status {
	case domain.KeyPendingMentor:
		return apikey.StatusPendingMentor, nil
	case domain.KeyPendingTeacher:
		return apikey.StatusPendingTeacher, nil
	case domain.KeyApproved:
		return apikey.StatusApproved, nil
	case domain.KeyActive:
		return apikey.StatusActive, nil
	case domain.KeyRejected:
		return apikey.StatusRejected, nil
	case domain.KeyRevoked:
		return apikey.StatusRevoked, nil
	default:
		return "", fmt.Errorf("unknown API key status %q", status)
	}
}
