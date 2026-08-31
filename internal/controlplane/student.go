package controlplane

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikey"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikeymodel"
	entmodel "github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/model"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/organization"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/project"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/projectmember"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/user"
)

type SubmitAPIKeyInput struct {
	Name                    string
	OrganizationID          uuid.UUID
	ProjectID               uuid.UUID
	Models                  []RequestedModelInput
	RequestedMonthlyCredits int64
}

type RequestedModelInput struct {
	ModelID     string
	DisplayName string
}

type ClaimView struct {
	APIKey    string    `json:"api_key"`
	KeyPrefix string    `json:"key_prefix"`
	ClaimedAt time.Time `json:"claimed_at"`
}

func (s *Service) StudentOrganizations(ctx context.Context) ([]OrganizationView, error) {
	rows, err := s.db.Organization.Query().
		Where(organization.StatusEQ(organization.StatusActive)).
		Order(ent.Asc(organization.FieldName)).
		All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list organizations", err)
	}
	result := make([]OrganizationView, 0, len(rows))
	for _, row := range rows {
		result = append(result, organizationView(row))
	}
	return result, nil
}

func (s *Service) StudentProjects(ctx context.Context, organizationID uuid.UUID) ([]ProjectView, error) {
	orgExists, err := s.db.Organization.Query().Where(
		organization.IDEQ(organizationID),
		organization.StatusEQ(organization.StatusActive),
	).Exist(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query organization", err)
	}
	if !orgExists {
		return nil, domain.NewError(domain.CodeNotFound, "organization not found")
	}
	rows, err := s.db.Project.Query().
		Where(project.OrganizationIDEQ(organizationID)).
		Order(ent.Asc(project.FieldName)).
		All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list projects", err)
	}
	projectIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		projectIDs = append(projectIDs, row.ID)
	}
	mentorProjects := make(map[uuid.UUID]bool)
	if len(projectIDs) > 0 {
		members, err := s.db.ProjectMember.Query().Where(
			projectmember.ProjectIDIn(projectIDs...),
			projectmember.HasUserWith(
				user.RoleEQ(user.RoleMentor),
				user.StatusEQ(user.StatusActive),
			),
		).All(ctx)
		if err != nil {
			return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query project mentors", err)
		}
		for _, member := range members {
			mentorProjects[member.ProjectID] = true
		}
	}
	result := make([]ProjectView, 0, len(rows))
	for _, row := range rows {
		result = append(result, projectView(row, mentorProjects[row.ID]))
	}
	return result, nil
}

func (s *Service) StudentModels(ctx context.Context, studentID uuid.UUID) ([]ModelView, error) {
	rows, err := s.db.Model.Query().Where(
		entmodel.Or(
			entmodel.And(
				entmodel.StatusEQ(entmodel.StatusActive),
				entmodel.IsCommonEQ(true),
			),
			entmodel.HasAPIKeyModelsWith(
				apikeymodel.HasAPIKeyWith(
					apikey.OwnerUserIDEQ(studentID),
					apikey.StatusIn(apikey.StatusApproved, apikey.StatusActive),
				),
			),
		),
	).Order(ent.Asc(entmodel.FieldModelID)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list student models", err)
	}
	result := make([]ModelView, 0, len(rows))
	for _, row := range rows {
		result = append(result, modelView(row))
	}
	return result, nil
}

func (s *Service) SubmitAPIKey(ctx context.Context, studentID uuid.UUID, input SubmitAPIKeyInput) (APIKeyView, error) {
	name, err := ValidateName(input.Name, 128)
	if err != nil {
		return APIKeyView{}, err
	}
	requestedModels, err := normalizeRequestedModels(input.Models)
	if err != nil {
		return APIKeyView{}, err
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "start API key transaction", err)
	}
	defer func() { _ = tx.Rollback() }()

	orgExists, err := tx.Organization.Query().Where(
		organization.IDEQ(input.OrganizationID),
		organization.StatusEQ(organization.StatusActive),
	).Exist(ctx)
	if err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query organization", err)
	}
	if !orgExists {
		return APIKeyView{}, domain.NewError(domain.CodeNotFound, "organization not found")
	}
	targetProject, err := tx.Project.Query().Where(
		project.IDEQ(input.ProjectID),
		project.OrganizationIDEQ(input.OrganizationID),
		project.StatusEQ(project.StatusActive),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return APIKeyView{}, domain.NewError(domain.CodeNotFound, "project not found")
	}
	if err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query project", err)
	}
	hasMentor, err := tx.ProjectMember.Query().Where(
		projectmember.ProjectIDEQ(targetProject.ID),
		projectmember.HasUserWith(
			user.RoleEQ(user.RoleMentor),
			user.StatusEQ(user.StatusActive),
		),
	).Exist(ctx)
	if err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query project mentors", err)
	}
	if !hasMentor {
		return APIKeyView{}, domain.NewError(domain.CodeProjectNoMentor, "project has no active mentor")
	}
	if input.RequestedMonthlyCredits < 0 || input.RequestedMonthlyCredits > targetProject.MonthlyCreditQuotaMilli {
		return APIKeyView{}, domain.NewError(domain.CodeValidation, "requested monthly credits exceed project quota")
	}

	modelRows := make([]*ent.Model, 0, len(requestedModels))
	for _, requested := range requestedModels {
		modelID := requested.ModelID
		row, queryErr := tx.Model.Query().Where(entmodel.ModelIDEQ(modelID)).Only(ctx)
		if ent.IsNotFound(queryErr) {
			builder := tx.Model.Create().
				SetModelID(requested.ModelID).
				SetDisplayName(requested.DisplayName).
				SetCategory(entmodel.CategoryText).
				SetStatus(entmodel.StatusPendingConfiguration)
			row, queryErr = builder.Save(ctx)
			if ent.IsConstraintError(queryErr) {
				row, queryErr = tx.Model.Query().Where(entmodel.ModelIDEQ(modelID)).Only(ctx)
			}
		}
		if queryErr != nil {
			return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "resolve requested model", queryErr)
		}
		modelRows = append(modelRows, row)
	}
	key, err := tx.APIKey.Create().
		SetOwnerUserID(studentID).
		SetProjectID(targetProject.ID).
		SetName(name).
		SetStatus(apikey.StatusPendingMentor).
		SetRequestedMonthlyCreditQuotaMilli(input.RequestedMonthlyCredits).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return APIKeyView{}, domain.NewError(domain.CodeNameConflict, "API key name is already in use")
	}
	if err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create API key application", err)
	}
	for _, row := range modelRows {
		if _, err := tx.APIKeyModel.Create().
			SetAPIKeyID(key.ID).
			SetModelID(row.ID).
			Save(ctx); err != nil {
			return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create API key model link", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return APIKeyView{}, domain.WrapError(domain.CodeDependencyUnavailable, "commit API key transaction", err)
	}
	view, err := s.keyView(ctx, s.db.APIKey.Query().Where(
		apikey.IDEQ(key.ID),
		apikey.OwnerUserIDEQ(studentID),
	))
	if err == nil {
		s.publishKeyStatus(ctx, studentID, key.ID, domain.KeyPendingMentor, key.UpdatedAt)
	}
	return view, err
}

