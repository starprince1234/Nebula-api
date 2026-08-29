package dataplane

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/modelbinding"
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
		{http.MethodPost, "/v2/rerank"},
		{http.MethodPost, "/v1/images/generations"},
		{http.MethodPost, "/v1/images/edits"},
		{http.MethodPost, "/v1/images/variations"},
		{http.MethodPost, "/v1/audio/transcriptions"},
		{http.MethodPost, "/v1/audio/translations"},
		{http.MethodPost, "/v1/audio/speech"},
		{http.MethodPost, "/v1/videos"},
		{http.MethodGet, "/v1/videos/video-123"},
		{http.MethodGet, "/v1/videos/video-123/content"},
		{http.MethodPost, "/v1/videos/video-123/remix"},
		{http.MethodPost, "/v1/moderations"},
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent"},
		{http.MethodPost, "/v1beta/models/gemini-2.5-pro:streamGenerateContent"},
		{http.MethodPost, "/v1beta/models/gemini-embedding-001:embedContent"},
		{http.MethodPost, "/v1beta/models/gemini-embedding-001:batchEmbedContents"},
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

func TestAdapterForPath(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"/v1/chat/completions":     "openai_compatible",
		"/v1/responses":            "openai_responses",
		"/v1/responses/compact":    "openai_responses",
		"/v1/embeddings":           "openai_embeddings",
		"/v1/images/generations":   "openai_images",
		"/v1/audio/transcriptions": "openai_audio",
		"/v1/videos":               "openai_video",
		"/v1/realtime":             "openai_realtime",
		"/v1/moderations":          "openai_moderations",
		"/v1/messages":             "anthropic",
		"/v2/rerank":               "cohere_rerank_v2",
		"/v1beta/models/gemini-2.5-pro:generateContent": "google_gemini_v1beta",
	}
	for requestPath, expected := range tests {
		if got := string(adapterForPath(requestPath)); got != expected {
			t.Fatalf("adapter for %s = %s, want %s", requestPath, got, expected)
		}
	}
}

