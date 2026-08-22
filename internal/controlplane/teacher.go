package controlplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikey"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikeyaudit"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/mentorprojectapplication"
	entmodel "github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/model"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/modelbinding"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/organization"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/organizationmember"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/project"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/projectmember"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/provider"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/user"
)

type OrganizationInput struct {
	Name        string
	Description *string
	Status      *string
}

type ProjectInput struct {
	OrganizationID uuid.UUID
	Name           string
	Description    *string
	Status         *string
}

type ProviderInput struct {
	Name       string
	BaseURL    string
	Credential *string
	Status     *string
}

type ModelInput struct {
	ModelID            string
	DisplayName        string
	Description        *string
	Category           string
	Capabilities       []string
	InputModalities    []string
	OutputModalities   []string
	ContextWindow      *int
	ContextWindowSet   bool
	MaxOutputTokens    *int
	MaxOutputTokensSet bool
	IsCommon           *bool
	Status             *string
}

type MentorCandidateCursor struct {
	Name string    `json:"name"`
	ID   uuid.UUID `json:"id"`
}

func DecodeMentorCandidateCursor(value string) (*MentorCandidateCursor, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, domain.NewError(domain.CodeValidation, "invalid cursor")
	}
	var cursor MentorCandidateCursor
	if err := json.Unmarshal(raw, &cursor); err != nil || cursor.Name == "" || cursor.ID == uuid.Nil {
		return nil, domain.NewError(domain.CodeValidation, "invalid cursor")
	}
	return &cursor, nil
}

func encodeMentorCandidateCursor(name string, id uuid.UUID) string {
	raw, _ := json.Marshal(MentorCandidateCursor{Name: name, ID: id})
	return base64.RawURLEncoding.EncodeToString(raw)
}

type BindingInput struct {
	ProviderID        uuid.UUID
	UpstreamModelName string
	Adapter           string
	Priority          int
	Status            string
}

func (s *Service) TeacherOrganizations(ctx context.Context) ([]OrganizationView, error) {
	rows, err := s.db.Organization.Query().Order(ent.Asc(organization.FieldName)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list organizations", err)
	}
	result := make([]OrganizationView, 0, len(rows))
	for _, row := range rows {
		result = append(result, organizationView(row))
	}
	return result, nil
}

func (s *Service) MentorCandidates(
	ctx context.Context,
	organizationID uuid.UUID,
	search string,
	cursor *MentorCandidateCursor,
	limit int,
) ([]MentorCandidateView, *string, error) {
	if limit < 1 || limit > 50 {
		return nil, nil, domain.NewError(domain.CodeValidation, "limit must be between 1 and 50")
	}
	exists, err := s.db.Organization.Query().Where(organization.IDEQ(organizationID)).Exist(ctx)
	if err != nil {
		return nil, nil, domain.WrapError(domain.CodeDependencyUnavailable, "query organization", err)
	}
	if !exists {
		return nil, nil, domain.NewError(domain.CodeNotFound, "organization not found")
	}
	query := s.db.User.Query().Where(
		user.RoleEQ(user.RoleMentor),
		user.StatusEQ(user.StatusActive),
	).Order(ent.Asc(user.FieldName), ent.Asc(user.FieldID))
	if search = strings.TrimSpace(search); search != "" {
		query.Where(user.Or(user.NameContainsFold(search), user.EmailContainsFold(search)))
	}
	if cursor != nil {
		query.Where(user.Or(
			user.NameGT(cursor.Name),
			user.And(user.NameEQ(cursor.Name), user.IDGT(cursor.ID)),
		))
	}
	rows, err := query.Limit(limit + 1).All(ctx)
	if err != nil {
		return nil, nil, domain.WrapError(domain.CodeDependencyUnavailable, "list mentor candidates", err)
	}
	var nextCursor *string
	if len(rows) > limit {
		last := rows[limit-1]
		encoded := encodeMentorCandidateCursor(last.Name, last.ID)
		nextCursor = &encoded
		rows = rows[:limit]
	}
	result := make([]MentorCandidateView, 0, len(rows))
	for _, row := range rows {
		member, err := s.db.OrganizationMember.Query().Where(
			organizationmember.OrganizationIDEQ(organizationID),
			organizationmember.UserIDEQ(row.ID),
		).Exist(ctx)
		if err != nil {
			return nil, nil, domain.WrapError(domain.CodeDependencyUnavailable, "query mentor membership", err)
		}
		result = append(result, MentorCandidateView{
			ID: row.ID.String(), Name: row.Name, Email: row.Email, IsMember: member,
		})
	}
	return result, nextCursor, nil
}

func (s *Service) CreateOrganization(ctx context.Context, input OrganizationInput) (OrganizationView, error) {
	name, err := ValidateName(input.Name, 128)
	if err != nil {
		return OrganizationView{}, err
	}
	status, err := resourceStatus(input.Status, string(organization.StatusActive))
	if err != nil {
		return OrganizationView{}, err
	}
	builder := s.db.Organization.Create().SetName(name).SetStatus(organization.Status(status))
	if input.Description != nil && strings.TrimSpace(*input.Description) != "" {
		builder.SetDescription(strings.TrimSpace(*input.Description))
	}
	row, err := builder.Save(ctx)
	if ent.IsConstraintError(err) {
		return OrganizationView{}, domain.NewError(domain.CodeNameConflict, "organization name already exists")
	}
	if err != nil {
		return OrganizationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create organization", err)
	}
	return organizationView(row), nil
}

