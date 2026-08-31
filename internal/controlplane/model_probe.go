package controlplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/catalog"
	"github.com/starprince1234/Nebula-api/internal/domain"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/provider"
)

type ProbeInput struct {
	ProviderID                             *uuid.UUID
	BaseURL, Credential, UpstreamModelName string
	Endpoints                              []string
}
type ProbeLimits struct {
	ContextWindow   *int `json:"context_window,omitempty"`
	MaxInputTokens  *int `json:"max_input_tokens,omitempty"`
	MaxOutputTokens *int `json:"max_output_tokens,omitempty"`
}
type ProbeResult struct {
	Endpoint   string      `json:"endpoint"`
	Path       string      `json:"path"`
	HTTPStatus int         `json:"http_status"`
	DurationMS int64       `json:"duration_ms"`
	Truncated  bool        `json:"truncated"`
	Error      string      `json:"error,omitempty"`
	Limits     ProbeLimits `json:"limits"`
}
type ProbeConfiguration struct {
	Description      string   `json:"description,omitempty"`
	Category         string   `json:"category,omitempty"`
	Capabilities     []string `json:"capabilities,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	ProbeLimits
}
type ProbeResponse struct {
	Results       []ProbeResult      `json:"results"`
	Configuration ProbeConfiguration `json:"configuration"`
}
type probeRequest struct {
	Method    string
	Path      string
	Body      []byte
	Anthropic bool
}

func buildProbeRequest(endpoint, model string) (probeRequest, error) {
	var path string
	var payload any
	switch endpoint {
	case "chat_completions":
		path = "/v1/chat/completions"
		payload = map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": "Reply with OK."}}, "max_tokens": 1}
	case "responses":
		path = "/v1/responses"
		payload = map[string]any{"model": model, "input": "Reply with OK.", "max_output_tokens": 1}
	case "messages":
		path = "/v1/messages"
		payload = map[string]any{"model": model, "messages": []any{map[string]any{"role": "user", "content": "Reply with OK."}}, "max_tokens": 1}
	case "embeddings":
		path = "/v1/embeddings"
		payload = map[string]any{"model": model, "input": "probe"}
	case "rerank":
		path = "/v2/rerank"
		payload = map[string]any{"model": model, "query": "probe", "documents": []string{"probe"}, "top_n": 1}
	default:
		return probeRequest{}, domain.NewError(domain.CodeValidation, "invalid probe endpoint")
	}
	body, _ := json.Marshal(payload)
	return probeRequest{Method: http.MethodPost, Path: path, Body: body, Anthropic: endpoint == "messages"}, nil
}

func modelCatalogProbeRequest() probeRequest {
	return probeRequest{Method: http.MethodGet, Path: "/v1/models"}
}

func extractProbeLimits(body []byte) ProbeLimits {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return ProbeLimits{}
	}
	result := ProbeLimits{}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, item := range typed {
				normalized := strings.ToLower(key)
				if number, ok := item.(float64); ok && number > 0 && number <= 1_000_000_000 {
					v := int(number)
					switch normalized {
					case "context_window", "context_length", "max_context_tokens":
						if result.ContextWindow == nil {
							result.ContextWindow = &v
						}
					case "max_input_tokens", "input_token_limit":
						if result.MaxInputTokens == nil {
							result.MaxInputTokens = &v
						}
					case "max_output_tokens", "output_token_limit":
						if result.MaxOutputTokens == nil {
							result.MaxOutputTokens = &v
						}
					}
				}
				walk(item)
			}
		case []any:
			for _, item := range typed {
				walk(item)
			}
		}
	}
	walk(value)
	return result
}
func mergeLimits(target *ProbeLimits, source ProbeLimits) {
	if target.ContextWindow == nil {
		target.ContextWindow = source.ContextWindow
	}
	if target.MaxInputTokens == nil {
		target.MaxInputTokens = source.MaxInputTokens
	}
	if target.MaxOutputTokens == nil {
		target.MaxOutputTokens = source.MaxOutputTokens
	}
}

func stringsFromProbeValue(value any) []string {
	result := []string{}
	appendValue := func(value string) {
		for _, item := range strings.Split(value, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			seen := false
			for _, current := range result {
				if strings.EqualFold(current, item) {
					seen = true
					break
				}
			}
			if !seen {
				result = append(result, item)
			}
		}
	}
	switch typed := value.(type) {
	case string:
		appendValue(typed)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				appendValue(text)
			}
		}
	}
	return result
}

func modelRecords(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case map[string]any:
		for _, key := range []string{"data", "models"} {
			if records, ok := typed[key].([]any); ok {
				return records
			}
		}
	}
	return nil
}

func extractProbeConfiguration(body []byte, modelID string) ProbeConfiguration {
	var payload any
	if json.Unmarshal(body, &payload) != nil {
		return ProbeConfiguration{}
	}
	for _, value := range modelRecords(payload) {
		record, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id, _ := record["id"].(string)
		if id == "" {
			id, _ = record["model"].(string)
		}
		if !strings.EqualFold(strings.TrimSpace(id), strings.TrimSpace(modelID)) {
			continue
		}
		description, _ := record["description"].(string)
		description = catalog.PlainTextDescription(description)
		item := catalog.Item{ModelID: id, Description: description}
		item.ModelTypes = stringsFromProbeValue(record["model_types"])
		if len(item.ModelTypes) == 0 {
			item.ModelTypes = stringsFromProbeValue(record["model_type"])
		}
		item.Tags = stringsFromProbeValue(record["tags"])
		item.SupportedEndpointTypes = stringsFromProbeValue(record["supported_endpoint_types"])
		suggestion := catalog.SuggestConfiguration(item)
		raw, _ := json.Marshal(record)
		limits := extractProbeLimits(raw)
		return ProbeConfiguration{
			Description: description,
			Category:    suggestion.Category, Capabilities: suggestion.Capabilities,
			InputModalities: suggestion.InputModalities, OutputModalities: suggestion.OutputModalities,
			ProbeLimits: limits,
		}
	}
	return ProbeConfiguration{}
}

func isPublicIP(value string) bool {
	ip := net.ParseIP(strings.Trim(value, "[]"))
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified() && !ip.IsMulticast()
}
func publicHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 30 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !isPublicIP(ip.String()) {
				return nil, errors.New("target resolves to a non-public address")
			}
		}
		if len(ips) == 0 {
			return nil, errors.New("target has no address")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
	}}
	return &http.Client{Transport: transport, Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func validateProbeBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", domain.NewError(domain.CodeValidation, "invalid probe base URL")
	}
	if net.ParseIP(parsed.Hostname()) != nil && !isPublicIP(parsed.Hostname()) {
		return "", domain.NewError(domain.CodeValidation, "probe base URL must target a public address")
	}
	return value, nil
}

func probeTarget(baseURL, requestPath string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if (strings.HasSuffix(basePath, "/v1") && strings.HasPrefix(requestPath, "/v1/")) || (strings.HasSuffix(basePath, "/v2") && strings.HasPrefix(requestPath, "/v2/")) {
		requestPath = requestPath[3:]
	}
	parsed.Path = strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(requestPath, "/")
	return parsed.String(), nil
}

func (s *Service) ProbeModel(ctx context.Context, input ProbeInput) (ProbeResponse, error) {
	model := strings.TrimSpace(input.UpstreamModelName)
	if model == "" {
		return ProbeResponse{}, domain.NewError(domain.CodeValidation, "upstream_model_name is required")
	}
	baseURL, credential := strings.TrimSpace(input.BaseURL), input.Credential
	client := publicHTTPClient()
	if input.ProviderID != nil {
		row, err := s.db.Provider.Query().Where(provider.IDEQ(*input.ProviderID)).Only(ctx)
		if err != nil {
			return ProbeResponse{}, domain.NewError(domain.CodeNotFound, "provider not found")
		}
		baseURL = row.BaseURL
		credential, err = s.security.DecryptCredential(row.CredentialCiphertext)
		if err != nil {
			return ProbeResponse{}, domain.WrapError(domain.CodeDependencyUnavailable, "decrypt provider credential", err)
		}
		client = &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	} else {
		var err error
		baseURL, err = validateProbeBaseURL(baseURL)
		if err != nil {
			return ProbeResponse{}, err
		}
	}
	if strings.TrimSpace(credential) == "" {
		return ProbeResponse{}, domain.NewError(domain.CodeValidation, "probe credential is required")
	}
	if len(input.Endpoints) < 1 || len(input.Endpoints) > 5 {
		return ProbeResponse{}, domain.NewError(domain.CodeValidation, "probe endpoints must contain 1 to 5 items")
	}
	response := ProbeResponse{Results: []ProbeResult{}}
	definitions := []struct {
		endpoint   string
		definition probeRequest
	}{{endpoint: "models", definition: modelCatalogProbeRequest()}}
	for _, endpoint := range input.Endpoints {
		definition, err := buildProbeRequest(endpoint, model)
		if err != nil {
			return ProbeResponse{}, err
		}
		definitions = append(definitions, struct {
			endpoint   string
			definition probeRequest
		}{endpoint: endpoint, definition: definition})
	}
	for _, probe := range definitions {
		endpoint, definition := probe.endpoint, probe.definition
		target, err := probeTarget(baseURL, definition.Path)
		if err != nil {
			return ProbeResponse{}, domain.NewError(domain.CodeValidation, "invalid probe target")
		}
		req, err := http.NewRequestWithContext(ctx, definition.Method, target, bytes.NewReader(definition.Body))
		if err != nil {
			return ProbeResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		if definition.Anthropic {
			req.Header.Set("x-api-key", credential)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		started := time.Now()
		upstream, requestErr := client.Do(req)
		item := ProbeResult{Endpoint: endpoint, Path: definition.Path, DurationMS: time.Since(started).Milliseconds()}
		if requestErr != nil {
			item.Error = "request failed"
			response.Results = append(response.Results, item)
			continue
		}
		limited := io.LimitReader(upstream.Body, 65537)
		raw, _ := io.ReadAll(limited)
		_ = upstream.Body.Close()
		item.HTTPStatus = upstream.StatusCode
		if len(raw) > 65536 {
			raw = raw[:65536]
			item.Truncated = true
		}
		if upstream.StatusCode >= 200 && upstream.StatusCode < 300 {
			if endpoint == "models" {
				response.Configuration = extractProbeConfiguration(raw, model)
				item.Limits = response.Configuration.ProbeLimits
			} else {
				item.Limits = extractProbeLimits(raw)
				mergeLimits(&response.Configuration.ProbeLimits, item.Limits)
			}
		} else {
			item.Error = fmt.Sprintf("upstream returned HTTP %d", upstream.StatusCode)
		}
		response.Results = append(response.Results, item)
	}
	return response, nil
}
