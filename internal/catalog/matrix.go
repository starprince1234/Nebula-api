package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type MatrixEntry struct {
	ID                     string   `json:"id"`
	Object                 string   `json:"object"`
	Created                int64    `json:"created"`
	OwnedBy                string   `json:"owned_by"`
	SupportedEndpointTypes []string `json:"supported_endpoint_types"`
	ModelType              string   `json:"model_type"`
	Description            string   `json:"description"`
	Tags                   string   `json:"tags"`
}

type Item struct {
	ModelID, Description                              string
	OwnedBy, ModelTypes, SupportedEndpointTypes, Tags []string
	RawEntries                                        []MatrixEntry
}

type Suggestion struct {
	Category         string   `json:"category"`
	Capabilities     []string `json:"capabilities"`
	InputModalities  []string `json:"input_modalities"`
	OutputModalities []string `json:"output_modalities"`
	ContextWindow    int      `json:"context_window"`
	MaxInputTokens   int      `json:"max_input_tokens"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

func plainText(value string) string {
	return strings.Join(strings.Fields(html.UnescapeString(htmlTag.ReplaceAllString(value, " "))), " ")
}

func PlainTextDescription(value string) string {
	return plainText(value)
}

func MergeEntries(entries []MatrixEntry) []Item {
	byID := map[string]*Item{}
	for _, entry := range entries {
		entry.ID = strings.TrimSpace(entry.ID)
		if entry.ID == "" {
			continue
		}
		key := strings.ToLower(entry.ID)
		item := byID[key]
		if item == nil {
			item = &Item{ModelID: entry.ID}
			byID[key] = item
		}
		description := plainText(entry.Description)
		if len([]rune(description)) > 4096 {
			description = string([]rune(description)[:4096])
		}
		if len([]rune(description)) > len([]rune(item.Description)) {
			item.Description = description
		}
		item.OwnedBy = add(item.OwnedBy, entry.OwnedBy)
		item.ModelTypes = add(item.ModelTypes, entry.ModelType)
		for _, value := range entry.SupportedEndpointTypes {
			item.SupportedEndpointTypes = add(item.SupportedEndpointTypes, value)
		}
		for _, value := range strings.Split(entry.Tags, ",") {
			item.Tags = add(item.Tags, value)
		}
		item.RawEntries = append(item.RawEntries, entry)
	}
	result := make([]Item, 0, len(byID))
	for _, item := range byID {
		sort.Strings(item.OwnedBy)
		sort.Strings(item.ModelTypes)
		sort.Strings(item.SupportedEndpointTypes)
		sort.Strings(item.Tags)
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool { return strings.ToLower(result[i].ModelID) < strings.ToLower(result[j].ModelID) })
	return result
}

func add(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if strings.EqualFold(current, value) {
			return values
		}
	}
	return append(values, value)
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func hasAny(value string, terms ...string) bool {
	value = strings.ToLower(value)
	for _, term := range terms {
		if strings.Contains(value, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

func joined(values []string) string {
	return strings.ToLower(strings.Join(values, " "))
}

var (
	contextWanPattern      = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*万\s*(?:令牌|代币|token|上下文|字符|词元)`)
	contextMillionPattern  = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*m\b[^。，；]{0,30}(?:token|令牌|代币|上下文|context|窗口)`)
	contextKBeforePattern  = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*k\b[^。，；]{0,40}(?:上下文|context|窗口)`)
	contextKAfterPattern   = regexp.MustCompile(`(?i)(?:上下文|context|窗口|长上下文)[^。，；0-9]{0,30}(\d+(?:\.\d+)?)\s*k\b`)
	outputKBeforePattern   = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*k\b[^。，；]{0,30}(?:最大输出|max[_\s-]?output|输出令牌|输出token|输出长度)`)
	outputKAfterPattern    = regexp.MustCompile(`(?i)(?:最大输出|max[_\s-]?output|输出令牌|输出token|输出长度)[^。，；0-9]{0,20}(\d+(?:\.\d+)?)\s*k\b`)
	reasoningModelPattern = regexp.MustCompile(`(?:^|[-_.])(o[134]|r1)(?:[-_.]|$)`)
)

func scaledMatch(pattern *regexp.Regexp, description string, factor float64) (int, bool) {
	match := pattern.FindStringSubmatch(description)
	if len(match) != 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil || value <= 0 {
		return 0, false
	}
	tokens := int(math.Round(value * factor))
	return tokens, tokens > 0 && tokens <= 1_000_000_000
}

func descriptionCapacities(description string) (contextWindow, maxOutput int) {
	description = plainText(description)
	for _, candidate := range []struct {
		pattern *regexp.Regexp
		factor  float64
	}{{contextWanPattern, 10_000}, {contextMillionPattern, 1_000_000}, {contextKBeforePattern, 1_000}, {contextKAfterPattern, 1_000}} {
		if value, ok := scaledMatch(candidate.pattern, description, candidate.factor); ok {
			contextWindow = value
			break
		}
	}
	for _, pattern := range []*regexp.Regexp{outputKBeforePattern, outputKAfterPattern} {
		if value, ok := scaledMatch(pattern, description, 1_000); ok {
			maxOutput = value
			break
		}
	}
	return contextWindow, maxOutput
}

type capacityRule struct {
	pattern                   *regexp.Regexp
	contextWindow, maxOutput int
}

var capacityRules = []capacityRule{
	{regexp.MustCompile(`^gpt-5(?:\.|-|$)`), 200_000, 16_000},
	{regexp.MustCompile(`^gpt-4o`), 128_000, 16_000},
	{regexp.MustCompile(`^gpt-4(?:-|$)`), 128_000, 4_000},
	{regexp.MustCompile(`^gpt-3\.5`), 16_000, 4_000},
	{regexp.MustCompile(`^o[1-9]`), 200_000, 16_000},
	{regexp.MustCompile(`^claude-(?:sonnet|opus|haiku)-5(?:\b|-)`), 1_000_000, 128_000},
	{regexp.MustCompile(`^claude-sonnet-4-5`), 200_000, 16_000},
	{regexp.MustCompile(`^claude-(?:sonnet|opus)-4`), 200_000, 8_000},
	{regexp.MustCompile(`^claude-haiku`), 200_000, 8_000},
	{regexp.MustCompile(`^claude-fable`), 1_000_000, 128_000},
	{regexp.MustCompile(`^claude-3`), 200_000, 8_000},
	{regexp.MustCompile(`qwen3-(?:0\.6|1\.7|4b)`), 32_000, 4_000},
	{regexp.MustCompile(`qwen3`), 128_000, 8_000},
	{regexp.MustCompile(`qwen2(?:\.5)?|qwen-(?:plus|turbo)|^qwen(?:-|$)`), 128_000, 8_000},
	{regexp.MustCompile(`deepseek-v4`), 1_000_000, 16_000},
	{regexp.MustCompile(`deepseek-v3|deepseek-r1|deepseek`), 128_000, 8_000},
	{regexp.MustCompile(`glm-5`), 1_000_000, 16_000},
	{regexp.MustCompile(`glm-4\.(?:7|6|5)`), 128_000, 16_000},
	{regexp.MustCompile(`glm-4-(?:air|flash)`), 128_000, 4_000},
	{regexp.MustCompile(`glm`), 128_000, 8_000},
	{regexp.MustCompile(`kimi`), 256_000, 16_000},
	{regexp.MustCompile(`minimax`), 1_000_000, 16_000},
	{regexp.MustCompile(`grok-4`), 256_000, 16_000},
	{regexp.MustCompile(`grok`), 128_000, 8_000},
	{regexp.MustCompile(`llama-2`), 40_000, 4_000},
	{regexp.MustCompile(`llama`), 128_000, 8_000},
	{regexp.MustCompile(`gemini-2\.5`), 1_000_000, 64_000},
	{regexp.MustCompile(`gemini-2`), 1_000_000, 32_000},
	{regexp.MustCompile(`gemini-1\.5|gemini`), 1_000_000, 8_000},
	{regexp.MustCompile(`mimo`), 1_000_000, 16_000},
	{regexp.MustCompile(`mistral|phi|^yi|ernie|spark`), 128_000, 8_000},
	{regexp.MustCompile(`rerank`), 32_000, 4_000},
	{regexp.MustCompile(`embed`), 8_192, 4_000},
}

func inferredCapacities(modelID, description string) (contextWindow, maxInput, maxOutput int) {
	contextWindow, maxOutput = descriptionCapacities(description)
	for _, rule := range capacityRules {
		if !rule.pattern.MatchString(strings.ToLower(modelID)) {
			continue
		}
		if contextWindow == 0 {
			contextWindow = rule.contextWindow
		}
		if maxOutput == 0 {
			maxOutput = rule.maxOutput
		}
		break
	}
	if contextWindow == 0 {
		contextWindow = 128_000
	}
	if maxOutput == 0 {
		maxOutput = 4_000
	}
	return contextWindow, contextWindow, maxOutput
}

func SuggestConfiguration(item Item) Suggestion {
	modelID := strings.ToLower(strings.TrimSpace(item.ModelID))
	description := strings.ToLower(plainText(item.Description))
	tags, endpoints, modelTypes := joined(item.Tags), joined(item.SupportedEndpointTypes), joined(item.ModelTypes)
	all := strings.Join([]string{modelID, description, tags, endpoints, modelTypes}, " ")
	s := Suggestion{Category: "text"}

	imageGeneration := hasAny(tags, "绘画") || hasAny(endpoints, "image-generation", "dall", "生图", "图像生成", "绘画")
	videoGeneration := hasAny(endpoints, "视频", "video", "vidu", "kling", "海螺", "wan", "生视频") && !hasAny(tags, "识图")
	if hasAny(all, "rerank", "重排序", "检索") {
		s.Category = "rerank"
	} else if hasAny(all, "embedding", "embed", "嵌入", "文本嵌入") {
		s.Category = "embedding"
	} else if imageGeneration || hasAny(modelTypes, "图像") {
		s.Category = "image"
	} else if hasAny(endpoints, "语音", "音频", "tts", "speech", "音乐", "suno") {
		s.Category = "audio"
	} else if videoGeneration || hasAny(modelTypes, "音视频") {
		s.Category = "video"
	}

	textIO := hasAny(tags, "对话") || hasAny(endpoints, "openai", "anthropic", "gemini")
	imageInput := hasAny(tags, "识图") || hasAny(description, "视觉", "图像", "图片", "vision", "image", "多模态", "vl") || hasAny(endpoints, "图像识别", "omni-image")
	audioInput := hasAny(tags, "音频", "实时语音") || hasAny(description, "语音", "音频", "audio", "speech") || hasAny(endpoints, "语音", "音频", "realtime")
	videoInput := hasAny(tags, "视频") || hasAny(description, "视频", "video", "长视频") || hasAny(endpoints, "omni-video")
	if textIO || (!imageInput && !audioInput && !videoInput) {
		s.InputModalities = add(s.InputModalities, "text")
	}
	if imageInput { s.InputModalities = add(s.InputModalities, "image") }
	if audioInput { s.InputModalities = add(s.InputModalities, "audio") }
	if videoInput { s.InputModalities = add(s.InputModalities, "video") }
	if textIO || (!imageGeneration && !videoGeneration && s.Category != "audio") { s.OutputModalities = add(s.OutputModalities, "text") }
	if imageGeneration { s.OutputModalities = add(s.OutputModalities, "image") }
	if hasAny(tags, "音乐", "音频") || hasAny(endpoints, "文本转语音", "语音合成", "tts", "音乐", "suno", "音效") { s.OutputModalities = add(s.OutputModalities, "audio") }
	if videoGeneration { s.OutputModalities = add(s.OutputModalities, "video") }
	if len(s.OutputModalities) == 0 { s.OutputModalities = []string{"text"} }
	if imageInput && s.Category == "text" { s.Category = "multimodal" }

	if hasAny(tags, "思考") || hasAny(all, "推理", "thinking", "reasoning", "深度思考") || reasoningModelPattern.MatchString(modelID) { s.Capabilities = add(s.Capabilities, "reasoning") }
	if imageInput { s.Capabilities = add(s.Capabilities, "vision") }
	if hasAny(tags, "工具") || hasAny(description, "工具调用", "function call", "function_call", "tool call", "tool_call", "agent") || hasAny(endpoints, "openai-response") { s.Capabilities = add(s.Capabilities, "tool_calling") }
	if hasAny(description, "结构化输出", "json mode", "json_mode", "structured output", "structured_output", "response_format") { s.Capabilities = add(s.Capabilities, "structured_output") }
	if hasAny(description, "联网", "搜索", "search", "browse", "web_search") || hasAny(tags, "联网搜索") { s.Capabilities = add(s.Capabilities, "web_search") }
	if hasAny(modelID+" "+description, "代码", "编程", "coding", "code", "codex", "coder") { s.Capabilities = add(s.Capabilities, "coding") }
	if s.Category == "embedding" { s.Capabilities = add(s.Capabilities, "embeddings") }
	if s.Category == "rerank" { s.Capabilities = add(s.Capabilities, "rerank") }
	if hasAny(endpoints, "realtime") || hasAny(tags, "实时语音") { s.Capabilities = add(s.Capabilities, "realtime") }
	if imageGeneration { s.Capabilities = add(s.Capabilities, "image_generation") }
	if videoGeneration { s.Capabilities = add(s.Capabilities, "video_generation") }
	if hasAny(endpoints, "语音转文字", "语音识别", "asr", "whisper") || hasAny(description, "语音识别", "speech to text", "speech_to_text", "asr") { s.Capabilities = add(s.Capabilities, "speech_to_text") }
	if hasAny(endpoints, "文本转语音", "语音合成", "tts") || hasAny(tags, "音频", "音乐") { s.Capabilities = add(s.Capabilities, "text_to_speech") }

	s.ContextWindow, s.MaxInputTokens, s.MaxOutputTokens = inferredCapacities(modelID, description)
	return s
}

type Syncer struct {
	DB     *sql.DB
	APIKey string
	Client *http.Client
	URL    string
}

func (s *Syncer) Sync(ctx context.Context) error {
	url := s.URL
	if url == "" {
		url = "https://ai-model-api.matrix-studio.top/v1/models"
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("matrix catalog returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data    []MatrixEntry `json:"data"`
		Object  string        `json:"object"`
		Success bool          `json:"success"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(&payload) != nil || payload.Object != "list" || !payload.Success {
		return fmt.Errorf("invalid matrix catalog response")
	}
	items := MergeEntries(payload.Data)
	if len(items) == 0 {
		return fmt.Errorf("matrix catalog is empty")
	}
	now := time.Now().UTC()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE matrix_model_catalog SET status='inactive',synced_at=$1,updated_at=$1`, now); err != nil {
		return err
	}
	for _, item := range items {
		owned, _ := json.Marshal(item.OwnedBy)
		types, _ := json.Marshal(item.ModelTypes)
		endpoints, _ := json.Marshal(item.SupportedEndpointTypes)
		tags, _ := json.Marshal(item.Tags)
		raw, _ := json.Marshal(item.RawEntries)
		if _, err = tx.ExecContext(ctx, `INSERT INTO matrix_model_catalog(id,model_id,description,owned_by,model_types,supported_endpoint_types,tags,raw_entries,status,last_seen_at,synced_at,created_at,updated_at) VALUES($1,$2,NULLIF($3,''),$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,'active',$9,$9,$9,$9) ON CONFLICT(model_id) DO UPDATE SET description=EXCLUDED.description,owned_by=EXCLUDED.owned_by,model_types=EXCLUDED.model_types,supported_endpoint_types=EXCLUDED.supported_endpoint_types,tags=EXCLUDED.tags,raw_entries=EXCLUDED.raw_entries,status='active',last_seen_at=$9,synced_at=$9,updated_at=$9`, uuid.Must(uuid.NewV7()), item.ModelID, item.Description, string(owned), string(types), string(endpoints), string(tags), string(raw), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