func (s *Service) UpdateOrganization(ctx context.Context, id uuid.UUID, input OrganizationInput) (OrganizationView, error) {
	builder := s.db.Organization.UpdateOneID(id)
	if input.Name != "" {
		name, err := ValidateName(input.Name, 128)
		if err != nil {
			return OrganizationView{}, err
		}
		builder.SetName(name)
	}
	if input.Description != nil {
		if strings.TrimSpace(*input.Description) == "" {
			builder.ClearDescription()
		} else if len([]rune(*input.Description)) > 1024 {
			return OrganizationView{}, domain.NewError(domain.CodeValidation, "description is too long")
		} else {
			builder.SetDescription(strings.TrimSpace(*input.Description))
		}
	}
	if input.Status != nil {
		status, err := resourceStatus(input.Status, "")
		if err != nil {
			return OrganizationView{}, err
		}
		builder.SetStatus(organization.Status(status))
	}
	row, err := builder.Save(ctx)
	if ent.IsNotFound(err) {
		return OrganizationView{}, domain.NewError(domain.CodeNotFound, "organization not found")
	}
	if ent.IsConstraintError(err) {
		return OrganizationView{}, domain.NewError(domain.CodeNameConflict, "organization name already exists")
	}
	if err != nil {
		return OrganizationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "update organization", err)
	}
	return organizationView(row), nil
}

func (s *Service) AssignMentorToOrganization(ctx context.Context, organizationID, mentorID uuid.UUID) error {
	orgExists, err := s.db.Organization.Query().Where(organization.IDEQ(organizationID)).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query organization", err)
	}
	if !orgExists {
		return domain.NewError(domain.CodeNotFound, "organization not found")
	}
	mentorExists, err := s.db.User.Query().Where(
		user.IDEQ(mentorID),
		user.RoleEQ(user.RoleMentor),
		user.StatusEQ(user.StatusActive),
	).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query mentor", err)
	}
	if !mentorExists {
		return domain.NewError(domain.CodeNotFound, "mentor not found")
	}
	exists, err := s.db.OrganizationMember.Query().Where(
		organizationmember.OrganizationIDEQ(organizationID),
		organizationmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query organization membership", err)
	}
	if exists {
		return nil
	}
	if _, err := s.db.OrganizationMember.Create().
		SetOrganizationID(organizationID).
		SetUserID(mentorID).
		Save(ctx); err != nil && !ent.IsConstraintError(err) {
		return domain.WrapError(domain.CodeDependencyUnavailable, "assign mentor to organization", err)
	}
	return nil
}

func (s *Service) TeacherProjects(ctx context.Context, organizationID *uuid.UUID, status *string) ([]ProjectView, error) {
	query := s.db.Project.Query().Order(ent.Asc(project.FieldName))
	if organizationID != nil {
		query.Where(project.OrganizationIDEQ(*organizationID))
	}
	if status != nil {
		parsed, err := resourceStatus(status, "")
		if err != nil {
			return nil, err
		}
		query.Where(project.StatusEQ(project.Status(parsed)))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list projects", err)
	}
	result := make([]ProjectView, 0, len(rows))
	for _, row := range rows {
		result = append(result, projectView(row, false))
	}
	return result, nil
}

func (s *Service) CreateProject(ctx context.Context, input ProjectInput) (ProjectView, error) {
	name, err := ValidateName(input.Name, 128)
	if err != nil {
		return ProjectView{}, err
	}
	orgExists, err := s.db.Organization.Query().Where(organization.IDEQ(input.OrganizationID)).Exist(ctx)
	if err != nil {
		return ProjectView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query organization", err)
	}
	if !orgExists {
		return ProjectView{}, domain.NewError(domain.CodeNotFound, "organization not found")
	}
	status, err := resourceStatus(input.Status, string(project.StatusActive))
	if err != nil {
		return ProjectView{}, err
	}
	builder := s.db.Project.Create().
		SetOrganizationID(input.OrganizationID).
		SetName(name).
		SetStatus(project.Status(status))
	if input.Description != nil && strings.TrimSpace(*input.Description) != "" {
		builder.SetDescription(strings.TrimSpace(*input.Description))
	}
	row, err := builder.Save(ctx)
	if ent.IsConstraintError(err) {
		return ProjectView{}, domain.NewError(domain.CodeNameConflict, "project name already exists in this organization")
	}
	if err != nil {
		return ProjectView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create project", err)
	}
	return projectView(row, false), nil
}

