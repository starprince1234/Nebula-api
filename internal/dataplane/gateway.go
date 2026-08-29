package dataplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/cache"
	security "github.com/starprince1234/Nebula-api/internal/infrastructure/crypto"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikey"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/apikeymodel"
	entmodel "github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/model"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/modelbinding"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/provider"
	"github.com/starprince1234/Nebula-api/internal/usage"
)

const replayMemoryThreshold = 1 << 20

type Config struct {
	ConnectTimeout        time.Duration
	ResponseHeaderTimeout time.Duration
	MaxRequestBytes       int64
	ResourceRouteTTL      time.Duration
	AllowedOrigin         string
}

type Gateway struct {
	db       *ent.Client
	cache    *cache.Store
	security *security.Manager
	usage    *usage.Store
	client   *http.Client
	dialer   *websocket.Dialer
	config   Config
}

type route struct {
	Binding    *ent.ModelBinding
	Provider   *ent.Provider
	Credential string
}

func NewGateway(db *ent.Client, usageStore *usage.Store, cacheStore *cache.Store, securityManager *security.Manager, cfg Config) *Gateway {
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: cfg.ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   cfg.ConnectTimeout,
		ResponseHeaderTimeout: cfg.ResponseHeaderTimeout,
		ExpectContinueTimeout: time.Second,
	}
	return &Gateway{
		db: db, usage: usageStore, cache: cacheStore, security: securityManager,
		client: &http.Client{Transport: transport},
		dialer: &websocket.Dialer{
			Proxy:             http.ProxyFromEnvironment,
			HandshakeTimeout:  cfg.ResponseHeaderTimeout,
			EnableCompression: true,
		},
		config: cfg,
	}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if isWebSocketUpgrade(r) && (r.URL.Path == "/v1/realtime" || r.URL.Path == "/v1/responses") {
		g.serveWebSocket(w, r)
		return
	}
	if !supportedRoute(r.Method, r.URL.Path) {
		writeOpenAIError(w, http.StatusNotFound, "invalid_request_error", "route_not_found", "Route not found.", "")
		return
	}
	if isGeminiPath(r.URL.Path) && r.URL.Query().Has("key") {
		writeGeminiError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "API keys are not accepted in query parameters.")
		return
	}
	key, err := g.authenticate(r)
	if err != nil {
		writeProtocolError(w, r.URL.Path, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "Invalid API key.", "")
		return
	}
	if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
		g.serveModels(w, r, key)
		return
	}
	var replay *replayBody
	if r.Body != nil {
		replay, err = captureBody(r.Body, g.config.MaxRequestBytes)
		if err != nil {
			writeProtocolError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "invalid_request_body", err.Error(), "")
			return
		}
		defer replay.Close()
	}

	adapter := adapterForPath(r.URL.Path)
	if adapter == modelbinding.AdapterAnthropic {
		if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
			writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "anthropic-version header is required")
			return
		}
	}
	if resourceType, resourceID, ok := routedResource(r.Method, r.URL.Path); ok {
		routes, routeErr := g.resourceRoutes(r.Context(), key.ID, resourceType, resourceID, adapter)
		if routeErr != nil || len(routes) == 0 {
			writeProtocolError(w, r.URL.Path, http.StatusNotFound, "invalid_request_error", "resource_not_found", "The requested upstream resource is unavailable.", "")
			return
		}
		g.proxyHTTP(w, r, replay, routes, uuid.Nil)
		return
	}

	modelID, extractErr := extractRequestedModel(r.URL.Path, r.Header.Get("Content-Type"), replay)
	if extractErr != nil {
		writeProtocolError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "invalid_model", extractErr.Error(), "model")
		return
	}
	routes, err := g.routesForModel(r.Context(), key.ID, modelID, adapter)
	if err != nil || len(routes) == 0 {
		writeProtocolError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "model_not_allowed", "The requested model is not allowed or is unavailable for this API key.", "model")
		return
	}
	prompt, promptSource := extractInputSnapshot(r.URL.Path, r.Header.Get("Content-Type"), replay)
	reservation, reserveErr := g.usage.Reserve(r.Context(), key.ID, routes[0].Binding.ModelID, requestIDFromContext(r), string(adapter), r.URL.Path, prompt, promptSource)
	if reserveErr != nil {
		var quotaErr *usage.QuotaError
		if errors.As(reserveErr, &quotaErr) {
			writeProtocolError(w, r.URL.Path, http.StatusTooManyRequests, "rate_limit_error", quotaErr.Code, fmt.Sprintf("%s quota is exhausted until %s.", quotaErr.Name, quotaErr.EndsAt.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02 15:04 MST")), "")
			return
		}
		writeProtocolError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "billing_unavailable", "Unable to authorize this model call.", "model")
		return
	}
	g.proxyHTTP(w, r, replay, routes, reservation)
}

func requestIDFromContext(r *http.Request) string {
	if value := usage.RequestID(r.Context()); value != "" {
		return value
	}
	return uuid.Must(uuid.NewV7()).String()
}

func (g *Gateway) authenticate(r *http.Request) (*ent.APIKey, error) {
	raw := ""
	if r.URL.Path == "/v1/messages" {
		raw = strings.TrimSpace(r.Header.Get("x-api-key"))
	} else if isGeminiPath(r.URL.Path) {
		raw = strings.TrimSpace(r.Header.Get("x-goog-api-key"))
	}
	if raw == "" {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if len(authorization) > 7 && strings.EqualFold(authorization[:7], "Bearer ") {
			raw = strings.TrimSpace(authorization[7:])
		}
	}
	if raw == "" {
		return nil, errors.New("missing API key")
	}
	hash := g.security.HashAPIKey(raw)
	key, err := g.db.APIKey.Query().Where(
		apikey.KeyHashEQ(hash),
		apikey.StatusEQ(apikey.StatusActive),
	).Only(r.Context())
	if err != nil {
		return nil, errors.New("invalid API key")
	}
	return key, nil
}