func normalizeRequestedModels(requested []RequestedModelInput) ([]RequestedModelInput, error) {
	if len(requested) < 1 || len(requested) > 100 {
		return nil, domain.NewError(domain.CodeValidation, "models must contain 1 to 100 items")
	}
	result := make([]RequestedModelInput, 0, len(requested))
	seen := map[string]struct{}{}
	for _, item := range requested {
		modelID, err := normalizeSingleModelID(item.ModelID)
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(modelID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		displayName := strings.TrimSpace(item.DisplayName)
		if displayName == "" {
			displayName = modelID
		} else {
			displayName, err = ValidateName(displayName, 256)
			if err != nil {
				return nil, err
			}
		}
		item.ModelID = modelID
		item.DisplayName = displayName
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) StudentAPIKeys(ctx context.Context, studentID uuid.UUID) ([]APIKeyView, error) {
	rows, err := s.db.APIKey.Query().
		Where(apikey.OwnerUserIDEQ(studentID)).
		Order(ent.Desc(apikey.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list API keys", err)
	}
	result := make([]APIKeyView, 0, len(rows))
	for _, row := range rows {
		view, err := s.keyView(ctx, s.db.APIKey.Query().Where(
			apikey.IDEQ(row.ID),
			apikey.OwnerUserIDEQ(studentID),
		))
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func (s *Service) StudentAPIKey(ctx context.Context, studentID, keyID uuid.UUID) (APIKeyView, error) {
	return s.keyView(ctx, s.db.APIKey.Query().Where(
		apikey.IDEQ(keyID),
		apikey.OwnerUserIDEQ(studentID),
	))
}

func (s *Service) ClaimAPIKey(ctx context.Context, studentID, keyID uuid.UUID) (ClaimView, error) {
	secret, prefix, hash, err := s.security.NewAPIKey()
	if err != nil {
		return ClaimView{}, domain.WrapError("INTERNAL_ERROR", "generate API key", err)
	}
	claimedAt := s.now().UTC()
	updated, err := s.db.APIKey.Update().
		Where(
			apikey.IDEQ(keyID),
			apikey.OwnerUserIDEQ(studentID),
			apikey.StatusEQ(apikey.StatusApproved),
		).
		SetStatus(apikey.StatusActive).
		SetKeyHash(hash).
		SetKeyPrefix(prefix).
		SetClaimedAt(claimedAt).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return ClaimView{}, domain.NewError(domain.CodeInvalidTransition, "API key could not be claimed")
	}
	if err != nil {
		return ClaimView{}, domain.WrapError(domain.CodeDependencyUnavailable, "claim API key", err)
	}
	if updated == 0 {
		exists, queryErr := s.db.APIKey.Query().Where(
			apikey.IDEQ(keyID),
			apikey.OwnerUserIDEQ(studentID),
		).Exist(ctx)
		if queryErr != nil {
			return ClaimView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query API key", queryErr)
		}
		if !exists {
			return ClaimView{}, domain.NewError(domain.CodeNotFound, "API key not found")
		}
		return ClaimView{}, domain.NewError(domain.CodeKeyAlreadyClaimed, "API key has already been claimed or is not approved")
	}
	s.publishKeyStatus(ctx, studentID, keyID, domain.KeyActive, claimedAt)
	return ClaimView{APIKey: secret, KeyPrefix: prefix, ClaimedAt: claimedAt}, nil
}