func (s *Service) UpdateProject(ctx context.Context, id uuid.UUID, input ProjectInput) (ProjectView, error) {
	builder := s.db.Project.UpdateOneID(id)
	if input.Name != "" {
		name, err := ValidateName(input.Name, 128)
		if err != nil {
			return ProjectView{}, err
		}
		builder.SetName(name)
	}
	if input.Description != nil {
		if strings.TrimSpace(*input.Description) == "" {
			builder.ClearDescription()
		} else if len([]rune(*input.Description)) > 1024 {
			return ProjectView{}, domain.NewError(domain.CodeValidation, "description is too long")
		} else {
			builder.SetDescription(strings.TrimSpace(*input.Description))
		}
	}
	if input.Status != nil {
		status, err := resourceStatus(input.Status, "")
		if err != nil {
			return ProjectView{}, err
		}
		builder.SetStatus(project.Status(status))
	}
	row, err := builder.Save(ctx)
	if ent.IsNotFound(err) {
		return ProjectView{}, domain.NewError(domain.CodeNotFound, "project not found")
	}
	if ent.IsConstraintError(err) {
		return ProjectView{}, domain.NewError(domain.CodeNameConflict, "project name already exists in this organization")
	}
	if err != nil {
		return ProjectView{}, domain.WrapError(domain.CodeDependencyUnavailable, "update project", err)
	}
	return projectView(row, false), nil
}

func (s *Service) TeacherMentorApplications(ctx context.Context, status *string) ([]MentorProjectApplicationView, error) {
	query := s.db.MentorProjectApplication.Query().Order(ent.Desc(mentorprojectapplication.FieldCreatedAt))
	if status != nil {
		parsed, err := mentorApplicationStatus(*status)
		if err != nil {
			return nil, err
		}
		query.Where(mentorprojectapplication.StatusEQ(parsed))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list mentor project applications", err)
	}
	result := make([]MentorProjectApplicationView, 0, len(rows))
	for _, row := range rows {
		item, err := s.mentorApplication(ctx, row.ID, uuid.Nil)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) ReviewMentorApplication(
	ctx context.Context,
	teacherID, applicationID uuid.UUID,
	approve bool,
	comment string,
) error {
	comment = strings.TrimSpace(comment)
	if !approve && comment == "" {
		return domain.NewError(domain.CodeValidation, "rejection reason is required")
	}
	if len([]rune(comment)) > 1000 {
		return domain.NewError(domain.CodeValidation, "comment is too long")
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "start project application transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	application, err := tx.MentorProjectApplication.Query().Where(
		mentorprojectapplication.IDEQ(applicationID),
		mentorprojectapplication.StatusEQ(mentorprojectapplication.StatusPending),
	).WithProject().Only(ctx)
	if ent.IsNotFound(err) {
		return domain.NewError(domain.CodeInvalidTransition, "project application is no longer pending")
	}
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query project application", err)
	}
	status := mentorprojectapplication.StatusRejected
	if approve {
		status = mentorprojectapplication.StatusApproved
		if application.Edges.Project == nil {
			return domain.NewError(domain.CodeNotFound, "project not found")
		}
		if err := ensureOrganizationMember(ctx, tx, application.Edges.Project.OrganizationID, application.MentorID); err != nil {
			return err
		}
		if err := ensureProjectMember(ctx, tx, application.ProjectID, application.MentorID); err != nil {
			return err
		}
	}
	now := s.now().UTC()
	update := tx.MentorProjectApplication.Update().Where(
		mentorprojectapplication.IDEQ(applicationID),
		mentorprojectapplication.StatusEQ(mentorprojectapplication.StatusPending),
	).SetStatus(status).SetReviewedBy(teacherID).SetReviewedAt(now)
	if comment != "" {
		update.SetReviewComment(comment)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "review project application", err)
	}
	if updated != 1 {
		return domain.NewError(domain.CodeInvalidTransition, "project application was reviewed concurrently")
	}
	if err := tx.Commit(); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "commit project application review", err)
	}
	return nil
}

func (s *Service) TeacherProviders(ctx context.Context) ([]ProviderView, error) {
	rows, err := s.db.Provider.Query().Order(ent.Asc(provider.FieldName)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list providers", err)
	}
	result := make([]ProviderView, 0, len(rows))
	for _, row := range rows {
		result = append(result, providerView(row))
	}
	return result, nil
}

func (s *Service) CreateProvider(ctx context.Context, input ProviderInput) (ProviderView, error) {
	name, err := ValidateName(input.Name, 128)
	if err != nil {
		return ProviderView{}, err
	}
	baseURL, err := ValidateBaseURL(input.BaseURL)
	if err != nil {
		return ProviderView{}, err
	}
	if input.Credential == nil {
		return ProviderView{}, domain.NewError(domain.CodeValidation, "credential is required")
	}
	ciphertext, err := s.security.EncryptCredential(*input.Credential)
	if err != nil {
		return ProviderView{}, domain.NewError(domain.CodeValidation, err.Error())
	}
	status, err := resourceStatus(input.Status, string(provider.StatusActive))
	if err != nil {
		return ProviderView{}, err
	}
	row, err := s.db.Provider.Create().
		SetName(name).
		SetBaseURL(baseURL).
		SetCredentialCiphertext(ciphertext).
		SetStatus(provider.Status(status)).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return ProviderView{}, domain.NewError(domain.CodeNameConflict, "provider name already exists")
	}
	if err != nil {
		return ProviderView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create provider", err)
	}
	return providerView(row), nil
}

func (s *Service) TeacherProvider(ctx context.Context, id uuid.UUID) (ProviderView, error) {
	row, err := s.db.Provider.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ProviderView{}, domain.NewError(domain.CodeNotFound, "provider not found")
	}
	if err != nil {
		return ProviderView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query provider", err)
	}
	return providerView(row), nil
}

