package httpapi

import (
	"net/http"
	"testing"

	"github.com/starprince1234/Nebula-api/internal/domain"
)

func TestErrorStatusMapping(t *testing.T) {
	t.Parallel()
	tests := map[string]int{
		domain.CodeValidation:             http.StatusBadRequest,
		domain.CodeAuthenticationRequired: http.StatusUnauthorized,
		domain.CodeForbidden:              http.StatusForbidden,
		domain.CodeNotFound:               http.StatusNotFound,
		domain.CodeModelNotReady:          http.StatusConflict,
		domain.CodeModelRoutingRequired:   http.StatusConflict,
		domain.CodeVerificationInvalid:    http.StatusUnprocessableEntity,
		domain.CodeRateLimited:            http.StatusTooManyRequests,
		domain.CodeDependencyUnavailable:  http.StatusServiceUnavailable,
	}
	for code, expected := range tests {
		if actual := errorStatus(code); actual != expected {
			t.Fatalf("code %s: expected %d, got %d", code, expected, actual)
		}
	}
}
