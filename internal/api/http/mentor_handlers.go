package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
	"github.com/starprince1234/Nebula-api/internal/domain"
)

func (s *Server) mentorOrganizations(c *gin.Context) {
	rows, err := s.service.MentorOrganizations(c.Request.Context(), identity(c).UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) mentorProjects(c *gin.Context) {
	organizationID, ok := pathID(c, "organization_id")
	if !ok {
		return
	}
	rows, err := s.service.MentorProjects(c.Request.Context(), identity(c).UserID, organizationID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) applyMentorProject(c *gin.Context) {
	var input struct {
		ProjectID string `json:"project_id" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	projectID, err := controlplane.UUID(input.ProjectID)
	if err != nil {
		writeError(c, err)
		return
	}
	view, err := s.service.ApplyMentorProject(c.Request.Context(), identity(c).UserID, projectID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, view)
}

func (s *Server) mentorProjectApplications(c *gin.Context) {
	rows, err := s.service.MentorProjectApplications(c.Request.Context(), identity(c).UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) mentorKeyReviews(c *gin.Context) {
	rows, err := s.service.MentorKeyReviews(c.Request.Context(), identity(c).UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) mentorKeyReview(c *gin.Context) {
	keyID, ok := pathID(c, "api_key_id")
	if !ok {
		return
	}
	view, err := s.service.MentorKeyReview(c.Request.Context(), identity(c).UserID, keyID)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, view)
}

func (s *Server) reviewKeyAsMentor(approve bool) gin.HandlerFunc {
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
		if err := s.service.ReviewKeyAsMentor(c.Request.Context(), identity(c).UserID, keyID, approve, comment, quota...); err != nil {
			writeError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func (s *Server) mentorActiveKeys(c *gin.Context) {
	projectID, ok := pathID(c, "project_id")
	if !ok {
		return
	}
	rows, err := s.service.MentorActiveKeys(c.Request.Context(), identity(c).UserID, projectID)
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}

func (s *Server) revokeKeyAsMentor(c *gin.Context) {
	keyID, ok := pathID(c, "api_key_id")
	if !ok {
		return
	}
	comment, ok := bindOptionalComment(c)
	if !ok {
		return
	}
	if err := s.service.RevokeKeyAsMentor(c.Request.Context(), identity(c).UserID, keyID, comment); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
