package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
	"github.com/starprince1234/Nebula-api/internal/domain"
)

func (s *Server) inviteTeacher(c *gin.Context) {
	var input struct {
		Email string `json:"email" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	if err := s.service.InviteTeacher(c.Request.Context(), identity(c), input.Email); err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusAccepted, gin.H{"invited": true})
}

func (s *Server) teacherOrganizations(c *gin.Context) {
	rows, err := s.service.TeacherOrganizations(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) createOrganization(c *gin.Context) {
	var input struct {
		Name        string  `json:"name" binding:"required"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if !bindJSON(c, &input) {
		return
	}
	view, err := s.service.CreateOrganization(c.Request.Context(), controlplane.OrganizationInput{
		Name: input.Name, Description: input.Description, Status: input.Status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) updateOrganization(c *gin.Context) {
	organizationID, ok := pathID(c, "organization_id")
	if !ok {
		return
	}
	var input struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if !bindJSON(c, &input) {
		return
	}
	view, err := s.service.UpdateOrganization(c.Request.Context(), organizationID, controlplane.OrganizationInput{
		Name: input.Name, Description: input.Description, Status: input.Status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) mentorCandidates(c *gin.Context) {
	organizationID, ok := pathID(c, "organization_id")
	if !ok {
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			writeError(c, domain.NewError(domain.CodeValidation, "invalid limit"))
			return
		}
		limit = parsed
	}
	cursor, err := controlplane.DecodeMentorCandidateCursor(c.Query("cursor"))
	if err != nil {
		writeError(c, err)
		return
	}
	rows, nextCursor, err := s.service.MentorCandidates(
		c.Request.Context(), organizationID, c.Query("q"), cursor, limit,
	)
	if err != nil {
		writeError(c, err)
		return
	}
	respondPage(c, rows, nextCursor)
}

func (s *Server) assignMentor(c *gin.Context) {
	organizationID, ok := pathID(c, "organization_id")
	if !ok {
		return
	}
	mentorID, ok := pathID(c, "mentor_id")
	if !ok {
		return
	}
	if err := s.service.AssignMentorToOrganization(c.Request.Context(), organizationID, mentorID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) teacherProjects(c *gin.Context) {
	var organizationID *string
	if raw := strings.TrimSpace(c.Query("organization_id")); raw != "" {
		organizationID = &raw
	}
	var parsedOrganizationID *uuid.UUID
	if organizationID != nil {
		id, err := controlplane.UUID(*organizationID)
		if err != nil {
			writeError(c, err)
			return
		}
		parsedOrganizationID = &id
	}
	var status *string
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		status = &raw
	}
	rows, err := s.service.TeacherProjects(c.Request.Context(), parsedOrganizationID, status)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) createProject(c *gin.Context) {
	var input struct {
		OrganizationID     string  `json:"organization_id" binding:"required"`
		Name               string  `json:"name" binding:"required"`
		Description        *string `json:"description"`
		Status             *string `json:"status"`
		MonthlyCreditQuota string  `json:"monthly_credit_quota"`
	}
	if !bindJSON(c, &input) {
		return
	}
	organizationID, err := controlplane.UUID(input.OrganizationID)
	if err != nil {
		writeError(c, err)
		return
	}
	var quota *int64
	if input.MonthlyCreditQuota != "" {
		value, parseErr := domain.ParseCredits(input.MonthlyCreditQuota)
		if parseErr != nil {
			writeError(c, parseErr)
			return
		}
		quota = &value
	}
	view, err := s.service.CreateProject(c.Request.Context(), controlplane.ProjectInput{
		OrganizationID: organizationID, Name: input.Name,
		Description: input.Description, Status: input.Status,
		MonthlyCreditQuotaMilli: quota,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) updateProject(c *gin.Context) {
	projectID, ok := pathID(c, "project_id")
	if !ok {
		return
	}
	var input struct {
		Name               string  `json:"name"`
		Description        *string `json:"description"`
		Status             *string `json:"status"`
		MonthlyCreditQuota string  `json:"monthly_credit_quota"`
		QuotaChangeReason  string  `json:"quota_change_reason"`
	}
	if !bindJSON(c, &input) {
		return
	}
	var quota *int64
	if input.MonthlyCreditQuota != "" {
		value, parseErr := domain.ParseCredits(input.MonthlyCreditQuota)
		if parseErr != nil {
			writeError(c, parseErr)
			return
		}
		quota = &value
	}
	view, err := s.service.UpdateProject(c.Request.Context(), identity(c).UserID, projectID, controlplane.ProjectInput{
		Name: input.Name, Description: input.Description, Status: input.Status,
		MonthlyCreditQuotaMilli: quota, QuotaChangeReason: input.QuotaChangeReason,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) teacherMentorApplications(c *gin.Context) {
	var status *string
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		status = &raw
	}
	rows, err := s.service.TeacherMentorApplications(c.Request.Context(), status)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) reviewMentorApplication(approve bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		applicationID, ok := pathID(c, "application_id")
		if !ok {
			return
		}
		comment, ok := bindOptionalComment(c)
		if !ok {
			return
		}
		if err := s.service.ReviewMentorApplication(
			c.Request.Context(), identity(c).UserID, applicationID, approve, comment,
		); err != nil {
			writeError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func (s *Server) teacherProviders(c *gin.Context) {
	rows, err := s.service.TeacherProviders(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) createProvider(c *gin.Context) {
	var input struct {
		Name       string  `json:"name" binding:"required"`
		BaseURL    string  `json:"base_url" binding:"required"`
		Credential string  `json:"credential" binding:"required"`
		Status     *string `json:"status"`
	}
	if !bindJSON(c, &input) {
		return
	}
	view, err := s.service.CreateProvider(c.Request.Context(), controlplane.ProviderInput{
		Name: input.Name, BaseURL: input.BaseURL, Credential: &input.Credential, Status: input.Status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) teacherProvider(c *gin.Context) {
	providerID, ok := pathID(c, "provider_id")
	if !ok {
		return
	}
	view, err := s.service.TeacherProvider(c.Request.Context(), providerID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) updateProvider(c *gin.Context) {
	providerID, ok := pathID(c, "provider_id")
	if !ok {
		return
	}
	var input struct {
		Name       string  `json:"name"`
		BaseURL    string  `json:"base_url"`
		Credential *string `json:"credential"`
		Status     *string `json:"status"`
	}
	if !bindJSON(c, &input) {
		return
	}
	view, err := s.service.UpdateProvider(c.Request.Context(), providerID, controlplane.ProviderInput{
		Name: input.Name, BaseURL: input.BaseURL, Credential: input.Credential, Status: input.Status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

type modelPayload struct {
	ModelID                string   `json:"model_id"`
	DisplayName            string   `json:"display_name"`
	Description            *string  `json:"description"`
	Category               string   `json:"category"`
	Capabilities           []string `json:"capabilities"`
	InputModalities        []string `json:"input_modalities"`
	OutputModalities       []string `json:"output_modalities"`
	ContextWindow          *int     `json:"context_window"`
	MaxOutputTokens        *int     `json:"max_output_tokens"`
	IsCommon               *bool    `json:"is_common"`
	Status                 *string  `json:"status"`
	CreditMultiplier       string   `json:"credit_multiplier"`
	MultiplierChangeReason string   `json:"multiplier_change_reason"`
}

type nullableIntPayload struct {
	Set   bool
	Value *int
}

func (value *nullableIntPayload) UnmarshalJSON(raw []byte) error {
	value.Set = true
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		value.Value = nil
		return nil
	}
	var parsed int
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	value.Value = &parsed
	return nil
}

type modelPatchPayload struct {
	DisplayName            string             `json:"display_name"`
	Description            *string            `json:"description"`
	Category               string             `json:"category"`
	Capabilities           []string           `json:"capabilities"`
	InputModalities        []string           `json:"input_modalities"`
	OutputModalities       []string           `json:"output_modalities"`
	ContextWindow          nullableIntPayload `json:"context_window"`
	MaxOutputTokens        nullableIntPayload `json:"max_output_tokens"`
	IsCommon               *bool              `json:"is_common"`
	Status                 *string            `json:"status"`
	CreditMultiplier       string             `json:"credit_multiplier"`
	MultiplierChangeReason string             `json:"multiplier_change_reason"`
}

func (payload modelPayload) input() controlplane.ModelInput {
	var multiplier *int64
	if payload.CreditMultiplier != "" {
		value, err := domain.ParseCredits(payload.CreditMultiplier)
		if err == nil {
			multiplier = &value
		}
	}
	return controlplane.ModelInput{
		ModelID: payload.ModelID, DisplayName: payload.DisplayName,
		Description: payload.Description, Category: payload.Category,
		Capabilities: payload.Capabilities, InputModalities: payload.InputModalities,
		OutputModalities: payload.OutputModalities, ContextWindow: payload.ContextWindow,
		MaxOutputTokens: payload.MaxOutputTokens, IsCommon: payload.IsCommon, Status: payload.Status,
		CreditMultiplierMilli: multiplier, MultiplierChangeReason: payload.MultiplierChangeReason,
	}
}

func (s *Server) teacherModels(c *gin.Context) {
	rows, err := s.service.TeacherModels(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) createModel(c *gin.Context) {
	var input modelPayload
	if !bindJSON(c, &input) {
		return
	}
	if input.ModelID == "" || input.DisplayName == "" {
		writeError(c, domain.NewError(domain.CodeValidation, "model_id and display_name are required"))
		return
	}
	if input.CreditMultiplier != "" {
		if _, err := domain.ParseCredits(input.CreditMultiplier); err != nil {
			writeError(c, err)
			return
		}
	}
	view, err := s.service.CreateModel(c.Request.Context(), input.input())
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) teacherModel(c *gin.Context) {
	modelID, ok := pathID(c, "model_id")
	if !ok {
		return
	}
	model, bindings, err := s.service.TeacherModel(c.Request.Context(), modelID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"model": model, "bindings": bindings})
}

func (s *Server) updateModel(c *gin.Context) {
	modelID, ok := pathID(c, "model_id")
	if !ok {
		return
	}
	var input modelPatchPayload
	if !bindJSON(c, &input) {
		return
	}
	var multiplier *int64
	if input.CreditMultiplier != "" {
		value, err := domain.ParseCredits(input.CreditMultiplier)
		if err != nil {
			writeError(c, err)
			return
		}
		multiplier = &value
	}
	view, err := s.service.UpdateModel(c.Request.Context(), identity(c).UserID, modelID, controlplane.ModelInput{
		DisplayName: input.DisplayName, Description: input.Description, Category: input.Category,
		Capabilities: input.Capabilities, InputModalities: input.InputModalities,
		OutputModalities: input.OutputModalities, ContextWindow: input.ContextWindow.Value,
		ContextWindowSet: input.ContextWindow.Set, MaxOutputTokens: input.MaxOutputTokens.Value,
		MaxOutputTokensSet: input.MaxOutputTokens.Set, IsCommon: input.IsCommon, Status: input.Status,
		CreditMultiplierMilli: multiplier, MultiplierChangeReason: input.MultiplierChangeReason,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) createBinding(c *gin.Context) {
	modelID, ok := pathID(c, "model_id")
	if !ok {
		return
	}
	var input struct {
		ProviderID        string `json:"provider_id" binding:"required"`
		UpstreamModelName string `json:"upstream_model_name" binding:"required"`
		Adapter           string `json:"adapter" binding:"required"`
		Priority          *int   `json:"priority"`
		Status            string `json:"status"`
	}
	if !bindJSON(c, &input) {
		return
	}
	providerID, err := controlplane.UUID(input.ProviderID)
	if err != nil {
		writeError(c, err)
		return
	}
	priority := 100
	if input.Priority != nil {
		priority = *input.Priority
	}
	if input.Status == "" {
		input.Status = "active"
	}
	view, err := s.service.CreateBinding(c.Request.Context(), modelID, controlplane.BindingInput{
		ProviderID: providerID, UpstreamModelName: input.UpstreamModelName,
		Adapter: input.Adapter, Priority: priority, Status: input.Status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) updateBinding(c *gin.Context) {
	bindingID, ok := pathID(c, "binding_id")
	if !ok {
		return
	}
	var input struct {
		UpstreamModelName string `json:"upstream_model_name"`
		Adapter           string `json:"adapter"`
		Priority          *int   `json:"priority"`
		Status            string `json:"status"`
	}
	if !bindJSON(c, &input) {
		return
	}
	priority := -1
	if input.Priority != nil {
		priority = *input.Priority
	}
	view, err := s.service.UpdateBinding(c.Request.Context(), bindingID, controlplane.BindingInput{
		UpstreamModelName: input.UpstreamModelName,
		Adapter:           input.Adapter, Priority: priority, Status: input.Status,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) teacherKeyReviews(c *gin.Context) {
	rows, err := s.service.TeacherKeyReviews(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) teacherKeyReview(c *gin.Context) {
	keyID, ok := pathID(c, "api_key_id")
	if !ok {
		return
	}
	view, err := s.service.TeacherKeyReview(c.Request.Context(), keyID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) reviewKeyAsTeacher(approve bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyID, ok := pathID(c, "api_key_id")
		if !ok {
			return
		}
		var input struct {
			Comment        string `json:"comment"`
			MonthlyCredits string `json:"monthly_credits"`
		}
		if c.Request.ContentLength != 0 {
			if !bindJSON(c, &input) {
				return
			}
		}
		comment := input.Comment
		var quota []int64
		if approve && strings.TrimSpace(input.MonthlyCredits) != "" {
			value, err := domain.ParseCredits(input.MonthlyCredits)
			if err != nil {
				writeError(c, err)
				return
			}
			quota = []int64{value}
		}
		if err := s.service.ReviewKeyAsTeacher(
			c.Request.Context(), identity(c).UserID, keyID, approve, comment, quota...,
		); err != nil {
			writeError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}