func (g *Gateway) serveModels(w http.ResponseWriter, r *http.Request, key *ent.APIKey) {
	rows, err := g.db.Model.Query().Where(
		entmodel.StatusEQ(entmodel.StatusActive),
		entmodel.HasAPIKeyModelsWith(apikeymodel.APIKeyIDEQ(key.ID)),
		entmodel.HasBindingsWith(
			modelbinding.StatusEQ(modelbinding.StatusActive),
			modelbinding.HasProviderWith(provider.StatusEQ(provider.StatusActive)),
		),
	).Order(ent.Asc(entmodel.FieldModelID)).All(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", "dependency_unavailable", "Model catalog is unavailable.", "")
		return
	}
	data := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		data = append(data, map[string]any{
			"id": row.ModelID, "object": "model", "created": row.CreatedAt.Unix(), "owned_by": "nebula",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (g *Gateway) routesForModel(
	ctx context.Context,
	keyID uuid.UUID,
	modelID string,
	adapter modelbinding.Adapter,
) ([]route, error) {
	allowed, err := g.db.APIKeyModel.Query().Where(
		apikeymodel.APIKeyIDEQ(keyID),
		apikeymodel.HasModelWith(
			entmodel.ModelIDEQ(modelID),
			entmodel.StatusEQ(entmodel.StatusActive),
		),
	).WithModel().Only(ctx)
	if err != nil || allowed.Edges.Model == nil {
		return nil, errors.New("model is not allowed")
	}
	bindings, err := g.db.ModelBinding.Query().Where(
		modelbinding.ModelIDEQ(allowed.ModelID),
		modelbinding.AdapterEQ(adapter),
		modelbinding.StatusEQ(modelbinding.StatusActive),
		modelbinding.HasProviderWith(provider.StatusEQ(provider.StatusActive)),
	).WithProvider().Order(
		ent.Asc(modelbinding.FieldPriority),
		ent.Asc(modelbinding.FieldID),
	).All(ctx)
	if err != nil {
		return nil, err
	}
	return g.decryptRoutes(bindings), nil
}

func (g *Gateway) resourceRoutes(ctx context.Context, keyID uuid.UUID, resourceType, resourceID string, adapter modelbinding.Adapter) ([]route, error) {
	bindingIDRaw, err := g.cache.GetGatewayResourceRoute(ctx, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	bindingID, err := uuid.Parse(bindingIDRaw)
	if err != nil {
		return nil, err
	}
	binding, err := g.db.ModelBinding.Query().Where(
		modelbinding.IDEQ(bindingID),
		modelbinding.AdapterEQ(adapter),
		modelbinding.StatusEQ(modelbinding.StatusActive),
		modelbinding.HasProviderWith(provider.StatusEQ(provider.StatusActive)),
		modelbinding.HasModelWith(
			entmodel.StatusEQ(entmodel.StatusActive),
			entmodel.HasAPIKeyModelsWith(apikeymodel.APIKeyIDEQ(keyID)),
		),
	).WithProvider().Only(ctx)
	if err != nil {
		return nil, err
	}
	return g.decryptRoutes([]*ent.ModelBinding{binding}), nil
}

func (g *Gateway) decryptRoutes(bindings []*ent.ModelBinding) []route {
	routes := make([]route, 0, len(bindings))
	for _, binding := range bindings {
		upstream := binding.Edges.Provider
		if upstream == nil || upstream.Status != provider.StatusActive {
			continue
		}
		credential, err := g.security.DecryptCredential(upstream.CredentialCiphertext)
		if err != nil || strings.TrimSpace(credential) == "" {
			continue
		}
		routes = append(routes, route{Binding: binding, Provider: upstream, Credential: credential})
	}
	return routes
}

func (g *Gateway) proxyHTTP(w http.ResponseWriter, original *http.Request, replay *replayBody, routes []route, reservation any) {
	var lastResponse *http.Response
	for index, candidate := range routes {
		attemptStarted := time.Now()
		var attemptID uuid.UUID
		if _, ok := reservation.(usage.Reservation); ok {
			attemptID, _ = g.usage.StartAttempt(original.Context(), reservation.(usage.Reservation).CallID, candidate.Provider.ID, candidate.Binding.ID, candidate.Provider.Name)
		}
		request, cleanup, err := g.upstreamRequest(original, replay, candidate)
		if err != nil {
			if attemptID != uuid.Nil {
				_ = g.usage.FinishAttempt(original.Context(), attemptID, false, nil, "request_rewrite", err.Error(), attemptStarted)
			}
			continue
		}
		if r, ok := reservation.(usage.Reservation); ok {
			_ = g.usage.MarkSent(original.Context(), r.CallID)
		}
		response, err := g.client.Do(request)
		cleanup()
		if err != nil {
			if attemptID != uuid.Nil {
				_ = g.usage.FinishAttempt(original.Context(), attemptID, false, nil, "connection", err.Error(), attemptStarted)
			}
			continue
		}
		if attemptID != uuid.Nil {
			_ = g.usage.FinishAttempt(original.Context(), attemptID, response.StatusCode >= 200 && response.StatusCode < 300, &response.StatusCode, "", "", attemptStarted)
		}
		lastResponse = response
		canRetry := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if canRetry && index < len(routes)-1 {
			_, _ = io.CopyN(io.Discard, response.Body, 64<<10)
			_ = response.Body.Close()
			continue
		}
		if r, ok := reservation.(usage.Reservation); ok && isSSEResponse(response) {
			g.proxySSE(w, original, response, r, candidate.Provider)
			return
		}
		if r, ok := reservation.(usage.Reservation); ok {
			success := response.StatusCode >= 200 && response.StatusCode < 300
			if success {
				_ = g.usage.Finalize(original.Context(), r, true, "succeeded", "", "", &candidate.Provider.ID, candidate.Provider.Name)
			} else {
				_ = g.usage.Finalize(original.Context(), r, false, "upstream_failed", "http", response.Status, &candidate.Provider.ID, candidate.Provider.Name)
			}
		}
		if resourceType, ok := createdResourceType(original.Method, original.URL.Path); ok && response.StatusCode >= 200 && response.StatusCode < 300 {
			g.writeResourceCreationResponse(w, response, resourceType, candidate.Binding.ID)
			return
		}
		copyUpstreamResponse(w, response)
		return
	}
	if lastResponse != nil {
		if r, ok := reservation.(usage.Reservation); ok {
			_ = g.usage.Finalize(original.Context(), r, false, "upstream_failed", "http", fmt.Sprintf("HTTP %d", lastResponse.StatusCode), nil, "")
		}
		copyUpstreamResponse(w, lastResponse)
		return
	}
	writeProtocolError(w, original.URL.Path, http.StatusBadGateway, "server_error", "upstream_unavailable", "No upstream provider is available.", "")
	if r, ok := reservation.(usage.Reservation); ok {
		_ = g.usage.Finalize(original.Context(), r, false, "upstream_failed", "connection", "No upstream provider is available.", nil, "")
	}
}

func isSSEResponse(response *http.Response) bool {
	return strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
}

func (g *Gateway) proxySSE(w http.ResponseWriter, original *http.Request, response *http.Response, reservation usage.Reservation, provider *ent.Provider) {
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	var buffered bytes.Buffer
	confirmed := false
	for !confirmed {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			buffered.Write(line)
			trimmed := bytes.TrimSpace(line)
			if bytes.HasPrefix(trimmed, []byte("data:")) && !bytes.Contains(bytes.ToLower(trimmed), []byte("error")) {
				confirmed = true
			}
		}
		if err != nil {
			if !confirmed {
				_ = g.usage.Finalize(original.Context(), reservation, false, "upstream_failed", "stream", "stream ended before a successful event", nil, provider.Name)
			}
			copyHeaders(w.Header(), response.Header)
			w.WriteHeader(response.StatusCode)
			_, _ = w.Write(buffered.Bytes())
			return
		}
	}
	if err := g.usage.Finalize(original.Context(), reservation, true, "succeeded", "", "", &provider.ID, provider.Name); err != nil {
		writeProtocolError(w, original.URL.Path, http.StatusServiceUnavailable, "server_error", "billing_unavailable", "Unable to finalize model usage.", "")
		return
	}
	copyHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(buffered.Bytes())
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	_, _ = io.Copy(w, reader)
}

func (g *Gateway) upstreamRequest(original *http.Request, replay *replayBody, candidate route) (*http.Request, func(), error) {
	requestPath, err := upstreamPath(original.URL.Path, candidate.Binding.UpstreamModelName, candidate.Binding.Adapter)
	if err != nil {
		return nil, func() {}, err
	}
	target, err := joinUpstreamURL(candidate.Provider.BaseURL, requestPath, original.URL.RawQuery)
	if err != nil {
		return nil, func() {}, err
	}
	body, contentType, contentLength, cleanup, err := rewriteUpstreamBody(
		original.Header.Get("Content-Type"), replay, candidate.Binding.UpstreamModelName, candidate.Binding.Adapter,
	)
	if err != nil {
		return nil, func() {}, err
	}
	request, err := http.NewRequestWithContext(original.Context(), original.Method, target, body)
	if err != nil {
		if body != nil {
			_ = body.Close()
		}
		cleanup()
		return nil, func() {}, err
	}
	request.Header = original.Header.Clone()
	removeHopByHop(request.Header)
	request.Header.Del("Content-Length")
	request.Header.Del("Authorization")
	request.Header.Del("x-api-key")
	request.Header.Del("x-goog-api-key")
	if candidate.Binding.Adapter == modelbinding.AdapterAnthropic {
		request.Header.Set("x-api-key", candidate.Credential)
	} else if candidate.Binding.Adapter == modelbinding.AdapterGoogleGeminiV1beta {
		request.Header.Set("x-goog-api-key", candidate.Credential)
	} else {
		request.Header.Set("Authorization", "Bearer "+candidate.Credential)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Host = request.URL.Host
	request.ContentLength = contentLength
	return request, cleanup, nil
}

func (g *Gateway) writeResourceCreationResponse(w http.ResponseWriter, response *http.Response, resourceType string, bindingID uuid.UUID) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "server_error", "invalid_upstream_response", "Invalid upstream resource response.", "")
		return
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		resourceID := firstString(payload, "id", "task_id")
		if resourceID != "" {
			_ = g.cache.SetGatewayResourceRoute(context.Background(), resourceType, resourceID, bindingID.String(), g.config.ResourceRouteTTL)
		}
	}
	copyHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}

func (g *Gateway) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/v1/responses" {
		g.serveResponsesWebSocket(w, r)
		return
	}
	g.serveRealtimeWebSocket(w, r)
}