func (s *Service) UpdateProvider(ctx context.Context, id uuid.UUID, input ProviderInput) (ProviderView, error) {
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return ProviderView{}, domain.WrapError(domain.CodeDependencyUnavailable, "start provider update transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.Provider.Query().Where(provider.IDEQ(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return ProviderView{}, domain.NewError(domain.CodeNotFound, "provider not found")
	}
	if err != nil {
		return ProviderView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query provider", err)
	}
	if input.Status != nil && *input.Status == string(provider.StatusInactive) && current.Status == provider.StatusActive {
		if err := ensureProviderCanBeDisabledTx(ctx, tx, id); err != nil {
			return ProviderView{}, err
		}
	}
	builder := current.Update()
	if input.Name != "" {
		name, err := ValidateName(input.Name, 128)
		if err != nil {
			return ProviderView{}, err
		}
		builder.SetName(name)
	}
	if input.BaseURL != "" {
		baseURL, err := ValidateBaseURL(input.BaseURL)
		if err != nil {
			return ProviderView{}, err
		}
		builder.SetBaseURL(baseURL)
	}
	if input.Credential != nil {
		ciphertext, err := s.security.EncryptCredential(*input.Credential)
		if err != nil {
			return ProviderView{}, domain.NewError(domain.CodeValidation, err.Error())
		}
		builder.SetCredentialCiphertext(ciphertext)
	}
	if input.Status != nil {
		status, err := resourceStatus(input.Status, "")
		if err != nil {
			return ProviderView{}, err
		}
		builder.SetStatus(provider.Status(status))
	}
	row, err := builder.Save(ctx)
	if ent.IsNotFound(err) {
		return ProviderView{}, domain.NewError(domain.CodeNotFound, "provider not found")
	}
	if ent.IsConstraintError(err) {
		return ProviderView{}, domain.NewError(domain.CodeNameConflict, "provider name already exists")
	}
	if err != nil {
		return ProviderView{}, domain.WrapError(domain.CodeDependencyUnavailable, "update provider", err)
	}
	if err := tx.Commit(); err != nil {
		return ProviderView{}, domain.WrapError(domain.CodeDependencyUnavailable, "commit provider update", err)
	}
	return providerView(row), nil
}

