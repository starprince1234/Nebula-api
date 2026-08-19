package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/starprince1234/Nebula-api/internal/dataplane"
)

type databasePingStub struct{ err error }

func (s databasePingStub) PingContext(context.Context) error { return s.err }

type cachePingStub struct{ err error }

func (s cachePingStub) Ping(context.Context) error { return s.err }

func TestHealthEndpoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		path       string
		health     HealthDependencies
		wantStatus int
	}{
		{name: "liveness", path: "/health/live", wantStatus: http.StatusOK},
		{
			name: "ready", path: "/health/ready",
			health:     HealthDependencies{Database: databasePingStub{}, Cache: cachePingStub{}},
			wantStatus: http.StatusOK,
		},
		{
			name: "database unavailable", path: "/health/ready",
			health:     HealthDependencies{Database: databasePingStub{err: errors.New("offline")}, Cache: cachePingStub{}},
			wantStatus: http.StatusServiceUnavailable,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := NewServer(nil, nil, nil, (*dataplane.Gateway)(nil), test.health, Config{})
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("expected status %d, got %d", test.wantStatus, response.Code)
			}
		})
	}
}