func (g *Gateway) serveRealtimeWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("api_key") != "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "api_key_query_forbidden", "API keys are not accepted in query parameters.", "")
		return
	}
	key, err := g.authenticateRealtime(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "Invalid API key.", "")
		return
	}
	modelID := strings.TrimSpace(r.URL.Query().Get("model"))
	if modelID == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid_model", "model query parameter is required.", "model")
		return
	}
	routes, err := g.routesForModel(r.Context(), key.ID, modelID, modelbinding.AdapterOpenaiRealtime)
	if err != nil || len(routes) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "model_not_allowed", "No allowed model is available for this protocol.", "model")
		return
	}
	var upstream *websocket.Conn
	for _, candidate := range routes {
		query := r.URL.Query()
		query.Set("model", candidate.Binding.UpstreamModelName)
		target, err := joinUpstreamURL(candidate.Provider.BaseURL, r.URL.Path, query.Encode())
		if err != nil {
			continue
		}
		target = strings.Replace(target, "https://", "wss://", 1)
		target = strings.Replace(target, "http://", "ws://", 1)
		headers := r.Header.Clone()
		removeHopByHop(headers)
		headers.Del("Authorization")
		headers.Del("Sec-WebSocket-Key")
		headers.Del("Sec-WebSocket-Version")
		headers.Del("Sec-WebSocket-Extensions")
		headers.Del("Sec-WebSocket-Protocol")
		headers.Set("Authorization", "Bearer "+candidate.Credential)
		upstream, _, err = g.dialer.DialContext(r.Context(), target, headers)
		if err == nil {
			break
		}
	}
	if upstream == nil {
		writeOpenAIError(w, http.StatusBadGateway, "server_error", "upstream_unavailable", "No WebSocket provider is available.", "")
		return
	}
	defer upstream.Close()
	upgrader := websocket.Upgrader{CheckOrigin: g.validWebSocketOrigin}
	client, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer client.Close()
	client.SetReadLimit(g.config.MaxRequestBytes)
	upstream.SetReadLimit(g.config.MaxRequestBytes)
	proxyRealtimeWebSocket(client, upstream, g, key.ID, routes[0].Binding.ModelID)
}