func (s *Service) TeacherModels(ctx context.Context) ([]ModelView, error) {
	rows, err := s.db.Model.Query().Order(ent.Asc(entmodel.FieldModelID)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list models", err)
	}
	result := make([]ModelView, 0, len(rows))
	for _, row := range rows {
		item := modelView(row)
		ready, err := s.modelHasActiveRoute(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		item.RouteReady = ready
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) CreateModel(ctx context.Context, input ModelInput) (ModelView, error) {
	modelID, err := normalizeSingleModelID(input.ModelID)
	if err != nil {
		return ModelView{}, err
	}
	displayName, err := ValidateName(input.DisplayName, 256)
	if err != nil {
		return ModelView{}, err
	}
	category, err := modelCategory(input.Category)
	if err != nil {
		return ModelView{}, err
	}
	status := entmodel.StatusPendingConfiguration
	if input.Status != nil {
		status, err = modelStatus(*input.Status)
		if err != nil {
			return ModelView{}, err
		}
		if status == entmodel.StatusActive {
			return ModelView{}, domain.NewError(domain.CodeModelNotReady, "create a binding before activating the model")
		}
	}
	builder := s.db.Model.Create().
		SetModelID(modelID).
		SetDisplayName(displayName).
		SetCategory(category).
		SetCapabilities(cleanStringList(input.Capabilities)).
		SetInputModalities(cleanStringList(input.InputModalities)).
		SetOutputModalities(cleanStringList(input.OutputModalities)).
		SetStatus(status)
	if input.Description != nil && strings.TrimSpace(*input.Description) != "" {
		builder.SetDescription(strings.TrimSpace(*input.Description))
	}
	if input.ContextWindow != nil {
		builder.SetContextWindow(*input.ContextWindow)
	}
	if input.MaxOutputTokens != nil {
		builder.SetMaxOutputTokens(*input.MaxOutputTokens)
	}
	if input.IsCommon != nil {
		builder.SetIsCommon(*input.IsCommon)
	}
	row, err := builder.Save(ctx)
	if ent.IsConstraintError(err) {
		return ModelView{}, domain.NewError(domain.CodeNameConflict, "model ID already exists")
	}
	if err != nil {
		return ModelView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create model", err)
	}
	if row.IsCommon && row.Status == entmodel.StatusActive {
		_ = s.cache.PublishGlobalEvent(ctx, "models.common_changed", map[string]any{"revision": uuid.Must(uuid.NewV7()).String()})
	}
	return modelView(row), nil
}

func (s *Service) TeacherModel(ctx context.Context, id uuid.UUID) (ModelView, []BindingView, error) {
	row, err := s.db.Model.Get(ctx, id)
	if ent.IsNotFound(err) {
		return ModelView{}, nil, domain.NewError(domain.CodeNotFound, "model not found")
	}
	if err != nil {
		return ModelView{}, nil, domain.WrapError(domain.CodeDependencyUnavailable, "query model", err)
	}
	bindings, err := s.db.ModelBinding.Query().Where(
		modelbinding.ModelIDEQ(id),
	).Order(ent.Asc(modelbinding.FieldPriority), ent.Asc(modelbinding.FieldID)).All(ctx)
	if err != nil {
		return ModelView{}, nil, domain.WrapError(domain.CodeDependencyUnavailable, "query model bindings", err)
	}
	result := make([]BindingView, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, bindingView(binding))
	}
	view := modelView(row)
	view.RouteReady, err = s.modelHasActiveRoute(ctx, id)
	if err != nil {
		return ModelView{}, nil, err
	}
	return view, result, nil
}

func (s *Service) UpdateModel(ctx context.Context, id uuid.UUID, input ModelInput) (ModelView, error) {
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return ModelView{}, domain.WrapError(domain.CodeDependencyUnavailable, "start model update transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.Model.Query().Where(entmodel.IDEQ(id)).Only(ctx)
	if ent.IsNotFound(err) {
		return ModelView{}, domain.NewError(domain.CodeNotFound, "model not found")
	}
	if err != nil {
		return ModelView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query model", err)
	}
	builder := current.Update()
	if input.DisplayName != "" {
		name, err := ValidateName(input.DisplayName, 256)
		if err != nil {
			return ModelView{}, err
		}
		builder.SetDisplayName(name)
	}
	if input.Description != nil {
		if strings.TrimSpace(*input.Description) == "" {
			builder.ClearDescription()
		} else {
			builder.SetDescription(strings.TrimSpace(*input.Description))
		}
	}
	if input.Category != "" {
		category, err := modelCategory(input.Category)
		if err != nil {
			return ModelView{}, err
		}
		builder.SetCategory(category)
	}
	if input.Capabilities != nil {
		builder.SetCapabilities(cleanStringList(input.Capabilities))
	}
	if input.InputModalities != nil {
		builder.SetInputModalities(cleanStringList(input.InputModalities))
	}
	if input.OutputModalities != nil {
		builder.SetOutputModalities(cleanStringList(input.OutputModalities))
	}
	if input.ContextWindowSet {
		if input.ContextWindow == nil {
			builder.ClearContextWindow()
		} else {
			if *input.ContextWindow <= 0 {
				return ModelView{}, domain.NewError(domain.CodeValidation, "context_window must be positive")
			}
			builder.SetContextWindow(*input.ContextWindow)
		}
	}
	if input.MaxOutputTokensSet {
		if input.MaxOutputTokens == nil {
			builder.ClearMaxOutputTokens()
		} else {
			if *input.MaxOutputTokens <= 0 {
				return ModelView{}, domain.NewError(domain.CodeValidation, "max_output_tokens must be positive")
			}
			builder.SetMaxOutputTokens(*input.MaxOutputTokens)
		}
	}
	if input.IsCommon != nil {
		builder.SetIsCommon(*input.IsCommon)
	}
	if input.Status != nil {
		status, err := modelStatus(*input.Status)
		if err != nil {
			return ModelView{}, err
		}
		if status == entmodel.StatusActive {
			ready, err := modelHasActiveRouteTx(ctx, tx, id)
			if err != nil {
				return ModelView{}, err
			}
			if !ready {
				appErr := domain.NewError(domain.CodeModelRoutingRequired, "model requires an active binding and provider")
				appErr.Details = map[string]any{"model_ids": []string{current.ModelID}}
				return ModelView{}, appErr
			}
		}
		builder.SetStatus(status)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return ModelView{}, domain.WrapError(domain.CodeDependencyUnavailable, "update model", err)
	}
	commonWasVisible := current.IsCommon && current.Status == entmodel.StatusActive
	commonIsVisible := row.IsCommon && row.Status == entmodel.StatusActive
	if err := tx.Commit(); err != nil {
		return ModelView{}, domain.WrapError(domain.CodeDependencyUnavailable, "commit model update", err)
	}
	if commonWasVisible != commonIsVisible {
		_ = s.cache.PublishGlobalEvent(ctx, "models.common_changed", map[string]any{"revision": uuid.Must(uuid.NewV7()).String()})
	}
	return modelView(row), nil
}

func (s *Service) CreateBinding(ctx context.Context, modelID uuid.UUID, input BindingInput) (BindingView, error) {
	if _, err := s.db.Model.Get(ctx, modelID); ent.IsNotFound(err) {
		return BindingView{}, domain.NewError(domain.CodeNotFound, "model not found")
	} else if err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query model", err)
	}
	if err := validateBindingInput(input); err != nil {
		return BindingView{}, err
	}
	providerExists, err := s.db.Provider.Query().Where(provider.IDEQ(input.ProviderID)).Exist(ctx)
	if err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query provider", err)
	}
	if !providerExists {
		return BindingView{}, domain.NewError(domain.CodeNotFound, "provider not found")
	}
	row, err := s.db.ModelBinding.Create().
		SetModelID(modelID).
		SetProviderID(input.ProviderID).
		SetUpstreamModelName(strings.TrimSpace(input.UpstreamModelName)).
		SetAdapter(modelbinding.Adapter(input.Adapter)).
		SetPriority(input.Priority).
		SetStatus(modelbinding.Status(input.Status)).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return BindingView{}, domain.NewError(domain.CodeNameConflict, "model binding already exists")
	}
	if err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create model binding", err)
	}
	return bindingView(row), nil
}