func TestCodexResponsesBodyAndHeadersArePreserved(t *testing.T) {
	t.Parallel()
	replay, err := captureBody(io.NopCloser(strings.NewReader(`{"model":"public-codex","input":[{"type":"compaction","encrypted_content":"opaque"}],"context_management":[{"type":"compaction","compact_threshold":200000}],"prompt_cache_key":"thread-1","service_tier":"priority","include":["reasoning.encrypted_content"],"store":false,"stream":true}`)), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	original := httptest.NewRequest(http.MethodPost, "https://nebula.example/v1/responses", nil)
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("Authorization", "Bearer nebula-key")
	original.Header.Set("OpenAI-Beta", "responses_websockets=2026-02-06")
	original.Header.Set("x-codex-turn-state", "turn-state")
	request, cleanup, err := (&Gateway{}).upstreamRequest(original, replay, route{
		Binding:  &ent.ModelBinding{UpstreamModelName: "gpt-codex-upstream", Adapter: modelbinding.AdapterOpenaiResponses},
		Provider: &ent.Provider{BaseURL: "https://api.openai.com/v1"}, Credential: "provider-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if request.Header.Get("OpenAI-Beta") != "responses_websockets=2026-02-06" || request.Header.Get("x-codex-turn-state") != "turn-state" {
		t.Fatalf("Codex headers were not preserved: %#v", request.Header)
	}
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"model":"gpt-codex-upstream"`, `"encrypted_content":"opaque"`, `"prompt_cache_key":"thread-1"`, `"service_tier":"priority"`, `"context_management"`} {
		if !bytes.Contains(raw, []byte(field)) {
			t.Fatalf("Responses field %s was lost: %s", field, raw)
		}
	}
}

func TestOfficialVideoResourceRouting(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		method, path, id string
	}{
		{http.MethodGet, "/v1/videos/video-1", "video-1"},
		{http.MethodGet, "/v1/videos/video-1/content", "video-1"},
		{http.MethodPost, "/v1/videos/video-1/remix", "video-1"},
	} {
		resourceType, resourceID, ok := routedResource(test.method, test.path)
		if !ok || resourceType != "video" || resourceID != test.id {
			t.Fatalf("unexpected resource route for %s %s: %s %s %t", test.method, test.path, resourceType, resourceID, ok)
		}
	}
}

func TestResponsesWebSocketModelRewritePreservesCodexFields(t *testing.T) {
	t.Parallel()
	message := []byte(`{"type":"response.create","model":"public-codex","previous_response_id":"resp-1","input":[{"type":"compaction","encrypted_content":"opaque"}],"prompt_cache_key":"thread-1","client_metadata":{"session_id":"session-1"}}`)
	model, err := responsesWebSocketModel(message)
	if err != nil || model != "public-codex" {
		t.Fatalf("unexpected model %q, err=%v", model, err)
	}
	rewritten, err := rewriteResponsesWebSocketModel(message, "gpt-codex-upstream")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"model":"gpt-codex-upstream"`, `"previous_response_id":"resp-1"`, `"encrypted_content":"opaque"`, `"prompt_cache_key":"thread-1"`, `"session_id":"session-1"`} {
		if !bytes.Contains(rewritten, []byte(field)) {
			t.Fatalf("Responses WebSocket field %s was lost: %s", field, rewritten)
		}
	}
}

func TestGeminiModelAndUpstreamPath(t *testing.T) {
	t.Parallel()
	requestPath := "/v1beta/models/public-gemini:streamGenerateContent"
	model, err := extractRequestedModel(requestPath, "application/json", nil)
	if err != nil || model != "public-gemini" {
		t.Fatalf("unexpected model %q, err=%v", model, err)
	}
	upstream, err := upstreamPath(requestPath, "models/gemini-2.5-pro", "google_gemini_v1beta")
	if err != nil || upstream != "/v1beta/models/gemini-2.5-pro:streamGenerateContent" {
		t.Fatalf("unexpected upstream path %q, err=%v", upstream, err)
	}
	joined, err := joinUpstreamURL("https://generativelanguage.googleapis.com/v1beta", upstream, "alt=sse")
	if err != nil || joined != "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse" {
		t.Fatalf("unexpected joined URL %q, err=%v", joined, err)
	}
}

func TestCohereRerankUsesV2Upstream(t *testing.T) {
	t.Parallel()
	upstream, err := upstreamPath("/v2/rerank", "rerank-v4.0-pro", "cohere_rerank_v2")
	if err != nil || upstream != "/v2/rerank" {
		t.Fatalf("unexpected upstream path %q, err=%v", upstream, err)
	}
}

func TestRewriteGeminiBatchModel(t *testing.T) {
	t.Parallel()
	replay, err := captureBody(io.NopCloser(strings.NewReader(`{"requests":[{"model":"models/public-model","content":{"parts":[{"text":"hello"}]}}]}`)), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	body, _, _, cleanup, err := rewriteUpstreamBody("application/json", replay, "gemini-embedding-001", "google_gemini_v1beta")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	raw, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"model":"models/gemini-embedding-001"`)) || !bytes.Contains(raw, []byte(`"text":"hello"`)) {
		t.Fatalf("rewritten Gemini JSON lost fields: %s", raw)
	}
}

func TestGeminiUpstreamRequestUsesHeaderCredentialAndRewritesModels(t *testing.T) {
	t.Parallel()
	replay, err := captureBody(io.NopCloser(strings.NewReader(`{"requests":[{"model":"models/public-model","content":{"parts":[{"text":"hello"}]}}]}`)), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	original := httptest.NewRequest(http.MethodPost, "https://nebula.example/v1beta/models/public-model:batchEmbedContents", nil)
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("x-goog-api-key", "nebula-client-key")
	request, cleanup, err := (&Gateway{}).upstreamRequest(original, replay, route{
		Binding:    &ent.ModelBinding{UpstreamModelName: "gemini-embedding-001", Adapter: modelbinding.AdapterGoogleGeminiV1beta},
		Provider:   &ent.Provider{BaseURL: "https://generativelanguage.googleapis.com/v1beta"},
		Credential: "provider-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if request.URL.String() != "https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:batchEmbedContents" {
		t.Fatalf("unexpected target %q", request.URL)
	}
	if request.Header.Get("x-goog-api-key") != "provider-key" || request.Header.Get("Authorization") != "" {
		t.Fatalf("unexpected upstream authentication headers: %#v", request.Header)
	}
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"model":"models/gemini-embedding-001"`)) || bytes.Contains(raw, []byte("public-model")) {
		t.Fatalf("unexpected upstream body: %s", raw)
	}
}

