package controlplane

import (
	"time"

	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
)

type UserView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type OrganizationView struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
}

type ProjectView struct {
	ID                string  `json:"id"`
	OrganizationID    string  `json:"organization_id"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	Status            string  `json:"status"`
	HasMentor         bool    `json:"has_mentor"`
	IsResponsible     bool    `json:"is_responsible,omitempty"`
	ApplicationStatus *string `json:"application_status,omitempty"`
}

type ModelView struct {
	ID               string   `json:"id"`
	ModelID          string   `json:"model_id"`
	DisplayName      string   `json:"display_name"`
	Description      *string  `json:"description"`
	Category         string   `json:"category"`
	Capabilities     []string `json:"capabilities"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	ContextWindow    *int     `json:"context_window"`
	MaxOutputTokens  *int     `json:"max_output_tokens"`
	IsCommon         bool     `json:"is_common"`
	Status           string   `json:"status"`
	RouteReady       bool     `json:"route_ready"`
}

type MentorCandidateView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	IsMember bool   `json:"is_member"`
}

type AuditView struct {
	Action    string    `json:"action"`
	ActorRole string    `json:"actor_role"`
	Comment   *string   `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type ProgressView struct {
	Current        string   `json:"current"`
	CompletedSteps []string `json:"completed_steps"`
}

type APIKeyView struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Status       string           `json:"status"`
	Progress     ProgressView     `json:"progress"`
	Applicant    UserView         `json:"applicant,omitempty"`
	Organization OrganizationView `json:"organization"`
	Project      ProjectView      `json:"project"`
	Models       []ModelView      `json:"models"`
	Audits       []AuditView      `json:"audits,omitempty"`
	KeyPrefix    *string          `json:"key_prefix"`
	ClaimedAt    *time.Time       `json:"claimed_at"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type SessionView struct {
	AccessToken  string   `json:"access_token"`
	TokenType    string   `json:"token_type"`
	ExpiresIn    int64    `json:"expires_in"`
	RefreshToken string   `json:"-"`
	User         UserView `json:"user"`
}

type MentorProjectApplicationView struct {
	ID            string      `json:"id"`
	Status        string      `json:"status"`
	Mentor        UserView    `json:"mentor"`
	Project       ProjectView `json:"project"`
	Reviewer      *UserView   `json:"reviewer,omitempty"`
	ReviewComment *string     `json:"review_comment"`
	ReviewedAt    *time.Time  `json:"reviewed_at"`
	CreatedAt     time.Time   `json:"created_at"`
}

type ProviderView struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	BaseURL              string    `json:"base_url"`
	CredentialConfigured bool      `json:"credential_configured"`
	Status               string    `json:"status"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type BindingView struct {
	ID                string    `json:"id"`
	ModelID           string    `json:"model_id"`
	ProviderID        string    `json:"provider_id"`
	UpstreamModelName string    `json:"upstream_model_name"`
	Adapter           string    `json:"adapter"`
	Priority          int       `json:"priority"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func userView(user *ent.User) UserView {
	return UserView{
		ID: user.ID.String(), Name: user.Name, Email: user.Email,
		Role: string(user.Role), Status: string(user.Status), CreatedAt: user.CreatedAt,
	}
}

func organizationView(organization *ent.Organization) OrganizationView {
	return OrganizationView{
		ID: organization.ID.String(), Name: organization.Name,
		Description: organization.Description, Status: string(organization.Status),
	}
}

func projectView(project *ent.Project, hasMentor bool) ProjectView {
	return ProjectView{
		ID: project.ID.String(), OrganizationID: project.OrganizationID.String(),
		Name: project.Name, Description: project.Description,
		Status: string(project.Status), HasMentor: hasMentor,
	}
}

func modelView(model *ent.Model) ModelView {
	return ModelView{
		ID: model.ID.String(), ModelID: model.ModelID, DisplayName: model.DisplayName,
		Description: model.Description, Category: string(model.Category),
		Capabilities: model.Capabilities, InputModalities: model.InputModalities,
		OutputModalities: model.OutputModalities, ContextWindow: model.ContextWindow,
		MaxOutputTokens: model.MaxOutputTokens, IsCommon: model.IsCommon,
		Status: string(model.Status),
	}
}
