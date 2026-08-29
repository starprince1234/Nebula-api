package controlplane

import (
	"context"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikey"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikeyaudit"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/mentorprojectapplication"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/organization"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/organizationmember"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/project"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/projectmember"
)

func (s *Service) MentorOrganizations(ctx context.Context, mentorID uuid.UUID) ([]OrganizationView, error) {
	rows, err := s.db.Organization.Query().Where(
		organization.StatusEQ(organization.StatusActive),
		organization.HasMembershipsWith(organizationmember.UserIDEQ(mentorID)),
	).Order(ent.Asc(organization.FieldName)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list mentor organizations", err)
	}
	result := make([]OrganizationView, 0, len(rows))
	for _, row := range rows {
		result = append(result, organizationView(row))
	}
	return result, nil
}

func (s *Service) MentorProjects(ctx context.Context, mentorID, organizationID uuid.UUID) ([]ProjectView, error) {
	member, err := s.db.OrganizationMember.Query().Where(
		organizationmember.OrganizationIDEQ(organizationID),
		organizationmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query organization membership", err)
	}
	if !member {
		return nil, domain.NewError(domain.CodeNotFound, "organization not found")
	}
	rows, err := s.db.Project.Query().Where(
		project.OrganizationIDEQ(organizationID),
		project.StatusEQ(project.StatusActive),
	).Order(ent.Asc(project.FieldName)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list organization projects", err)
	}
	projectIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		projectIDs = append(projectIDs, row.ID)
	}
	responsible := make(map[uuid.UUID]bool)
	applicationStatus := make(map[uuid.UUID]string)
	if len(projectIDs) > 0 {
		memberships, err := s.db.ProjectMember.Query().Where(
			projectmember.ProjectIDIn(projectIDs...),
			projectmember.UserIDEQ(mentorID),
		).All(ctx)
		if err != nil {
			return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query mentor projects", err)
		}
		for _, membership := range memberships {
			responsible[membership.ProjectID] = true
		}
		applications, err := s.db.MentorProjectApplication.Query().Where(
			mentorprojectapplication.ProjectIDIn(projectIDs...),
			mentorprojectapplication.MentorIDEQ(mentorID),
		).Order(ent.Desc(mentorprojectapplication.FieldCreatedAt)).All(ctx)
		if err != nil {
			return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query project applications", err)
		}
		for _, application := range applications {
			if _, exists := applicationStatus[application.ProjectID]; !exists {
				applicationStatus[application.ProjectID] = string(application.Status)
			}
		}
	}
	result := make([]ProjectView, 0, len(rows))
	for _, row := range rows {
		item := projectView(row, responsible[row.ID])
		item.IsResponsible = responsible[row.ID]
		if status, exists := applicationStatus[row.ID]; exists {
			item.ApplicationStatus = &status
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) ApplyMentorProject(ctx context.Context, mentorID, projectID uuid.UUID) (MentorProjectApplicationView, error) {
	target, err := s.db.Project.Query().Where(
		project.IDEQ(projectID),
		project.StatusEQ(project.StatusActive),
	).WithOrganization().Only(ctx)
	if ent.IsNotFound(err) {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeNotFound, "project not found")
	}
	if err != nil {
		return MentorProjectApplicationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query project", err)
	}
	if target.Edges.Organization == nil || target.Edges.Organization.Status != organization.StatusActive {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeNotFound, "project not found")
	}
	isOrganizationMember, err := s.db.OrganizationMember.Query().Where(
		organizationmember.OrganizationIDEQ(target.OrganizationID),
		organizationmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return MentorProjectApplicationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query mentor organization", err)
	}
	if !isOrganizationMember {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeForbidden, "mentor does not belong to the project organization")
	}
	isProjectMember, err := s.db.ProjectMember.Query().Where(
		projectmember.ProjectIDEQ(projectID),
		projectmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return MentorProjectApplicationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query project membership", err)
	}
	if isProjectMember {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeInvalidTransition, "mentor already manages this project")
	}
	pending, err := s.db.MentorProjectApplication.Query().Where(
		mentorprojectapplication.ProjectIDEQ(projectID),
		mentorprojectapplication.MentorIDEQ(mentorID),
		mentorprojectapplication.StatusEQ(mentorprojectapplication.StatusPending),
	).Exist(ctx)
	if err != nil {
		return MentorProjectApplicationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query pending project application", err)
	}
	if pending {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeInvalidTransition, "project application is already pending")
	}
	application, err := s.db.MentorProjectApplication.Create().
		SetProjectID(projectID).
		SetMentorID(mentorID).
		SetStatus(mentorprojectapplication.StatusPending).
		Save(ctx)
	if ent.IsConstraintError(err) {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeInvalidTransition, "project application is already pending")
	}
	if err != nil {
		return MentorProjectApplicationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "create project application", err)
	}
	return s.mentorApplication(ctx, application.ID, mentorID)
}