func TestCohereUpstreamRequestUsesBearerCredential(t *testing.T) {
	t.Parallel()
	replay, err := captureBody(io.NopCloser(strings.NewReader(`{"model":"public-rerank","query":"q","documents":["a","b"]}`)), 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	original := httptest.NewRequest(http.MethodPost, "https://nebula.example/v2/rerank", nil)
	original.Header.Set("Content-Type", "application/json")
	original.Header.Set("Authorization", "Bearer nebula-client-key")
	request, cleanup, err := (&Gateway{}).upstreamRequest(original, replay, route{
		Binding:    &ent.ModelBinding{UpstreamModelName: "rerank-v4.0-pro", Adapter: modelbinding.AdapterCohereRerankV2},
		Provider:   &ent.Provider{BaseURL: "https://api.cohere.com/v2"},
		Credential: "provider-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if request.URL.String() != "https://api.cohere.com/v2/rerank" {
		t.Fatalf("unexpected target %q", request.URL)
	}
	if request.Header.Get("Authorization") != "Bearer provider-key" {
		t.Fatalf("unexpected Authorization header %q", request.Header.Get("Authorization"))
	}
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"model":"rerank-v4.0-pro"`)) || bytes.Contains(raw, []byte("public-rerank")) {
		t.Fatalf("unexpected upstream body: %s", raw)
	}
}

func TestGeminiRejectsQueryAPIKeyBeforeAuthentication(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:generateContent?key=client-secret", strings.NewReader(`{"contents":[]}`))
	recorder := httptest.NewRecorder()
	(&Gateway{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "API keys are not accepted in query parameters") {
		t.Fatalf("unexpected response %d: %s", recorder.Code, recorder.Body.String())
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

func TestExtractInputSnapshotUsesLastUserText(t *testing.T) {
	t.Parallel()
	replay, err := captureBody(io.NopCloser(strings.NewReader(`{"model":"gpt","messages":[{"role":"user","content":"first"},{"role":"assistant","content":"answer"},{"role":"user","content":[{"type":"text","text":"second-a"},{"type":"image_url","image_url":"data:image/png;base64,..."},{"type":"text","text":"second-b"}]}]}`)), 4096)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Close()
	text, source := extractInputSnapshot("/v1/chat/completions", "application/json", replay)
	if text != "second-a\nsecond-b" || source != "user_message" {
		t.Fatalf("unexpected snapshot %q (%s)", text, source)
	}
}

func TestExtractInputSnapshotSkipsUnmonitoredRoutes(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/v1/embeddings", "/v1/moderations", "/v2/rerank", "/v1/responses/compact", "/v1/images/variations", "/v1/audio/transcriptions", "/v1/audio/translations"} {
		replay, err := captureBody(io.NopCloser(strings.NewReader(`{"model":"gpt","prompt":"sensitive","input":"sensitive"}`)), 1024)
		if err != nil {
			t.Fatal(err)
		}
		text, source := extractInputSnapshot(path, "application/json", replay)
		_ = replay.Close()
		if text != "" || source != "" {
			t.Fatalf("route %s generated a monitored input", path)
		}
	}
}
