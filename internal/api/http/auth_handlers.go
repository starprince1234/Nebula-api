package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const refreshCookieName = "nebula_refresh_token"

func (s *Server) sendVerificationCode(c *gin.Context) {
	var input struct {
		Email   string `json:"email" binding:"required"`
		Purpose string `json:"purpose" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	if err := s.service.SendRegistrationCode(c.Request.Context(), input.Email, input.Purpose); err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusAccepted, gin.H{"sent": true})
}

func (s *Server) register(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var input struct {
			Name             string `json:"name" binding:"required"`
			Email            string `json:"email" binding:"required"`
			Password         string `json:"password" binding:"required"`
			VerificationCode string `json:"verification_code" binding:"required"`
		}
		if !bindJSON(c, &input) {
			return
		}
		user, err := s.service.Register(c.Request.Context(), role, input.Name, input.Email, input.Password, input.VerificationCode)
		if err != nil {
			writeError(c, err)
			return
		}
		respond(c, http.StatusCreated, user)
	}
}

func (s *Server) login(c *gin.Context) {
	var input struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	session, err := s.service.Login(c.Request.Context(), input.Email, input.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	s.setRefreshCookie(c, session.RefreshToken)
	respond(c, http.StatusOK, session)
}

func (s *Server) refresh(c *gin.Context) {
	token, _ := c.Cookie(refreshCookieName)
	session, err := s.service.Refresh(c.Request.Context(), token)
	if err != nil {
		s.clearRefreshCookie(c)
		writeError(c, err)
		return
	}
	s.setRefreshCookie(c, session.RefreshToken)
	respond(c, http.StatusOK, session)
}

func (s *Server) logout(c *gin.Context) {
	token, _ := c.Cookie(refreshCookieName)
	if err := s.service.Logout(c.Request.Context(), token); err != nil {
		writeError(c, err)
		return
	}
	s.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (s *Server) forgotPassword(c *gin.Context) {
	var input struct {
		Email string `json:"email" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	if err := s.service.ForgotPassword(c.Request.Context(), input.Email); err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusAccepted, gin.H{"accepted": true})
}

func (s *Server) resetPassword(c *gin.Context) {
	var input struct {
		Email            string `json:"email" binding:"required"`
		VerificationCode string `json:"verification_code" binding:"required"`
		NewPassword      string `json:"new_password" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	if err := s.service.ResetPassword(c.Request.Context(), input.Email, input.VerificationCode, input.NewPassword); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) activateTeacher(c *gin.Context) {
	var input struct {
		Token    string `json:"token" binding:"required"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if !bindJSON(c, &input) {
		return
	}
	user, err := s.service.ActivateTeacher(c.Request.Context(), input.Token, input.Name, input.Password)
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusCreated, user)
}

func (s *Server) me(c *gin.Context) {
	user, err := s.service.CurrentUser(c.Request.Context(), identity(c))
	if err != nil {
		writeError(c, err)
		return
	}
	respond(c, http.StatusOK, user)
}

func (s *Server) setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		refreshCookieName,
		token,
		int(s.config.RefreshTTL.Seconds()),
		"/api/v1/auth",
		"",
		s.config.CookieSecure,
		true,
	)
}

func (s *Server) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, "/api/v1/auth", "", s.config.CookieSecure, true)
	c.Header("Expires", time.Unix(1, 0).UTC().Format(http.TimeFormat))
}
