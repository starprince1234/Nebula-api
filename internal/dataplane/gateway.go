package dataplane

import (
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
	"path"
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
)

const replayMemoryThreshold = 1 << 20

type Config struct {
	ConnectTimeout        time.Duration
	ResponseHeaderTimeout time.Duration
	MaxRequestBytes       int64
	VideoTaskRouteTTL     time.Duration
}

type Gateway struct {
	db       *ent.Client
	cache    *cache.Store
	security *security.Manager
	client   *http.Client
	dialer   *websocket.Dialer
	config   Config
}

type route struct {
	Binding    *ent.ModelBinding
	Provider   *ent.Provider
	Credential string
}

func NewGateway(db *ent.Client, cacheStore *cache.Store, securityManager *security.Manager, cfg Config) *Gateway {
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
		db: db, cache: cacheStore, security: securityManager,
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
	if r.URL.Path == "/v1/realtime" {
		g.serveRealtime(w, r)
		return
	}
	if !supportedRoute(r.Method, r.URL.Path) {
		writeOpenAIError(w, http.StatusNotFound, "invalid_request_error", "route_not_found", "Route not found.", "")
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

	adapter := modelbinding.AdapterOpenaiCompatible
	if r.URL.Path == "/v1/messages" {
		adapter = modelbinding.AdapterAnthropic
		if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
			writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "anthropic-version header is required")
			return
		}
	}

	var routes []route
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/video/generations/") {
		taskID := path.Base(r.URL.Path)
		routes, err = g.videoTaskRoutes(r.Context(), key.ID, taskID)
	} else {
		modelID, extractErr := extractModel(r.Header.Get("Content-Type"), replay)
		if extractErr != nil {
			writeProtocolError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "invalid_model", extractErr.Error(), "model")
			return
		}
		routes, err = g.routesForModel(r.Context(), key.ID, modelID, adapter)
	}
	if err != nil || len(routes) == 0 {
		writeProtocolError(w, r.URL.Path, http.StatusBadRequest, "invalid_request_error", "model_not_allowed", "The requested model is not allowed or is unavailable for this API key.", "model")
		return
	}
	g.proxyHTTP(w, r, replay, routes)
}