func (s *Service) UpdateBinding(ctx context.Context, bindingID uuid.UUID, input BindingInput) (BindingView, error) {
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "start binding update transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	current, err := tx.ModelBinding.Query().Where(modelbinding.IDEQ(bindingID)).Only(ctx)
	if ent.IsNotFound(err) {
		return BindingView{}, domain.NewError(domain.CodeNotFound, "model binding not found")
	}
	if err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query model binding", err)
	}
	if input.Status == string(modelbinding.StatusInactive) && current.Status == modelbinding.StatusActive {
		if err := ensureBindingCanBeDisabledTx(ctx, tx, current); err != nil {
			return BindingView{}, err
		}
	}
	builder := current.Update()
	if input.UpstreamModelName != "" {
		builder.SetUpstreamModelName(strings.TrimSpace(input.UpstreamModelName))
	}
	if input.Adapter != "" {
		if !validBindingAdapter(input.Adapter) {
			return BindingView{}, domain.NewError(domain.CodeValidation, "invalid binding adapter")
		}
		builder.SetAdapter(modelbinding.Adapter(input.Adapter))
	}
	if input.Priority >= 0 {
		builder.SetPriority(input.Priority)
	}
	if input.Status != "" {
		if input.Status != string(modelbinding.StatusActive) && input.Status != string(modelbinding.StatusInactive) {
			return BindingView{}, domain.NewError(domain.CodeValidation, "invalid binding status")
		}
		builder.SetStatus(modelbinding.Status(input.Status))
	}
	row, err := builder.Save(ctx)
	if ent.IsConstraintError(err) {
		return BindingView{}, domain.NewError(domain.CodeNameConflict, "model binding already exists")
	}
	if err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "update model binding", err)
	}
	if err := tx.Commit(); err != nil {
		return BindingView{}, domain.WrapError(domain.CodeDependencyUnavailable, "commit binding update", err)
	}
	return bindingView(row), nil
}

func ensureProviderCanBeDisabledTx(ctx context.Context, tx *ent.Tx, providerID uuid.UUID) error {
	bindings, err := tx.ModelBinding.Query().Where(
		modelbinding.ProviderIDEQ(providerID),
		modelbinding.StatusEQ(modelbinding.StatusActive),
	).All(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query provider bindings", err)
	}
	modelIDs := make([]uuid.UUID, 0, len(bindings))
	seen := make(map[uuid.UUID]struct{}, len(bindings))
	for _, binding := range bindings {
		if _, exists := seen[binding.ModelID]; !exists {
			seen[binding.ModelID] = struct{}{}
			modelIDs = append(modelIDs, binding.ModelID)
		}
	}
	if len(modelIDs) == 0 {
		return nil
	}
	models, err := tx.Model.Query().Where(
		entmodel.IDIn(modelIDs...),
		entmodel.StatusEQ(entmodel.StatusActive),
	).All(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "lock provider models", err)
	}
	affected := make([]string, 0)
	for _, row := range models {
		ready, err := modelHasActiveRouteExcluding(ctx, tx, row.ID, providerID, uuid.Nil)
		if err != nil {
			return err
		}
		if !ready {
			affected = append(affected, row.ModelID)
		}
	}
	if len(affected) > 0 {
		appErr := domain.NewError(domain.CodeModelRoutingRequired, "provider is required by active models")
		appErr.Details = map[string]any{"model_ids": affected}
		return appErr
	}
	return nil
}

func ensureBindingCanBeDisabledTx(ctx context.Context, tx *ent.Tx, binding *ent.ModelBinding) error {
	modelRow, err := tx.Model.Query().Where(entmodel.IDEQ(binding.ModelID)).Only(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "lock binding model", err)
	}
	if modelRow.Status != entmodel.StatusActive {
		return nil
	}
	ready, err := modelHasActiveRouteExcluding(ctx, tx, binding.ModelID, uuid.Nil, binding.ID)
	if err != nil {
		return err
	}
	if !ready {
		appErr := domain.NewError(domain.CodeModelRoutingRequired, "binding is required by an active model")
		appErr.Details = map[string]any{"model_ids": []string{modelRow.ModelID}}
		return appErr
	}
	return nil
}

func modelHasActiveRouteExcluding(
	ctx context.Context,
	tx *ent.Tx,
	modelID, excludedProviderID, excludedBindingID uuid.UUID,
) (bool, error) {
	query := tx.ModelBinding.Query().Where(
		modelbinding.ModelIDEQ(modelID),
		modelbinding.StatusEQ(modelbinding.StatusActive),
		modelbinding.HasProviderWith(provider.StatusEQ(provider.StatusActive)),
	)
	if excludedProviderID != uuid.Nil {
		query.Where(modelbinding.ProviderIDNEQ(excludedProviderID))
	}
	if excludedBindingID != uuid.Nil {
		query.Where(modelbinding.IDNEQ(excludedBindingID))
	}
	ready, err := query.Exist(ctx)
	if err != nil {
		return false, domain.WrapError(domain.CodeDependencyUnavailable, "query alternate model routes", err)
	}
	return ready, nil
}