func (g *Gateway) serveResponsesWebSocket(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("api_key") != "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "api_key_query_forbidden", "API keys are not accepted in query parameters.", "")
		return
	}
	key, err := g.authenticateRealtime(r)
	if err != nil {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", "invalid_api_key", "Invalid API key.", "")
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: g.validWebSocketOrigin}
	client, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer client.Close()
	client.SetReadLimit(g.config.MaxRequestBytes)
	messageType, firstMessage, err := client.ReadMessage()
	if err != nil {
		return
	}
	modelID, err := responsesWebSocketModel(firstMessage)
	if err != nil {
		_ = client.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"type": "invalid_request_error", "code": "invalid_model", "message": err.Error()}})
		return
	}
	routes, err := g.routesForModel(r.Context(), key.ID, modelID, modelbinding.AdapterOpenaiResponses)
	if err != nil || len(routes) == 0 {
		_ = client.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"type": "invalid_request_error", "code": "model_not_allowed", "message": "The requested model is not allowed."}})
		return
	}
	var upstream *websocket.Conn
	var rewritten []byte
	upstreamModel := ""
	for _, candidate := range routes {
		rewritten, err = rewriteResponsesWebSocketModel(firstMessage, candidate.Binding.UpstreamModelName)
		if err != nil {
			return
		}
		target, joinErr := joinUpstreamURL(candidate.Provider.BaseURL, "/v1/responses", r.URL.RawQuery)
		if joinErr != nil {
			continue
		}
		target = strings.Replace(target, "https://", "wss://", 1)
		target = strings.Replace(target, "http://", "ws://", 1)
		headers := upstreamWebSocketHeaders(r.Header, candidate.Credential)
		upstream, _, err = g.dialer.DialContext(r.Context(), target, headers)
		if err == nil {
			upstreamModel = candidate.Binding.UpstreamModelName
			break
		}
	}
	if upstream == nil {
		_ = client.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"type": "server_error", "code": "upstream_unavailable", "message": "No Responses WebSocket provider is available."}})
		return
	}
	defer upstream.Close()
	upstream.SetReadLimit(g.config.MaxRequestBytes)
	reservation, reserveErr := g.usage.Reserve(r.Context(), key.ID, routes[0].Binding.ModelID, requestIDFromContext(r), string(modelbinding.AdapterOpenaiResponses), "/v1/responses", "", "")
	if reserveErr != nil {
		_ = client.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"type": "rate_limit_error", "code": "quota_exceeded", "message": "The API key or project credit quota is exhausted."}})
		return
	}
	if err := upstream.WriteMessage(messageType, rewritten); err != nil {
		_ = g.usage.Finalize(r.Context(), reservation, false, "outcome_unknown", "connection", err.Error(), nil, "")
		return
	}
	_ = g.usage.MarkSent(r.Context(), reservation.CallID)
	_ = g.usage.Finalize(r.Context(), reservation, true, "succeeded", "", "", &routes[0].Provider.ID, routes[0].Provider.Name)
	proxyResponsesWebSockets(client, upstream, upstreamModel, g, key.ID, routes[0].Binding.ModelID)
}

func (g *Gateway) validWebSocketOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" || origin == g.config.AllowedOrigin {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && strings.EqualFold(parsed.Host, request.Host)
}

func upstreamWebSocketHeaders(source http.Header, credential string) http.Header {
	headers := source.Clone()
	removeHopByHop(headers)
	for _, name := range []string{
		"Authorization", "Sec-WebSocket-Key", "Sec-WebSocket-Version",
		"Sec-WebSocket-Extensions", "Sec-WebSocket-Protocol",
	} {
		headers.Del(name)
	}
	headers.Set("Authorization", "Bearer "+credential)
	return headers
}