func (g *Gateway) authenticate(r *http.Request) (*ent.APIKey, error) {
	raw := ""
	if r.URL.Path == "/v1/messages" {
		raw = strings.TrimSpace(r.Header.Get("x-api-key"))
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

func (g *Gateway) videoTaskRoutes(ctx context.Context, keyID uuid.UUID, taskID string) ([]route, error) {
	bindingIDRaw, err := g.cache.GetVideoRoute(ctx, taskID)
	if err != nil {
		return nil, err
	}
	bindingID, err := uuid.Parse(bindingIDRaw)
	if err != nil {
		return nil, err
	}
	binding, err := g.db.ModelBinding.Query().Where(
		modelbinding.IDEQ(bindingID),
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

func (g *Gateway) proxyHTTP(w http.ResponseWriter, original *http.Request, replay *replayBody, routes []route) {
	var lastResponse *http.Response
	for index, candidate := range routes {
		request, cleanup, err := g.upstreamRequest(original, replay, candidate)
		if err != nil {
			continue
		}
		response, err := g.client.Do(request)
		cleanup()
		if err != nil {
			continue
		}
		lastResponse = response
		canRetry := response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		if canRetry && index < len(routes)-1 {
			_, _ = io.CopyN(io.Discard, response.Body, 64<<10)
			_ = response.Body.Close()
			continue
		}
		if original.Method == http.MethodPost && original.URL.Path == "/v1/video/generations" && response.StatusCode >= 200 && response.StatusCode < 300 {
			g.writeVideoGenerationResponse(w, response, candidate.Binding.ID)
			return
		}
		copyUpstreamResponse(w, response)
		return
	}
	if lastResponse != nil {
		copyUpstreamResponse(w, lastResponse)
		return
	}
	writeProtocolError(w, original.URL.Path, http.StatusBadGateway, "server_error", "upstream_unavailable", "No upstream provider is available.", "")
}

func (g *Gateway) upstreamRequest(original *http.Request, replay *replayBody, candidate route) (*http.Request, func(), error) {
	target, err := joinUpstreamURL(candidate.Provider.BaseURL, original.URL.Path, original.URL.RawQuery)
	if err != nil {
		return nil, func() {}, err
	}
	body, contentType, contentLength, cleanup, err := rewriteRequestBody(
		original.Header.Get("Content-Type"),
		replay,
		candidate.Binding.UpstreamModelName,
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
	if candidate.Binding.Adapter == modelbinding.AdapterAnthropic {
		request.Header.Set("x-api-key", candidate.Credential)
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

func (g *Gateway) writeVideoGenerationResponse(w http.ResponseWriter, response *http.Response, bindingID uuid.UUID) {
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "server_error", "invalid_upstream_response", "Invalid video provider response.", "")
		return
	}
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		taskID := firstString(payload, "id", "task_id")
		if taskID != "" {
			_ = g.cache.SetVideoRoute(context.Background(), taskID, bindingID.String(), g.config.VideoTaskRouteTTL)
		}
	}
	copyHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(body)
}

func (g *Gateway) serveRealtime(w http.ResponseWriter, r *http.Request) {
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
	routes, err := g.routesForModel(r.Context(), key.ID, modelID, modelbinding.AdapterOpenaiCompatible)
	if err != nil || len(routes) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "model_not_allowed", "The requested model is not allowed.", "model")
		return
	}
	var upstream *websocket.Conn
	for _, candidate := range routes {
		query := r.URL.Query()
		query.Set("model", candidate.Binding.UpstreamModelName)
		target, err := joinUpstreamURL(candidate.Provider.BaseURL, "/v1/realtime", query.Encode())
		if err != nil {
			continue
		}
		target = strings.Replace(target, "https://", "wss://", 1)
		target = strings.Replace(target, "http://", "ws://", 1)
		headers := http.Header{"Authorization": []string{"Bearer " + candidate.Credential}}
		upstream, _, err = g.dialer.DialContext(r.Context(), target, headers)
		if err == nil {
			break
		}
	}
	if upstream == nil {
		writeOpenAIError(w, http.StatusBadGateway, "server_error", "upstream_unavailable", "No realtime provider is available.", "")
		return
	}
	defer upstream.Close()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(request *http.Request) bool {
			origin := request.Header.Get("Origin")
			if origin == "" {
				return true
			}
			parsed, err := url.Parse(origin)
			return err == nil && strings.EqualFold(parsed.Host, request.Host)
		},
	}
	client, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer client.Close()
	client.SetReadLimit(g.config.MaxRequestBytes)
	upstream.SetReadLimit(g.config.MaxRequestBytes)
	proxyWebSockets(client, upstream)
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
	case method == http.MethodPost && requestPath == "/v1/images/generations":
		return true
	case method == http.MethodPost && requestPath == "/v1/images/edits":
		return true
	case method == http.MethodPost && requestPath == "/v1/audio/transcriptions":
		return true
	case method == http.MethodPost && requestPath == "/v1/audio/translations":
		return true
	case method == http.MethodPost && requestPath == "/v1/audio/speech":
		return true
	case method == http.MethodPost && requestPath == "/v1/video/generations":
		return true
	case method == http.MethodGet && strings.HasPrefix(requestPath, "/v1/video/generations/"):
		return path.Base(requestPath) != "generations"
	case method == http.MethodPost && requestPath == "/v1/messages":
		return true
	default:
		return false
	}
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
	if strings.HasSuffix(base.Path, "/v1") && strings.HasPrefix(suffix, "/v1/") {
		suffix = strings.TrimPrefix(suffix, "/v1")
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
	writeOpenAIError(w, status, errorType, code, message, parameter)
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