func (s *Service) MentorProjectApplications(ctx context.Context, mentorID uuid.UUID) ([]MentorProjectApplicationView, error) {
	rows, err := s.db.MentorProjectApplication.Query().Where(
		mentorprojectapplication.MentorIDEQ(mentorID),
	).Order(ent.Desc(mentorprojectapplication.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list project applications", err)
	}
	result := make([]MentorProjectApplicationView, 0, len(rows))
	for _, row := range rows {
		item, err := s.mentorApplication(ctx, row.ID, mentorID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) MentorKeyReviews(ctx context.Context, mentorID uuid.UUID) ([]APIKeyView, error) {
	projectIDs, err := s.mentorProjectIDs(ctx, mentorID)
	if err != nil {
		return nil, err
	}
	if len(projectIDs) == 0 {
		return []APIKeyView{}, nil
	}
	rows, err := s.db.APIKey.Query().Where(
		apikey.ProjectIDIn(projectIDs...),
		apikey.StatusEQ(apikey.StatusPendingMentor),
	).Order(ent.Asc(apikey.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list mentor key reviews", err)
	}
	return s.keyViews(ctx, rows)
}

func (s *Service) MentorKeyReview(ctx context.Context, mentorID, keyID uuid.UUID) (APIKeyView, error) {
	projectIDs, err := s.mentorProjectIDs(ctx, mentorID)
	if err != nil {
		return APIKeyView{}, err
	}
	if len(projectIDs) == 0 {
		return APIKeyView{}, domain.NewError(domain.CodeNotFound, "API key application not found")
	}
	return s.keyView(ctx, s.db.APIKey.Query().Where(
		apikey.IDEQ(keyID),
		apikey.ProjectIDIn(projectIDs...),
		apikey.StatusEQ(apikey.StatusPendingMentor),
	))
}

func (s *Service) ReviewKeyAsMentor(ctx context.Context, mentorID, keyID uuid.UUID, approve bool, comment string, monthly ...int64) error {
	action := "mentor_approved"
	nextStatus := apikey.StatusPendingTeacher
	auditAction := apikeyaudit.ActionMentorApproved
	if !approve {
		action = "mentor_rejected"
		nextStatus = apikey.StatusRejected
		auditAction = apikeyaudit.ActionMentorRejected
	}
	comment, err := ValidateComment(action, comment)
	if err != nil {
		return err
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "start mentor review transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	key, err := tx.APIKey.Query().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusPendingMentor),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.NewError(domain.CodeInvalidTransition, "API key is no longer pending mentor review")
	}
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query API key", err)
	}
	if approve && len(monthly) > 0 {
		if monthly[0] < 0 {
			return domain.NewError(domain.CodeValidation, "monthly credits must be non-negative")
		}
	}
	responsible, err := tx.ProjectMember.Query().Where(
		projectmember.ProjectIDEQ(key.ProjectID),
		projectmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query mentor responsibility", err)
	}
	if !responsible {
		return domain.NewError(domain.CodeNotFound, "API key application not found")
	}
	if approve && len(monthly) > 0 {
		if monthly[0] < 0 || monthly[0] > 1_000_000_000_000 {
			return domain.NewError(domain.CodeValidation, "monthly credits are out of range")
		}
		if _, err := tx.APIKey.UpdateOneID(keyID).SetMentorMonthlyCreditQuotaMilli(monthly[0]).Save(ctx); err != nil {
			return domain.WrapError(domain.CodeDependencyUnavailable, "set mentor quota", err)
		}
	}
	updated, err := tx.APIKey.Update().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusPendingMentor),
	).SetStatus(nextStatus).Save(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "update API key review", err)
	}
	if updated != 1 {
		return domain.NewError(domain.CodeInvalidTransition, "API key was reviewed concurrently")
	}
	audit := tx.APIKeyAudit.Create().
		SetAPIKeyID(keyID).
		SetActorUserID(mentorID).
		SetAction(auditAction)
	if comment != "" {
		audit.SetComment(comment)
	}
	if _, err := audit.Save(ctx); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "create key audit", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "commit mentor review", err)
	}
	s.publishKeyStatus(ctx, key.OwnerUserID, keyID, string(nextStatus), s.now().UTC())
	return nil
}