func (s *Service) TeacherKeyReviews(ctx context.Context) ([]APIKeyView, error) {
	rows, err := s.db.APIKey.Query().Where(
		apikey.StatusEQ(apikey.StatusPendingTeacher),
	).Order(ent.Asc(apikey.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list teacher key reviews", err)
	}
	views, err := s.keyViews(ctx, rows)
	if err != nil {
		return nil, err
	}
	for i := range views {
		if err := s.populateRouteReadiness(ctx, &views[i]); err != nil {
			return nil, err
		}
	}
	return views, nil
}

func (s *Service) TeacherKeyReview(ctx context.Context, keyID uuid.UUID) (APIKeyView, error) {
	view, err := s.keyView(ctx, s.db.APIKey.Query().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusPendingTeacher),
	))
	if err != nil {
		return APIKeyView{}, err
	}
	if err := s.populateRouteReadiness(ctx, &view); err != nil {
		return APIKeyView{}, err
	}
	return view, nil
}

func (s *Service) populateRouteReadiness(ctx context.Context, view *APIKeyView) error {
	for i := range view.Models {
		modelID, err := uuid.Parse(view.Models[i].ID)
		if err != nil {
			return domain.NewError(domain.CodeDependencyUnavailable, "invalid model identity")
		}
		ready, err := s.modelHasActiveRoute(ctx, modelID)
		if err != nil {
			return err
		}
		view.Models[i].RouteReady = ready
	}
	return nil
}

func (s *Service) ReviewKeyAsTeacher(ctx context.Context, teacherID, keyID uuid.UUID, approve bool, comment string) error {
	action := "teacher_approved"
	nextStatus := apikey.StatusApproved
	auditAction := apikeyaudit.ActionTeacherApproved
	if !approve {
		action = "teacher_rejected"
		nextStatus = apikey.StatusRejected
		auditAction = apikeyaudit.ActionTeacherRejected
	}
	comment, err := ValidateComment(action, comment)
	if err != nil {
		return err
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "start teacher review transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	key, err := tx.APIKey.Query().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusPendingTeacher),
	).WithProject().WithModels(func(query *ent.APIKeyModelQuery) {
		query.WithModel()
	}).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.NewError(domain.CodeInvalidTransition, "API key is no longer pending teacher review")
	}
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query API key", err)
	}
	if approve {
		if len(key.Edges.Models) == 0 {
			return domain.NewError(domain.CodeModelNotReady, "API key has no model whitelist")
		}
		notReady := make([]string, 0)
		for _, link := range key.Edges.Models {
			if link.Edges.Model == nil || link.Edges.Model.Status != entmodel.StatusActive {
				if link.Edges.Model != nil {
					notReady = append(notReady, link.Edges.Model.ModelID)
				}
				continue
			}
			ready, err := modelHasActiveRouteTx(ctx, tx, link.ModelID)
			if err != nil {
				return err
			}
			if !ready {
				notReady = append(notReady, link.Edges.Model.ModelID)
			}
		}
		if len(notReady) > 0 {
			appErr := domain.NewError(domain.CodeModelNotReady, "one or more models are not ready")
			appErr.Details = map[string]any{"model_ids": notReady}
			return appErr
		}
		if key.Edges.Project == nil {
			return domain.NewError(domain.CodeNotFound, "project not found")
		}
		if err := ensureOrganizationMember(ctx, tx, key.Edges.Project.OrganizationID, key.OwnerUserID); err != nil {
			return err
		}
		if err := ensureProjectMember(ctx, tx, key.ProjectID, key.OwnerUserID); err != nil {
			return err
		}
	}
	updated, err := tx.APIKey.Update().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusPendingTeacher),
	).SetStatus(nextStatus).Save(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "update teacher key review", err)
	}
	if updated != 1 {
		return domain.NewError(domain.CodeInvalidTransition, "API key was reviewed concurrently")
	}
	audit := tx.APIKeyAudit.Create().
		SetAPIKeyID(keyID).
		SetActorUserID(teacherID).
		SetAction(auditAction)
	if comment != "" {
		audit.SetComment(comment)
	}
	if _, err := audit.Save(ctx); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "create teacher key audit", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "commit teacher key review", err)
	}
	s.publishKeyStatus(ctx, key.OwnerUserID, keyID, string(nextStatus), s.now().UTC())
	return nil
}

func ensureOrganizationMember(ctx context.Context, tx *ent.Tx, organizationID, userID uuid.UUID) error {
	exists, err := tx.OrganizationMember.Query().Where(
		organizationmember.OrganizationIDEQ(organizationID),
		organizationmember.UserIDEQ(userID),
	).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query organization membership", err)
	}
	if exists {
		return nil
	}
	if _, err := tx.OrganizationMember.Create().
		SetOrganizationID(organizationID).
		SetUserID(userID).
		Save(ctx); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "create organization membership", err)
	}
	return nil
}

func ensureProjectMember(ctx context.Context, tx *ent.Tx, projectID, userID uuid.UUID) error {
	exists, err := tx.ProjectMember.Query().Where(
		projectmember.ProjectIDEQ(projectID),
		projectmember.UserIDEQ(userID),
	).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query project membership", err)
	}
	if exists {
		return nil
	}
	if _, err := tx.ProjectMember.Create().
		SetProjectID(projectID).
		SetUserID(userID).
		Save(ctx); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "create project membership", err)
	}
	return nil
}

