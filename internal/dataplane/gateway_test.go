package dataplane

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestSupportedRoutesExcludeLegacyAliases(t *testing.T) {
	t.Parallel()
	expected := [][2]string{
		{http.MethodGet, "/v1/models"},
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/v1/completions"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodPost, "/v1/responses/compact"},
		{http.MethodPost, "/v1/embeddings"},
		{http.MethodPost, "/v1/images/generations"},
		{http.MethodPost, "/v1/images/edits"},
		{http.MethodPost, "/v1/audio/transcriptions"},
		{http.MethodPost, "/v1/audio/translations"},
		{http.MethodPost, "/v1/audio/speech"},
		{http.MethodPost, "/v1/video/generations"},
		{http.MethodGet, "/v1/video/generations/task-123"},
		{http.MethodPost, "/v1/messages"},
		{http.MethodGet, "/v1/realtime"},
	}
	for _, route := range expected {
		if !supportedRoute(route[0], route[1]) {
			t.Fatalf("standard route %s %s is missing", route[0], route[1])
		}
	}
	for _, route := range []string{"/chat/completions", "/api/nebula/gateway/v1/chat/completions", "/internal/usage"} {
		if supportedRoute(http.MethodPost, route) {
			t.Fatalf("legacy or internal route %q was accepted", route)
		}
	}
}

func TestJoinUpstreamURL(t *testing.T) {
	t.Parallel()
	got, err := joinUpstreamURL("https://provider.example/v1", "/v1/responses", "trace=1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://provider.example/v1/responses?trace=1" {
		t.Fatalf("unexpected upstream URL %q", got)
	}
}

func TestRewriteJSONModel(t *testing.T) {
	t.Parallel()
	replay, err := captureBody(io.NopCloser(strings.NewReader(`{"model":"public-model","input":"hello"}`)), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	if model, err := extractModel("application/json", replay); err != nil || model != "public-model" {
		t.Fatalf("unexpected model %q, err=%v", model, err)
	}
	body, _, _, cleanup, err := rewriteRequestBody("application/json", replay, "upstream-model")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"model":"upstream-model"`)) || !bytes.Contains(raw, []byte(`"input":"hello"`)) {
		t.Fatalf("rewritten JSON lost fields: %s", raw)
	}
}

func TestRewriteMultipartModelPreservesFile(t *testing.T) {
	t.Parallel()
	var original bytes.Buffer
	writer := multipart.NewWriter(&original)
	if err := writer.WriteField("model", "public-model"); err != nil {
		t.Fatal(err)
	}
	file, err := writer.CreateFormFile("file", "sample.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("file-content")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	contentType := writer.FormDataContentType()
	replay, err := captureBody(io.NopCloser(bytes.NewReader(original.Bytes())), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	body, rewrittenType, _, cleanup, err := rewriteRequestBody(contentType, replay, "upstream-model")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	_, params, err := mime.ParseMediaType(rewrittenType)
	if err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(body, params["boundary"])
	values := map[string]string{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		values[part.FormName()] = string(raw)
	}
	if values["model"] != "upstream-model" || values["file"] != "file-content" {
		t.Fatalf("multipart rewrite changed payload: %#v", values)
	}
}

func TestCaptureBodyLimit(t *testing.T) {
	t.Parallel()
	if _, err := captureBody(io.NopCloser(strings.NewReader("12345")), 4); err == nil {
		t.Fatal("oversized request body was accepted")
	}
}