func (s *Service) MentorActiveKeys(ctx context.Context, mentorID, projectID uuid.UUID) ([]APIKeyView, error) {
	responsible, err := s.db.ProjectMember.Query().Where(
		projectmember.ProjectIDEQ(projectID),
		projectmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query mentor responsibility", err)
	}
	if !responsible {
		return nil, domain.NewError(domain.CodeNotFound, "project not found")
	}
	rows, err := s.db.APIKey.Query().Where(
		apikey.ProjectIDEQ(projectID),
		apikey.StatusEQ(apikey.StatusActive),
	).Order(ent.Desc(apikey.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "list active API keys", err)
	}
	return s.keyViews(ctx, rows)
}

func (s *Service) RevokeKeyAsMentor(ctx context.Context, mentorID, keyID uuid.UUID, comment string) error {
	comment, err := ValidateComment("mentor_revoked", comment)
	if err != nil {
		return err
	}
	tx, err := s.db.Tx(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "start key revocation transaction", err)
	}
	defer func() { _ = tx.Rollback() }()
	key, err := tx.APIKey.Query().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusActive),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return domain.NewError(domain.CodeInvalidTransition, "API key is not active")
	}
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query API key", err)
	}
	responsible, err := tx.ProjectMember.Query().Where(
		projectmember.ProjectIDEQ(key.ProjectID),
		projectmember.UserIDEQ(mentorID),
	).Exist(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "query mentor responsibility", err)
	}
	if !responsible {
		return domain.NewError(domain.CodeNotFound, "API key not found")
	}
	revokedAt := s.now().UTC()
	updated, err := tx.APIKey.Update().Where(
		apikey.IDEQ(keyID),
		apikey.StatusEQ(apikey.StatusActive),
	).SetStatus(apikey.StatusRevoked).SetRevokedAt(revokedAt).Save(ctx)
	if err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "revoke API key", err)
	}
	if updated != 1 {
		return domain.NewError(domain.CodeInvalidTransition, "API key was changed concurrently")
	}
	if _, err := tx.APIKeyAudit.Create().
		SetAPIKeyID(keyID).
		SetActorUserID(mentorID).
		SetAction(apikeyaudit.ActionMentorRevoked).
		SetComment(comment).
		Save(ctx); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "create revocation audit", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.WrapError(domain.CodeDependencyUnavailable, "commit key revocation", err)
	}
	s.publishKeyStatus(ctx, key.OwnerUserID, keyID, domain.KeyRevoked, revokedAt)
	return nil
}

func (s *Service) mentorProjectIDs(ctx context.Context, mentorID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.db.ProjectMember.Query().Where(
		projectmember.UserIDEQ(mentorID),
	).All(ctx)
	if err != nil {
		return nil, domain.WrapError(domain.CodeDependencyUnavailable, "query mentor projects", err)
	}
	result := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.ProjectID)
	}
	return result, nil
}

func (s *Service) keyViews(ctx context.Context, rows []*ent.APIKey) ([]APIKeyView, error) {
	result := make([]APIKeyView, 0, len(rows))
	for _, row := range rows {
		view, err := s.keyView(ctx, s.db.APIKey.Query().Where(apikey.IDEQ(row.ID)))
		if err != nil {
			return nil, err
		}
		result = append(result, view)
	}
	return result, nil
}

func (s *Service) mentorApplication(ctx context.Context, applicationID, mentorID uuid.UUID) (MentorProjectApplicationView, error) {
	query := s.db.MentorProjectApplication.Query().Where(mentorprojectapplication.IDEQ(applicationID))
	if mentorID != uuid.Nil {
		query.Where(mentorprojectapplication.MentorIDEQ(mentorID))
	}
	row, err := query.
		WithMentor().
		WithProject().
		WithReviewer().
		Only(ctx)
	if ent.IsNotFound(err) {
		return MentorProjectApplicationView{}, domain.NewError(domain.CodeNotFound, "project application not found")
	}
	if err != nil {
		return MentorProjectApplicationView{}, domain.WrapError(domain.CodeDependencyUnavailable, "query project application", err)
	}
	view := MentorProjectApplicationView{
		ID: row.ID.String(), Status: string(row.Status),
		ReviewComment: row.ReviewComment, ReviewedAt: row.ReviewedAt, CreatedAt: row.CreatedAt,
	}
	if row.Edges.Mentor != nil {
		view.Mentor = userView(row.Edges.Mentor)
	}
	if row.Edges.Project != nil {
		view.Project = projectView(row.Edges.Project, false)
	}
	if row.Edges.Reviewer != nil {
		reviewer := userView(row.Edges.Reviewer)
		view.Reviewer = &reviewer
	}
	return view, nil
}
