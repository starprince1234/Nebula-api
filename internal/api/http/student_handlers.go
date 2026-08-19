package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
)

func (s *Server) studentOrganizations(c *gin.Context) {
	rows, err := s.service.StudentOrganizations(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) studentProjects(c *gin.Context) {
	organizationID, ok := pathID(c, "organization_id")
	if !ok {
		return
	}
	rows, err := s.service.StudentProjects(c.Request.Context(), organizationID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) studentModels(c *gin.Context) {
	rows, err := s.service.StudentModels(c.Request.Context(), identity(c).UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) resolveStudentModel(c *gin.Context) {
	view, err := s.service.ResolveStudentModel(c.Request.Context(), c.Query("model_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) submitAPIKey(c *gin.Context) {
	var input struct {
		Name            string   `json:"name" binding:"required"`
		OrganizationID  string   `json:"organization_id" binding:"required"`
		ProjectID       string   `json:"project_id" binding:"required"`
		ModelIDs        []string `json:"model_ids"`
		RequestedModels []struct {
			ModelID          string   `json:"model_id" binding:"required"`
			DisplayName      string   `json:"display_name" binding:"required"`
			Description      *string  `json:"description"`
			Category         string   `json:"category" binding:"required"`
			Capabilities     []string `json:"capabilities" binding:"required"`
			InputModalities  []string `json:"input_modalities" binding:"required"`
			OutputModalities []string `json:"output_modalities" binding:"required"`
			ContextWindow    *int     `json:"context_window"`
			MaxOutputTokens  *int     `json:"max_output_tokens"`
		} `json:"requested_models"`
	}
	if !bindJSON(c, &input) {
		return
	}
	organizationID, err := controlplane.UUID(input.OrganizationID)
	if err != nil {
		writeError(c, err)
		return
	}
	projectID, err := controlplane.UUID(input.ProjectID)
	if err != nil {
		writeError(c, err)
		return
	}
	requested := make([]controlplane.RequestedModelInput, 0, len(input.RequestedModels))
	for _, item := range input.RequestedModels {
		requested = append(requested, controlplane.RequestedModelInput{
			ModelID: item.ModelID, DisplayName: item.DisplayName, Description: item.Description,
			Category: item.Category, Capabilities: item.Capabilities,
			InputModalities: item.InputModalities, OutputModalities: item.OutputModalities,
			ContextWindow: item.ContextWindow, MaxOutputTokens: item.MaxOutputTokens,
		})
	}
	view, err := s.service.SubmitAPIKey(c.Request.Context(), identity(c).UserID, controlplane.SubmitAPIKeyInput{
		Name: input.Name, OrganizationID: organizationID, ProjectID: projectID,
		ModelIDs: input.ModelIDs, RequestedModels: requested,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) studentAPIKeys(c *gin.Context) {
	rows, err := s.service.StudentAPIKeys(c.Request.Context(), identity(c).UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) studentAPIKey(c *gin.Context) {
	keyID, ok := pathID(c, "api_key_id")
	if !ok {
		return
	}
	view, err := s.service.StudentAPIKey(c.Request.Context(), identity(c).UserID, keyID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) claimAPIKey(c *gin.Context) {
	keyID, ok := pathID(c, "api_key_id")
	if !ok {
		return
	}
	view, err := s.service.ClaimAPIKey(c.Request.Context(), identity(c).UserID, keyID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}
