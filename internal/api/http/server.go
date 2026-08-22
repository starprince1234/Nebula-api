package httpapi

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/starprince1234/Nebula-api/internal/controlplane"
	"github.com/starprince1234/Nebula-api/internal/dataplane"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/cache"
	security "github.com/starprince1234/Nebula-api/internal/infrastructure/crypto"
)

type Config struct {
	CookieSecure  bool
	RefreshTTL    time.Duration
	SSEHeartbeat  time.Duration
	AllowedOrigin string
}

type Server struct {
	engine   *gin.Engine
	service  *controlplane.Service
	security *security.Manager
	cache    *cache.Store
	health   HealthDependencies
	config   Config
}

func NewServer(
	service *controlplane.Service,
	securityManager *security.Manager,
	cacheStore *cache.Store,
	gateway *dataplane.Gateway,
	health HealthDependencies,
	cfg Config,
) *Server {
	gin.SetMode(gin.ReleaseMode)
	gin.EnableJsonDecoderDisallowUnknownFields()
	engine := gin.New()
	server := &Server{
		engine: engine, service: service, security: securityManager,
		cache: cacheStore, health: health, config: cfg,
	}
	engine.Use(server.requestContext(), server.recovery(), securityHeaders(), cors(server.config.AllowedOrigin))
	engine.GET("/health/live", server.live)
	engine.GET("/health/ready", server.ready)
	server.registerControlPlane()
	engine.GET("/v1/*path", gin.WrapH(gateway))
	engine.POST("/v1/*path", gin.WrapH(gateway))
	engine.POST("/v1beta/*path", gin.WrapH(gateway))
	engine.POST("/v2/*path", gin.WrapH(gateway))
	return server
}

func (s *Server) Handler() http.Handler {
	return s.engine
}

func (s *Server) registerControlPlane() {
	api := s.engine.Group("/api/v1")
	auth := api.Group("/auth")
	auth.POST("/verification-codes", s.sendVerificationCode)
	auth.POST("/register/student", s.register("student"))
	auth.POST("/register/mentor", s.register("mentor"))
	auth.POST("/login", s.login)
	auth.POST("/refresh", s.refresh)
	auth.POST("/logout", s.logout)
	auth.POST("/password/forgot", s.forgotPassword)
	auth.POST("/password/reset", s.resetPassword)
	auth.POST("/teacher-invitations/activate", s.activateTeacher)

	protected := api.Group("")
	protected.Use(s.authenticate())
	protected.GET("/me", s.me)
	protected.GET("/events", s.events)

	student := protected.Group("/student")
	student.Use(requireRole("student"))
	student.GET("/organizations", s.studentOrganizations)
	student.GET("/organizations/:organization_id/projects", s.studentProjects)
	student.GET("/models", s.studentModels)
	student.GET("/models/resolve", s.resolveStudentModel)
	student.POST("/api-keys", s.submitAPIKey)
	student.GET("/api-keys", s.studentAPIKeys)
	student.GET("/api-keys/:api_key_id", s.studentAPIKey)
	student.POST("/api-keys/:api_key_id/claim", s.claimAPIKey)

	mentor := protected.Group("/mentor")
	mentor.Use(requireRole("mentor"))
	mentor.GET("/organizations", s.mentorOrganizations)
	mentor.GET("/organizations/:organization_id/projects", s.mentorProjects)
	mentor.POST("/project-applications", s.applyMentorProject)
	mentor.GET("/project-applications", s.mentorProjectApplications)
	mentor.GET("/api-key-reviews", s.mentorKeyReviews)
	mentor.GET("/api-key-reviews/:api_key_id", s.mentorKeyReview)
	mentor.POST("/api-key-reviews/:api_key_id/approve", s.reviewKeyAsMentor(true))
	mentor.POST("/api-key-reviews/:api_key_id/reject", s.reviewKeyAsMentor(false))
	mentor.GET("/projects/:project_id/api-keys", s.mentorActiveKeys)
	mentor.POST("/api-keys/:api_key_id/revoke", s.revokeKeyAsMentor)

	teacher := protected.Group("/teacher")
	teacher.Use(requireRole("teacher"))
	teacher.POST("/invitations", s.inviteTeacher)
	teacher.GET("/organizations", s.teacherOrganizations)
	teacher.POST("/organizations", s.createOrganization)
	teacher.PATCH("/organizations/:organization_id", s.updateOrganization)
	teacher.GET("/organizations/:organization_id/mentor-candidates", s.mentorCandidates)
	teacher.POST("/organizations/:organization_id/mentors/:mentor_id", s.assignMentor)
	teacher.GET("/projects", s.teacherProjects)
	teacher.POST("/projects", s.createProject)
	teacher.PATCH("/projects/:project_id", s.updateProject)
	teacher.GET("/mentor-project-applications", s.teacherMentorApplications)
	teacher.POST("/mentor-project-applications/:application_id/approve", s.reviewMentorApplication(true))
	teacher.POST("/mentor-project-applications/:application_id/reject", s.reviewMentorApplication(false))
	teacher.GET("/providers", s.teacherProviders)
	teacher.POST("/providers", s.createProvider)
	teacher.GET("/providers/:provider_id", s.teacherProvider)
	teacher.PATCH("/providers/:provider_id", s.updateProvider)
	teacher.GET("/models", s.teacherModels)
	teacher.POST("/models", s.createModel)
	teacher.GET("/models/:model_id", s.teacherModel)
	teacher.PATCH("/models/:model_id", s.updateModel)
	teacher.POST("/models/:model_id/bindings", s.createBinding)
	teacher.PATCH("/model-bindings/:binding_id", s.updateBinding)
	teacher.GET("/api-key-reviews", s.teacherKeyReviews)
	teacher.GET("/api-key-reviews/:api_key_id", s.teacherKeyReview)
	teacher.POST("/api-key-reviews/:api_key_id/approve", s.reviewKeyAsTeacher(true))
	teacher.POST("/api-key-reviews/:api_key_id/reject", s.reviewKeyAsTeacher(false))
}
