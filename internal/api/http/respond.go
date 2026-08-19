package httpapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
	"github.com/starprince1234/Nebula-api/internal/domain"
)

const (
	requestIDKey = "request_id"
	identityKey  = "identity"
)

func (s *Server) requestContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.Must(uuid.NewV7()).String()
		}
		c.Set(requestIDKey, requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}

func (s *Server) recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("panic recovered",
					"request_id", requestID(c),
					"error", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				writeError(c, domain.NewError("INTERNAL_ERROR", "internal server error"))
				c.Abort()
			}
		}()
		c.Next()
	}
}

func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

func cors(allowedOrigin string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" && origin == allowedOrigin {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, Last-Event-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
			c.Header("Access-Control-Expose-Headers", "X-Request-ID")
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			if origin == allowedOrigin && origin != "" {
				c.Status(http.StatusNoContent)
			} else {
				c.Status(http.StatusForbidden)
			}
			c.Abort()
			return
		}
		c.Next()
	}
}

func (s *Server) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := strings.TrimSpace(c.GetHeader("Authorization"))
		if len(authorization) <= 7 || !strings.EqualFold(authorization[:7], "Bearer ") {
			writeError(c, domain.NewError(domain.CodeAuthenticationRequired, "access token is required"))
			c.Abort()
			return
		}
		claims, err := s.security.ParseAccessToken(strings.TrimSpace(authorization[7:]))
		if err != nil {
			writeError(c, domain.NewError(domain.CodeTokenExpired, "access token is invalid or expired"))
			c.Abort()
			return
		}
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			writeError(c, domain.NewError(domain.CodeAuthenticationRequired, "invalid access token"))
			c.Abort()
			return
		}
		current, err := s.service.CurrentUser(c.Request.Context(), controlplane.Identity{UserID: userID, Role: claims.Role})
		if err != nil {
			writeError(c, err)
			c.Abort()
			return
		}
		if current.Role != claims.Role {
			writeError(c, domain.NewError(domain.CodeAuthenticationRequired, "access token role is stale"))
			c.Abort()
			return
		}
		c.Set(identityKey, controlplane.Identity{UserID: userID, Role: current.Role})
		c.Next()
	}
}

func requireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if identity(c).Role != role {
			writeError(c, domain.NewError(domain.CodeForbidden, "role is not allowed"))
			c.Abort()
			return
		}
		c.Next()
	}
}

func bindJSON(c *gin.Context, destination any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	if err := c.ShouldBindJSON(destination); err != nil {
		writeError(c, &domain.Error{
			Code: domain.CodeValidation, Message: "request validation failed",
			Details: []map[string]string{{"field": "body", "reason": err.Error()}},
		})
		return false
	}
	return true
}

func bindOptionalComment(c *gin.Context) (string, bool) {
	if c.Request.ContentLength == 0 {
		return "", true
	}
	var input struct {
		Comment string `json:"comment"`
	}
	if !bindJSON(c, &input) {
		return "", false
	}
	return input.Comment, true
}

func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "request_id": requestID(c)})
}

func respondList(c *gin.Context, data any) {
	respondPage(c, data, nil)
}

func respondPage(c *gin.Context, data any, nextCursor *string) {
	c.JSON(http.StatusOK, gin.H{
		"data": data, "meta": gin.H{"next_cursor": nextCursor}, "request_id": requestID(c),
	})
}

func writeError(c *gin.Context, err error) {
	var appErr *domain.Error
	if !errors.As(err, &appErr) {
		slog.Error("unhandled request error", "request_id", requestID(c), "error", err)
		appErr = domain.NewError("INTERNAL_ERROR", "internal server error")
	} else if appErr.Cause != nil {
		slog.Error("request failed",
			"request_id", requestID(c),
			"code", appErr.Code,
			"error", appErr.Cause,
		)
	}
	status := errorStatus(appErr.Code)
	payload := gin.H{"code": appErr.Code, "message": appErr.Message}
	if appErr.Details != nil {
		payload["details"] = appErr.Details
	}
	c.AbortWithStatusJSON(status, gin.H{"error": payload, "request_id": requestID(c)})
}

func errorStatus(code string) int {
	switch code {
	case domain.CodeValidation:
		return http.StatusBadRequest
	case domain.CodeAuthenticationRequired, domain.CodeInvalidCredentials, domain.CodeTokenExpired:
		return http.StatusUnauthorized
	case domain.CodeForbidden, domain.CodeAccountDisabled:
		return http.StatusForbidden
	case domain.CodeNotFound:
		return http.StatusNotFound
	case domain.CodeEmailRegistered, domain.CodeNameConflict, domain.CodeInvalidTransition,
		domain.CodeProjectNoMentor, domain.CodeModelNotReady, domain.CodeModelRoutingRequired,
		domain.CodeKeyAlreadyClaimed:
		return http.StatusConflict
	case domain.CodeVerificationInvalid, domain.CodeVerificationExpired, domain.CodeInvitationInvalid:
		return http.StatusUnprocessableEntity
	case domain.CodeRateLimited, domain.CodeVerificationLocked:
		return http.StatusTooManyRequests
	case domain.CodeDependencyUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func requestID(c *gin.Context) string {
	value, _ := c.Get(requestIDKey)
	requestID, _ := value.(string)
	return requestID
}

func identity(c *gin.Context) controlplane.Identity {
	value, _ := c.Get(identityKey)
	identity, _ := value.(controlplane.Identity)
	return identity
}

func pathID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := controlplane.UUID(c.Param(name))
	if err != nil {
		writeError(c, err)
		return uuid.Nil, false
	}
	return id, true
}
