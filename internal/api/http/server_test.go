package httpapi

import (
	"strings"
	"testing"

	"github.com/starprince1234/Nebula-api/internal/dataplane"
)

func TestRegistersControlPlaneAndGatewayRoutes(t *testing.T) {
	t.Parallel()
	server := NewServer(nil, nil, nil, (*dataplane.Gateway)(nil), HealthDependencies{}, Config{})
	routes := make(map[string]struct{})
	for _, route := range server.engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
		if route.Method == "DELETE" {
			t.Fatalf("unexpected DELETE route: %s", route.Path)
		}
		if strings.Contains(route.Path, "/api/nebula/gateway") {
			t.Fatalf("legacy gateway route was registered: %s", route.Path)
		}
	}
	expected := []string{
		"GET /health/live",
		"GET /health/ready",
		"POST /api/v1/auth/verification-codes",
		"POST /api/v1/auth/register/student",
		"POST /api/v1/auth/register/mentor",
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/refresh",
		"POST /api/v1/auth/logout",
		"POST /api/v1/auth/password/forgot",
		"POST /api/v1/auth/password/reset",
		"POST /api/v1/auth/teacher-invitations/activate",
		"GET /api/v1/me",
		"GET /api/v1/events",
		"GET /api/v1/student/organizations",
		"GET /api/v1/student/organizations/:organization_id/projects",
		"GET /api/v1/student/models",
		"GET /api/v1/student/usage",
		"POST /api/v1/student/api-keys",
		"GET /api/v1/student/api-keys",
		"GET /api/v1/student/api-keys/:api_key_id",
		"POST /api/v1/student/api-keys/:api_key_id/claim",
		"GET /api/v1/mentor/organizations",
		"GET /api/v1/mentor/organizations/:organization_id/projects",
		"POST /api/v1/mentor/project-applications",
		"GET /api/v1/mentor/project-applications",
		"GET /api/v1/mentor/api-key-reviews",
		"GET /api/v1/mentor/api-key-reviews/:api_key_id",
		"POST /api/v1/mentor/api-key-reviews/:api_key_id/approve",
		"POST /api/v1/mentor/api-key-reviews/:api_key_id/reject",
		"GET /api/v1/mentor/projects/:project_id/api-keys",
		"GET /api/v1/mentor/projects/:project_id/usage",
		"PATCH /api/v1/mentor/api-keys/:api_key_id/monthly-credit-quota",
		"GET /api/v1/mentor/call-logs",
		"GET /api/v1/mentor/call-logs/:call_id",
		"GET /api/v1/mentor/input-monitor",
		"GET /api/v1/mentor/input-monitor/:call_id",
		"POST /api/v1/mentor/api-keys/:api_key_id/revoke",
		"POST /api/v1/teacher/invitations",
		"GET /api/v1/teacher/organizations",
		"POST /api/v1/teacher/organizations",
		"PATCH /api/v1/teacher/organizations/:organization_id",
		"GET /api/v1/teacher/organizations/:organization_id/mentor-candidates",
		"POST /api/v1/teacher/organizations/:organization_id/mentors/:mentor_id",
		"GET /api/v1/teacher/projects",
		"GET /api/v1/teacher/project-spend",
		"POST /api/v1/teacher/projects",
		"PATCH /api/v1/teacher/projects/:project_id",
		"GET /api/v1/teacher/mentor-project-applications",
		"POST /api/v1/teacher/mentor-project-applications/:application_id/approve",
		"POST /api/v1/teacher/mentor-project-applications/:application_id/reject",
		"GET /api/v1/teacher/providers",
		"POST /api/v1/teacher/providers",
		"GET /api/v1/teacher/providers/:provider_id",
		"PATCH /api/v1/teacher/providers/:provider_id",
		"GET /api/v1/teacher/models",
		"POST /api/v1/teacher/models",
		"GET /api/v1/teacher/models/:model_id",
		"PATCH /api/v1/teacher/models/:model_id",
		"POST /api/v1/teacher/models/:model_id/bindings",
		"PATCH /api/v1/teacher/model-bindings/:binding_id",
		"GET /api/v1/teacher/api-key-reviews",
		"GET /api/v1/teacher/api-key-reviews/:api_key_id",
		"POST /api/v1/teacher/api-key-reviews/:api_key_id/approve",
		"POST /api/v1/teacher/api-key-reviews/:api_key_id/reject",
		"POST /v1/*path",
		"GET /v1/*path",
		"POST /v1beta/*path",
		"POST /v2/*path",
	}
	for _, route := range expected {
		if _, exists := routes[route]; !exists {
			t.Fatalf("expected route %s is not registered", route)
		}
	}
}