func (s *Service) modelHasActiveRoute(ctx context.Context, modelID uuid.UUID) (bool, error) {
	exists, err := s.db.ModelBinding.Query().Where(
		modelbinding.ModelIDEQ(modelID),
		modelbinding.StatusEQ(modelbinding.StatusActive),
		modelbinding.HasProviderWith(provider.StatusEQ(provider.StatusActive)),
	).Exist(ctx)
	if err != nil {
		return false, domain.WrapError(domain.CodeDependencyUnavailable, "query model route", err)
	}
	return exists, nil
}

func modelHasActiveRouteTx(ctx context.Context, tx *ent.Tx, modelID uuid.UUID) (bool, error) {
	exists, err := tx.ModelBinding.Query().Where(
		modelbinding.ModelIDEQ(modelID),
		modelbinding.StatusEQ(modelbinding.StatusActive),
		modelbinding.HasProviderWith(provider.StatusEQ(provider.StatusActive)),
	).Exist(ctx)
	if err != nil {
		return false, domain.WrapError(domain.CodeDependencyUnavailable, "query model route", err)
	}
	return exists, nil
}

func providerView(row *ent.Provider) ProviderView {
	return ProviderView{
		ID: row.ID.String(), Name: row.Name, BaseURL: row.BaseURL,
		CredentialConfigured: len(row.CredentialCiphertext) > 0,
		Status:               string(row.Status), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func bindingView(row *ent.ModelBinding) BindingView {
	return BindingView{
		ID: row.ID.String(), ModelID: row.ModelID.String(), ProviderID: row.ProviderID.String(),
		UpstreamModelName: row.UpstreamModelName, Adapter: string(row.Adapter),
		Priority: row.Priority, Status: string(row.Status),
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

func resourceStatus(value *string, fallback string) (string, error) {
	if value == nil {
		return fallback, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(*value))
	if normalized != "active" && normalized != "inactive" {
		return "", domain.NewError(domain.CodeValidation, "invalid resource status")
	}
	return normalized, nil
}

func mentorApplicationStatus(value string) (mentorprojectapplication.Status, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(mentorprojectapplication.StatusPending):
		return mentorprojectapplication.StatusPending, nil
	case string(mentorprojectapplication.StatusApproved):
		return mentorprojectapplication.StatusApproved, nil
	case string(mentorprojectapplication.StatusRejected):
		return mentorprojectapplication.StatusRejected, nil
	case string(mentorprojectapplication.StatusCancelled):
		return mentorprojectapplication.StatusCancelled, nil
	default:
		return "", domain.NewError(domain.CodeValidation, "invalid project application status")
	}
}

func modelCategory(value string) (entmodel.Category, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(entmodel.CategoryText):
		return entmodel.CategoryText, nil
	case string(entmodel.CategoryImage):
		return entmodel.CategoryImage, nil
	case string(entmodel.CategoryAudio):
		return entmodel.CategoryAudio, nil
	case string(entmodel.CategoryVideo):
		return entmodel.CategoryVideo, nil
	case string(entmodel.CategoryMultimodal):
		return entmodel.CategoryMultimodal, nil
	case string(entmodel.CategoryEmbedding):
		return entmodel.CategoryEmbedding, nil
	case string(entmodel.CategoryRerank):
		return entmodel.CategoryRerank, nil
	default:
		return "", domain.NewError(domain.CodeValidation, "invalid model category")
	}
}

func modelStatus(value string) (entmodel.Status, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(entmodel.StatusPendingConfiguration):
		return entmodel.StatusPendingConfiguration, nil
	case string(entmodel.StatusActive):
		return entmodel.StatusActive, nil
	case string(entmodel.StatusInactive):
		return entmodel.StatusInactive, nil
	default:
		return "", domain.NewError(domain.CodeValidation, "invalid model status")
	}
}

func validateBindingInput(input BindingInput) error {
	if input.ProviderID == uuid.Nil {
		return domain.NewError(domain.CodeValidation, "provider_id is required")
	}
	if strings.TrimSpace(input.UpstreamModelName) == "" || len(input.UpstreamModelName) > 256 {
		return domain.NewError(domain.CodeValidation, "invalid upstream_model_name")
	}
	if !validBindingAdapter(input.Adapter) {
		return domain.NewError(domain.CodeValidation, "invalid binding adapter")
	}
	if input.Priority < 0 {
		return domain.NewError(domain.CodeValidation, "priority must be non-negative")
	}
	if input.Status != string(modelbinding.StatusActive) && input.Status != string(modelbinding.StatusInactive) {
		return domain.NewError(domain.CodeValidation, "invalid binding status")
	}
	return nil
}

func validBindingAdapter(value string) bool {
	switch modelbinding.Adapter(value) {
	case modelbinding.AdapterOpenaiCompatible,
		modelbinding.AdapterOpenaiResponses,
		modelbinding.AdapterOpenaiEmbeddings,
		modelbinding.AdapterOpenaiImages,
		modelbinding.AdapterOpenaiAudio,
		modelbinding.AdapterOpenaiVideo,
		modelbinding.AdapterOpenaiRealtime,
		modelbinding.AdapterOpenaiModerations,
		modelbinding.AdapterAnthropic,
		modelbinding.AdapterCohereRerankV2,
		modelbinding.AdapterGoogleGeminiV1beta:
		return true
	default:
		return false
	}
}

func normalizeSingleModelID(value string) (string, error) {
	values, err := normalizeModelIDs([]string{value})
	if err != nil {
		return "", err
	}
	return values[0], nil
}

func cleanStringList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
