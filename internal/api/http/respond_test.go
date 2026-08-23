package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/starprince1234/Nebula-api/internal/domain"
)

func TestErrorStatusMapping(t *testing.T) {
	t.Parallel()
	tests := map[string]int{
		domain.CodeValidation:                 http.StatusBadRequest,
		domain.CodeRejectionReasonRequired:    http.StatusBadRequest,
		domain.CodeAuthenticationRequired:     http.StatusUnauthorized,
		domain.CodeForbidden:                  http.StatusForbidden,
		domain.CodeNotFound:                   http.StatusNotFound,
		domain.CodeModelNotReady:              http.StatusConflict,
		domain.CodeModelRoutingRequired:       http.StatusConflict,
		domain.CodeMentorNotInOrganization:    http.StatusConflict,
		domain.CodeMentorAlreadyProjectMember: http.StatusConflict,
		domain.CodeVerificationInvalid:        http.StatusUnprocessableEntity,
		domain.CodeRateLimited:                http.StatusTooManyRequests,
		domain.CodeDependencyUnavailable:      http.StatusServiceUnavailable,
	}
	for code, expected := range tests {
		if actual := errorStatus(code); actual != expected {
			t.Fatalf("code %s: expected %d, got %d", code, expected, actual)
		}
	}
}

func TestBindOptionalCommentAcceptsEmptyHTTPBody(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "   \n\t"} {
		t.Run(body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
			comment, ok := bindOptionalComment(c)
			if !ok || comment != "" {
				t.Fatalf("expected empty optional comment, got %q (ok=%v)", comment, ok)
			}
		})
	}
}

func TestBindOptionalCommentRestoresJSONBody(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"comment":"通过"}`))
	comment, ok := bindOptionalComment(c)
	if !ok || comment != "通过" {
		t.Fatalf("expected comment to be decoded, got %q (ok=%v)", comment, ok)
	}
	if _, err := io.ReadAll(c.Request.Body); err != nil {
		t.Fatal(err)
	}
}
