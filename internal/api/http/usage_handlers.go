package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/usage"
)

func monthQuery(c *gin.Context) (time.Time, error) {
	return usage.ParseMonth(strings.TrimSpace(c.Query("month")), time.Now())
}
func (s *Server) studentUsage(c *gin.Context) {
	month, err := monthQuery(c)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := s.service.StudentUsage(c.Request.Context(), identity(c).UserID, month)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, v)
}
func (s *Server) mentorProjectUsage(c *gin.Context) {
	id, ok := pathID(c, "project_id")
	if !ok {
		return
	}
	month, err := monthQuery(c)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := s.service.MentorProjectUsage(c.Request.Context(), identity(c).UserID, id, month)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, v)
}

func (s *Server) updateMentorKeyQuota(c *gin.Context) {
	id, ok := pathID(c, "api_key_id")
	if !ok {
		return
	}
	var input struct {
		MonthlyCredits string `json:"monthly_credits" binding:"required"`
		Reason         string `json:"reason" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	quota, err := domain.ParseCredits(input.MonthlyCredits)
	if err != nil {
		writeError(c, err)
		return
	}
	if err := s.service.UpdateMentorKeyQuota(c.Request.Context(), identity(c).UserID, id, quota, input.Reason); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
func parseLogFilter(c *gin.Context) (usage.LogFilter, error) {
	f := usage.LogFilter{Status: strings.TrimSpace(c.Query("status")), Limit: 50}
	if raw := c.Query("limit"); raw != "" {
		if _, err := fmt.Sscan(raw, &f.Limit); err != nil {
			return f, domain.NewError(domain.CodeValidation, "invalid limit")
		}
	}
	cur, err := usage.DecodeCursor(c.Query("cursor"))
	if err != nil {
		return f, err
	}
	f.Cursor = cur
	for name, dst := range map[string]**uuid.UUID{"project_id": &f.ProjectID, "user_id": &f.UserID, "api_key_id": &f.APIKeyID, "model_id": &f.ModelID} {
		if raw := strings.TrimSpace(c.Query(name)); raw != "" {
			id, err := controlplane.UUID(raw)
			if err != nil {
				return f, err
			}
			*dst = &id
		}
	}
	return f, nil
}
func (s *Server) mentorCallLogs(c *gin.Context) {
	f, err := parseLogFilter(c)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := s.service.MentorCallLogs(c.Request.Context(), identity(c).UserID, f)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, v)
}
func (s *Server) mentorCallLog(c *gin.Context) {
	id, ok := pathID(c, "call_id")
	if !ok {
		return
	}
	v, err := s.service.MentorCallLog(c.Request.Context(), identity(c).UserID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, v)
}
func (s *Server) mentorInputs(c *gin.Context) {
	f, err := parseLogFilter(c)
	if err != nil {
		writeError(c, err)
		return
	}
	v, err := s.service.MentorInputs(c.Request.Context(), identity(c).UserID, usage.InputFilter{LogFilter: f, Query: c.Query("q")})
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	respond(c, http.StatusOK, v)
}
func (s *Server) mentorInput(c *gin.Context) {
	id, ok := pathID(c, "call_id")
	if !ok {
		return
	}
	v, err := s.service.MentorInput(c.Request.Context(), identity(c).UserID, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	respond(c, http.StatusOK, v)
}
func (s *Server) teacherProjectSpend(c *gin.Context) {
	month, err := monthQuery(c)
	if err != nil {
		writeError(c, err)
		return
	}
	rows, err := s.service.TeacherProjectSpend(c.Request.Context(), month, c.Query("organization_id"), c.Query("q"))
	if err != nil {
		writeError(c, err)
		return
	}
	respondList(c, rows)
}