func responsesWebSocketModel(message []byte) (string, error) {
	var payload struct {
		Type  string `json:"type"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(message, &payload); err != nil {
		return "", errors.New("invalid response.create message")
	}
	payload.Model = strings.TrimSpace(payload.Model)
	if payload.Type != "response.create" || payload.Model == "" || len(payload.Model) > 256 {
		return "", errors.New("response.create with model is required")
	}
	return payload.Model, nil
}

func rewriteResponsesWebSocketModel(message []byte, upstreamModel string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(message, &payload); err != nil {
		return nil, errors.New("invalid response.create message")
	}
	var messageType string
	if err := json.Unmarshal(payload["type"], &messageType); err != nil || messageType != "response.create" {
		return nil, errors.New("response.create message is required")
	}
	model, err := json.Marshal(upstreamModel)
	if err != nil {
		return nil, err
	}
	payload["model"] = model
	return json.Marshal(payload)
}

func proxyResponsesWebSockets(client, upstream *websocket.Conn, upstreamModel string, gateway *Gateway, keyID, modelID uuid.UUID) {
	errs := make(chan error, 2)
	go func() {
		for {
			messageType, data, err := client.ReadMessage()
			if err != nil {
				errs <- err
				return
			}
			if messageType == websocket.TextMessage {
				var envelope struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &envelope) == nil && envelope.Type == "response.create" {
					reservation, reserveErr := gateway.usage.Reserve(context.Background(), keyID, modelID, uuid.Must(uuid.NewV7()).String(), string(modelbinding.AdapterOpenaiResponses), "/v1/responses", "", "")
					if reserveErr != nil {
						_ = client.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"type": "rate_limit_error", "code": "quota_exceeded", "message": "The API key or project credit quota is exhausted."}})
						continue
					}
					data, err = rewriteResponsesWebSocketModel(data, upstreamModel)
					if err != nil {
						errs <- err
						return
					}
					if err := upstream.WriteMessage(messageType, data); err != nil {
						_ = gateway.usage.Finalize(context.Background(), reservation, false, "outcome_unknown", "connection", err.Error(), nil, "")
						errs <- err
						return
					}
					_ = gateway.usage.MarkSent(context.Background(), reservation.CallID)
					_ = gateway.usage.Finalize(context.Background(), reservation, true, "succeeded", "", "", nil, "")
					continue
				}
			}
			if err := upstream.WriteMessage(messageType, data); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		for {
			messageType, data, err := upstream.ReadMessage()
			if err != nil {
				errs <- err
				return
			}
			if err := client.WriteMessage(messageType, data); err != nil {
				errs <- err
				return
			}
		}
	}()
	<-errs
}

func proxyRealtimeWebSocket(client, upstream *websocket.Conn, gateway *Gateway, keyID, modelID uuid.UUID) {
	errs := make(chan error, 2)
	go func() {
		for {
			messageType, data, err := client.ReadMessage()
			if err != nil {
				errs <- err
				return
			}
			if messageType == websocket.TextMessage {
				var event struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &event) == nil && event.Type == "response.create" {
					reservation, reserveErr := gateway.usage.Reserve(context.Background(), keyID, modelID, uuid.Must(uuid.NewV7()).String(), string(modelbinding.AdapterOpenaiRealtime), "/v1/realtime", "", "")
					if reserveErr != nil {
						_ = client.WriteJSON(map[string]any{"type": "error", "error": map[string]any{"type": "rate_limit_error", "code": "quota_exceeded", "message": "The API key or project credit quota is exhausted."}})
						continue
					}
					if err := upstream.WriteMessage(messageType, data); err != nil {
						_ = gateway.usage.Finalize(context.Background(), reservation, false, "outcome_unknown", "connection", err.Error(), nil, "")
						errs <- err
						return
					}
					_ = gateway.usage.MarkSent(context.Background(), reservation.CallID)
					_ = gateway.usage.Finalize(context.Background(), reservation, true, "succeeded", "", "", nil, "")
					continue
				}
			}
			if err := upstream.WriteMessage(messageType, data); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		for {
			messageType, data, err := upstream.ReadMessage()
			if err != nil {
				errs <- err
				return
			}
			if err := client.WriteMessage(messageType, data); err != nil {
				errs <- err
				return
			}
		}
	}()
	<-errs
}

func (g *Gateway) authenticateRealtime(r *http.Request) (*ent.APIKey, error) {
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	raw := ""
	if len(authorization) > 7 && strings.EqualFold(authorization[:7], "Bearer ") {
		raw = strings.TrimSpace(authorization[7:])
	}
	if raw == "" {
		for _, protocol := range websocket.Subprotocols(r) {
			if strings.HasPrefix(protocol, "nebula-api-key.") {
				raw = strings.TrimPrefix(protocol, "nebula-api-key.")
				break
			}
		}
	}
	if raw == "" {
		return nil, errors.New("missing API key")
	}
	hash := g.security.HashAPIKey(raw)
	return g.db.APIKey.Query().Where(
		apikey.KeyHashEQ(hash),
		apikey.StatusEQ(apikey.StatusActive),
	).Only(r.Context())
}

func proxyWebSockets(left, right *websocket.Conn) {
	errs := make(chan error, 2)
	var once sync.Once
	copyFrames := func(dst, src *websocket.Conn) {
		for {
			messageType, data, err := src.ReadMessage()
			if err != nil {
				errs <- err
				return
			}
			if err := dst.WriteMessage(messageType, data); err != nil {
				errs <- err
				return
			}
		}
	}
	go copyFrames(right, left)
	go copyFrames(left, right)
	<-errs
	once.Do(func() {
		_ = left.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
		_ = right.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	})
}

func supportedRoute(method, requestPath string) bool {
	switch {
	case method == http.MethodGet && requestPath == "/v1/realtime":
		return true
	case method == http.MethodGet && requestPath == "/v1/models":
		return true
	case method == http.MethodPost && requestPath == "/v1/chat/completions":
		return true
	case method == http.MethodPost && requestPath == "/v1/completions":
		return true
	case method == http.MethodPost && requestPath == "/v1/responses":
		return true
	case method == http.MethodPost && requestPath == "/v1/responses/compact":
		return true
	case method == http.MethodPost && requestPath == "/v1/embeddings":
		return true
	case method == http.MethodPost && requestPath == "/v2/rerank":
		return true
	case method == http.MethodPost && requestPath == "/v1/images/generations":
		return true
	case method == http.MethodPost && requestPath == "/v1/images/edits":
		return true
	case method == http.MethodPost && requestPath == "/v1/images/variations":
		return true
	case method == http.MethodPost && requestPath == "/v1/audio/transcriptions":
		return true
	case method == http.MethodPost && requestPath == "/v1/audio/translations":
		return true
	case method == http.MethodPost && requestPath == "/v1/audio/speech":
		return true
	case method == http.MethodPost && requestPath == "/v1/videos":
		return true
	case method == http.MethodGet && isVideoResourcePath(requestPath):
		return true
	case method == http.MethodGet && isVideoContentPath(requestPath):
		return true
	case method == http.MethodPost && isVideoRemixPath(requestPath):
		return true
	case method == http.MethodPost && requestPath == "/v1/moderations":
		return true
	case method == http.MethodPost && requestPath == "/v1/messages":
		return true
	case method == http.MethodPost && isGeminiPath(requestPath):
		return true
	default:
		return false
	}
}

func adapterForPath(requestPath string) modelbinding.Adapter {
	switch {
	case requestPath == "/v1/responses" || requestPath == "/v1/responses/compact":
		return modelbinding.AdapterOpenaiResponses
	case requestPath == "/v1/embeddings":
		return modelbinding.AdapterOpenaiEmbeddings
	case strings.HasPrefix(requestPath, "/v1/images/"):
		return modelbinding.AdapterOpenaiImages
	case strings.HasPrefix(requestPath, "/v1/audio/"):
		return modelbinding.AdapterOpenaiAudio
	case requestPath == "/v1/videos" || strings.HasPrefix(requestPath, "/v1/videos/"):
		return modelbinding.AdapterOpenaiVideo
	case requestPath == "/v1/realtime" || strings.HasPrefix(requestPath, "/v1/realtime/"):
		return modelbinding.AdapterOpenaiRealtime
	case requestPath == "/v1/moderations":
		return modelbinding.AdapterOpenaiModerations
	case requestPath == "/v1/messages":
		return modelbinding.AdapterAnthropic
	case requestPath == "/v2/rerank":
		return modelbinding.AdapterCohereRerankV2
	case isGeminiPath(requestPath):
		return modelbinding.AdapterGoogleGeminiV1beta
	default:
		return modelbinding.AdapterOpenaiCompatible
	}
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func isVideoResourcePath(requestPath string) bool {
	if !strings.HasPrefix(requestPath, "/v1/videos/") || isVideoContentPath(requestPath) || isVideoRemixPath(requestPath) {
		return false
	}
	return validResourceID(strings.TrimPrefix(requestPath, "/v1/videos/"))
}

func isVideoContentPath(requestPath string) bool {
	if !strings.HasSuffix(requestPath, "/content") {
		return false
	}
	return validResourceID(strings.TrimSuffix(strings.TrimPrefix(requestPath, "/v1/videos/"), "/content"))
}

func isVideoRemixPath(requestPath string) bool {
	if !strings.HasSuffix(requestPath, "/remix") {
		return false
	}
	return validResourceID(strings.TrimSuffix(strings.TrimPrefix(requestPath, "/v1/videos/"), "/remix"))
}

func validResourceID(value string) bool {
	return value != "" && len(value) <= 256 && !strings.Contains(value, "/")
}

func createdResourceType(method, requestPath string) (string, bool) {
	if method == http.MethodPost && requestPath == "/v1/videos" {
		return "video", true
	}
	return "", false
}

func routedResource(method, requestPath string) (string, string, bool) {
	if adapterForPath(requestPath) != modelbinding.AdapterOpenaiVideo {
		return "", "", false
	}
	if method == http.MethodGet && isVideoResourcePath(requestPath) {
		return "video", strings.TrimPrefix(requestPath, "/v1/videos/"), true
	}
	if method == http.MethodGet && isVideoContentPath(requestPath) {
		return "video", strings.TrimSuffix(strings.TrimPrefix(requestPath, "/v1/videos/"), "/content"), true
	}
	if method == http.MethodPost && isVideoRemixPath(requestPath) {
		return "video", strings.TrimSuffix(strings.TrimPrefix(requestPath, "/v1/videos/"), "/remix"), true
	}
	return "", "", false
}

func isGeminiPath(requestPath string) bool {
	if !strings.HasPrefix(requestPath, "/v1beta/models/") {
		return false
	}
	for _, operation := range []string{":generateContent", ":streamGenerateContent", ":embedContent", ":batchEmbedContents"} {
		if strings.HasSuffix(requestPath, operation) && len(strings.TrimSuffix(strings.TrimPrefix(requestPath, "/v1beta/models/"), operation)) > 0 {
			return true
		}
	}
	return false
}

func extractRequestedModel(requestPath, contentType string, replay *replayBody) (string, error) {
	if isGeminiPath(requestPath) {
		value := strings.TrimPrefix(requestPath, "/v1beta/models/")
		if index := strings.LastIndex(value, ":"); index >= 0 {
			value = value[:index]
		}
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 256 || strings.Contains(value, "/") {
			return "", errors.New("model is required")
		}
		return value, nil
	}
	return extractModel(contentType, replay)
}

func extractNestedJSONModel(contentType string, replay *replayBody, objectField string) (string, error) {
	if replay == nil {
		return "", errors.New("request body is required")
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return "", errors.New("Content-Type must be application/json")
	}
	reader, err := replay.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return "", errors.New("invalid JSON body")
	}
	var nested struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(payload[objectField], &nested); err != nil {
		return "", fmt.Errorf("%s.model is required", objectField)
	}
	nested.Model = strings.TrimSpace(nested.Model)
	if nested.Model == "" || len(nested.Model) > 256 {
		return "", fmt.Errorf("%s.model is required", objectField)
	}
	return nested.Model, nil
}

func extractRealtimeCallModel(contentType string, replay *replayBody) (string, error) {
	if replay == nil {
		return "", errors.New("request body is required")
	}
	reader, err := replay.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return "", errors.New("Content-Type must be multipart/form-data")
	}
	form := multipart.NewReader(reader, params["boundary"])
	for {
		part, err := form.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", errors.New("invalid multipart body")
		}
		if part.FormName() != "session" {
			_ = part.Close()
			continue
		}
		var session struct {
			Model string `json:"model"`
		}
		err = json.NewDecoder(io.LimitReader(part, 1<<20)).Decode(&session)
		_ = part.Close()
		session.Model = strings.TrimSpace(session.Model)
		if err != nil || session.Model == "" || len(session.Model) > 256 {
			return "", errors.New("model is required in session")
		}
		return session.Model, nil
	}
	return "", errors.New("session is required")
}

func upstreamPath(requestPath, upstreamModel string, adapter modelbinding.Adapter) (string, error) {
	switch adapter {
	case modelbinding.AdapterCohereRerankV2:
		return "/v2/rerank", nil
	case modelbinding.AdapterGoogleGeminiV1beta:
		if !isGeminiPath(requestPath) {
			return "", errors.New("invalid Gemini route")
		}
		operation := requestPath[strings.LastIndex(requestPath, ":"):]
		return "/v1beta/models/" + url.PathEscape(strings.TrimPrefix(upstreamModel, "models/")) + operation, nil
	default:
		return requestPath, nil
	}
}

func rewriteUpstreamBody(contentType string, replay *replayBody, upstreamModel string, adapter modelbinding.Adapter) (
	io.ReadCloser,
	string,
	int64,
	func(),
	error,
) {
	if adapter != modelbinding.AdapterGoogleGeminiV1beta {
		if adapter == modelbinding.AdapterOpenaiRealtime && replay != nil {
			mediaType, _, _ := mime.ParseMediaType(contentType)
			if mediaType == "application/json" || mediaType == "multipart/form-data" {
				return rewriteRealtimeBody(contentType, replay, upstreamModel)
			}
		}
		return rewriteRequestBody(contentType, replay, upstreamModel)
	}
	if replay == nil {
		return nil, contentType, 0, func() {}, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return nil, "", 0, func() {}, errors.New("Gemini requests require application/json")
	}
	source, err := replay.Open()
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	defer source.Close()
	var payload map[string]any
	if err := json.NewDecoder(source).Decode(&payload); err != nil {
		return nil, "", 0, func() {}, errors.New("invalid JSON body")
	}
	qualifiedModel := "models/" + strings.TrimPrefix(upstreamModel, "models/")
	if _, exists := payload["model"]; exists {
		payload["model"] = qualifiedModel
	}
	if requests, ok := payload["requests"].([]any); ok {
		for _, raw := range requests {
			if request, ok := raw.(map[string]any); ok {
				request["model"] = qualifiedModel
			}
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	return io.NopCloser(bytes.NewReader(body)), contentType, int64(len(body)), func() {}, nil
}

func rewriteRealtimeBody(contentType string, replay *replayBody, upstreamModel string) (io.ReadCloser, string, int64, func(), error) {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	if mediaType == "application/json" {
		return rewriteNestedJSONModel(contentType, replay, "session", upstreamModel)
	}
	if mediaType != "multipart/form-data" {
		return nil, "", 0, func() {}, errors.New("Realtime requests require application/json or multipart/form-data")
	}
	return rewriteMultipartJSONField(contentType, replay, "session", "model", upstreamModel)
}

func rewriteNestedJSONModel(contentType string, replay *replayBody, objectField, upstreamModel string) (io.ReadCloser, string, int64, func(), error) {
	source, err := replay.Open()
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	defer source.Close()
	var payload map[string]json.RawMessage
	if err := json.NewDecoder(source).Decode(&payload); err != nil {
		return nil, "", 0, func() {}, errors.New("invalid JSON body")
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(payload[objectField], &nested); err != nil {
		return nil, "", 0, func() {}, fmt.Errorf("%s is required", objectField)
	}
	nested["model"], err = json.Marshal(upstreamModel)
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	payload[objectField], err = json.Marshal(nested)
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	return io.NopCloser(bytes.NewReader(body)), contentType, int64(len(body)), func() {}, nil
}

func rewriteMultipartJSONField(contentType string, replay *replayBody, fieldName, jsonField, replacement string) (io.ReadCloser, string, int64, func(), error) {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		return nil, "", 0, func() {}, errors.New("invalid multipart Content-Type")
	}
	source, err := replay.Open()
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	file, err := os.CreateTemp("", "nebula-upstream-*")
	if err != nil {
		_ = source.Close()
		return nil, "", 0, func() {}, err
	}
	filePath := file.Name()
	cleanupOnError := func() {
		_ = source.Close()
		_ = file.Close()
		_ = os.Remove(filePath)
	}
	writer := multipart.NewWriter(file)
	reader := multipart.NewReader(source, params["boundary"])
	found := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			cleanupOnError()
			return nil, "", 0, func() {}, err
		}
		target, err := writer.CreatePart(cloneMIMEHeader(part.Header))
		if err != nil {
			_ = part.Close()
			cleanupOnError()
			return nil, "", 0, func() {}, err
		}
		if part.FormName() == fieldName {
			var payload map[string]json.RawMessage
			err = json.NewDecoder(io.LimitReader(part, 1<<20)).Decode(&payload)
			if err == nil {
				payload[jsonField], err = json.Marshal(replacement)
			}
			if err == nil {
				var encoded []byte
				encoded, err = json.Marshal(payload)
				if err == nil {
					_, err = target.Write(encoded)
				}
			}
			found = true
		} else {
			_, err = io.Copy(target, part)
		}
		_ = part.Close()
		if err != nil {
			cleanupOnError()
			return nil, "", 0, func() {}, err
		}
	}
	_ = source.Close()
	if !found {
		cleanupOnError()
		return nil, "", 0, func() {}, fmt.Errorf("%s is required", fieldName)
	}
	if err := writer.Close(); err != nil {
		cleanupOnError()
		return nil, "", 0, func() {}, err
	}
	if err := file.Close(); err != nil {
		cleanupOnError()
		return nil, "", 0, func() {}, err
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		_ = os.Remove(filePath)
		return nil, "", 0, func() {}, err
	}
	body, err := os.Open(filePath)
	if err != nil {
		_ = os.Remove(filePath)
		return nil, "", 0, func() {}, err
	}
	cleanup := func() {
		_ = body.Close()
		_ = os.Remove(filePath)
	}
	return body, writer.FormDataContentType(), stat.Size(), cleanup, nil
}

func extractModel(contentType string, replay *replayBody) (string, error) {
	if replay == nil {
		return "", errors.New("request body is required")
	}
	reader, err := replay.Open()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", errors.New("invalid Content-Type")
	}
	if mediaType == "multipart/form-data" {
		boundary := params["boundary"]
		if boundary == "" {
			return "", errors.New("multipart boundary is required")
		}
		form := multipart.NewReader(reader, boundary)
		for {
			part, err := form.NextPart()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return "", errors.New("invalid multipart body")
			}
			if part.FormName() == "model" {
				value, err := io.ReadAll(io.LimitReader(part, 257))
				_ = part.Close()
				if err != nil || len(value) > 256 {
					return "", errors.New("invalid model")
				}
				modelID := strings.TrimSpace(string(value))
				if modelID == "" {
					return "", errors.New("model is required")
				}
				return modelID, nil
			}
			_ = part.Close()
		}
		return "", errors.New("model is required")
	}
	if mediaType != "application/json" {
		return "", errors.New("Content-Type must be application/json or multipart/form-data")
	}
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return "", errors.New("invalid JSON body")
	}
	payload.Model = strings.TrimSpace(payload.Model)
	if payload.Model == "" || len(payload.Model) > 256 {
		return "", errors.New("model is required")
	}
	return payload.Model, nil
}

func extractInputSnapshot(requestPath, contentType string, replay *replayBody) (string, string) {
	if replay == nil {
		return "", ""
	}
	reader, err := replay.Open()
	if err != nil {
		return "", ""
	}
	defer reader.Close()
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", ""
	}
	if mediaType == "multipart/form-data" {
		form := multipart.NewReader(reader, params["boundary"])
		for {
			part, e := form.NextPart()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				return "", ""
			}
			name := part.FormName()
			if name == "prompt" || name == "input" {
				value, _ := io.ReadAll(io.LimitReader(part, 1<<20))
				_ = part.Close()
				return strings.TrimSpace(string(value)), name
			}
			_ = part.Close()
		}
		return "", ""
	}
	if mediaType != "application/json" {
		return "", ""
	}
	var payload map[string]any
	if json.NewDecoder(reader).Decode(&payload) != nil {
		return "", ""
	}
	if value, ok := payload["prompt"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), "prompt"
	}
	if value, ok := payload["input"].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), "input"
	}
	if messages, ok := payload["messages"].([]any); ok {
		for i := len(messages) - 1; i >= 0; i-- {
			if m, ok := messages[i].(map[string]any); ok && strings.EqualFold(fmt.Sprint(m["role"]), "user") {
				if text := textFromJSONValue(m["content"]); text != "" {
					return text, "user_message"
				}
			}
		}
	}
	if input, ok := payload["input"].([]any); ok {
		for i := len(input) - 1; i >= 0; i-- {
			if text := textFromJSONValue(input[i]); text != "" {
				return text, "input_text"
			}
		}
	}
	return "", ""
}

func textFromJSONValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case []any:
		for i := len(v) - 1; i >= 0; i-- {
			if text := textFromJSONValue(v[i]); text != "" {
				return text
			}
		}
	case map[string]any:
		for _, key := range []string{"text", "input_text", "content"} {
			if text := textFromJSONValue(v[key]); text != "" {
				return text
			}
		}
	}
	return ""
}

func rewriteRequestBody(contentType string, replay *replayBody, upstreamModel string) (
	io.ReadCloser,
	string,
	int64,
	func(),
	error,
) {
	if replay == nil {
		return nil, contentType, 0, func() {}, nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	source, err := replay.Open()
	if err != nil {
		return nil, "", 0, func() {}, err
	}
	if mediaType == "application/json" {
		defer source.Close()
		var payload map[string]json.RawMessage
		if err := json.NewDecoder(source).Decode(&payload); err != nil {
			return nil, "", 0, func() {}, err
		}
		modelValue, err := json.Marshal(upstreamModel)
		if err != nil {
			return nil, "", 0, func() {}, err
		}
		payload["model"] = modelValue
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, "", 0, func() {}, err
		}
		return io.NopCloser(bytes.NewReader(body)), contentType, int64(len(body)), func() {}, nil
	}
	if mediaType != "multipart/form-data" {
		_ = source.Close()
		return nil, "", 0, func() {}, errors.New("unsupported request body type")
	}
	boundary := params["boundary"]
	if boundary == "" {
		_ = source.Close()
		return nil, "", 0, func() {}, errors.New("multipart boundary is required")
	}
	file, err := os.CreateTemp("", "nebula-upstream-*")
	if err != nil {
		_ = source.Close()
		return nil, "", 0, func() {}, err
	}
	filePath := file.Name()
	cleanupOnError := func() {
		_ = source.Close()
		_ = file.Close()
		_ = os.Remove(filePath)
	}
	writer := multipart.NewWriter(file)
	reader := multipart.NewReader(source, boundary)
	modelWritten := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			cleanupOnError()
			return nil, "", 0, func() {}, err
		}
		header := cloneMIMEHeader(part.Header)
		target, err := writer.CreatePart(header)
		if err != nil {
			_ = part.Close()
			cleanupOnError()
			return nil, "", 0, func() {}, err
		}
		if part.FormName() == "model" {
			_, err = io.WriteString(target, upstreamModel)
			modelWritten = true
		} else {
			_, err = io.Copy(target, part)
		}
		_ = part.Close()
		if err != nil {
			cleanupOnError()
			return nil, "", 0, func() {}, err
		}
	}
	_ = source.Close()
	if !modelWritten {
		cleanupOnError()
		return nil, "", 0, func() {}, errors.New("model is required")
	}
	if err := writer.Close(); err != nil {
		cleanupOnError()
		return nil, "", 0, func() {}, err
	}
	if err := file.Close(); err != nil {
		cleanupOnError()
		return nil, "", 0, func() {}, err
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		_ = os.Remove(filePath)
		return nil, "", 0, func() {}, err
	}
	body, err := os.Open(filePath)
	if err != nil {
		_ = os.Remove(filePath)
		return nil, "", 0, func() {}, err
	}
	cleanup := func() {
		_ = body.Close()
		_ = os.Remove(filePath)
	}
	return body, writer.FormDataContentType(), stat.Size(), cleanup, nil
}

func cloneMIMEHeader(source textproto.MIMEHeader) textproto.MIMEHeader {
	result := make(textproto.MIMEHeader, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}

func joinUpstreamURL(baseURL, requestPath, rawQuery string) (string, error) {
	base, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || base.Host == "" {
		return "", errors.New("invalid provider URL")
	}
	suffix := requestPath
	for _, version := range []string{"/v1beta", "/v2", "/v1"} {
		if strings.HasSuffix(base.Path, version) && strings.HasPrefix(suffix, version+"/") {
			suffix = strings.TrimPrefix(suffix, version)
			break
		}
	}
	base.Path = strings.TrimRight(base.Path, "/") + suffix
	base.RawQuery = rawQuery
	return base.String(), nil
}

func copyUpstreamResponse(w http.ResponseWriter, response *http.Response) {
	defer response.Body.Close()
	copyHeaders(w.Header(), response.Header)
	removeHopByHop(w.Header())
	w.WriteHeader(response.StatusCode)
	buffer := make([]byte, 32<<10)
	flusher, canFlush := w.(http.Flusher)
	for {
		n, err := response.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func copyHeaders(destination, source http.Header) {
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func removeHopByHop(headers http.Header) {
	for _, header := range []string{
		"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
	} {
		headers.Del(header)
	}
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func writeProtocolError(w http.ResponseWriter, requestPath string, status int, errorType, code, message, parameter string) {
	if requestPath == "/v1/messages" {
		writeAnthropicError(w, status, errorType, message)
		return
	}
	if isGeminiPath(requestPath) {
		writeGeminiError(w, status, geminiStatus(status), message)
		return
	}
	if requestPath == "/v2/rerank" {
		writeJSON(w, status, map[string]any{"message": message})
		return
	}
	writeOpenAIError(w, status, errorType, code, message, parameter)
}

func writeGeminiError(w http.ResponseWriter, status int, errorStatus, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"code": status, "message": message, "status": errorStatus,
	}})
}

func geminiStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "INVALID_ARGUMENT"
	case http.StatusUnauthorized:
		return "UNAUTHENTICATED"
	case http.StatusForbidden:
		return "PERMISSION_DENIED"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RESOURCE_EXHAUSTED"
	case http.StatusServiceUnavailable:
		return "UNAVAILABLE"
	default:
		return "INTERNAL"
	}
}

func writeOpenAIError(w http.ResponseWriter, status int, errorType, code, message, parameter string) {
	var parameterValue any
	if parameter != "" {
		parameterValue = parameter
	}
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"message": message, "type": errorType, "param": parameterValue, "code": code,
	}})
}

func writeAnthropicError(w http.ResponseWriter, status int, errorType, message string) {
	writeJSON(w, status, map[string]any{
		"type": "error", "error": map[string]any{"type": errorType, "message": message},
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type replayBody struct {
	memory []byte
	path   string
}

func captureBody(source io.ReadCloser, maxBytes int64) (*replayBody, error) {
	defer source.Close()
	var memory bytes.Buffer
	limited := io.LimitReader(source, maxBytes+1)
	if _, err := io.CopyN(&memory, limited, replayMemoryThreshold+1); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if int64(memory.Len()) > maxBytes {
		return nil, errors.New("request body is too large")
	}
	if memory.Len() <= replayMemoryThreshold {
		remaining, err := io.ReadAll(limited)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		if int64(memory.Len()+len(remaining)) > maxBytes {
			return nil, errors.New("request body is too large")
		}
		memory.Write(remaining)
		return &replayBody{memory: memory.Bytes()}, nil
	}
	file, err := os.CreateTemp("", "nebula-gateway-*")
	if err != nil {
		return nil, fmt.Errorf("create request spool: %w", err)
	}
	cleanup := func() {
		_ = file.Close()
		_ = os.Remove(file.Name())
	}
	if _, err := file.Write(memory.Bytes()); err != nil {
		cleanup()
		return nil, fmt.Errorf("write request spool: %w", err)
	}
	written, err := io.Copy(file, limited)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("write request spool: %w", err)
	}
	if int64(memory.Len())+written > maxBytes {
		cleanup()
		return nil, errors.New("request body is too large")
	}
	if err := file.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("close request spool: %w", err)
	}
	return &replayBody{path: file.Name()}, nil
}

func (r *replayBody) Open() (io.ReadCloser, error) {
	if r.path == "" {
		return io.NopCloser(bytes.NewReader(r.memory)), nil
	}
	return os.Open(r.path)
}

func (r *replayBody) Close() error {
	if r.path == "" {
		return nil
	}
	return os.Remove(r.path)
}
