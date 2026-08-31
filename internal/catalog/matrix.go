package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
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

func SuggestConfiguration(item Item) Suggestion {
	all := strings.ToLower(strings.Join(append(append([]string{}, item.ModelTypes...), append(item.Tags, item.SupportedEndpointTypes...)...), " "))
	s := Suggestion{Category: "text", InputModalities: []string{"text"}, OutputModalities: []string{"text"}}
	if strings.Contains(all, "检索") || strings.Contains(all, "rerank") {
		s.Category = "rerank"
		s.Capabilities = add(s.Capabilities, "rerank")
	}
	if strings.Contains(all, "嵌入") || strings.Contains(all, "embedding") {
		s.Category = "embedding"
		s.Capabilities = add(s.Capabilities, "embeddings")
	}
	if strings.Contains(all, "识图") || strings.Contains(all, "图像识别") {
		s.Category = "multimodal"
		s.InputModalities = add(s.InputModalities, "image")
		s.Capabilities = add(s.Capabilities, "vision")
	}
	if strings.Contains(all, "思考") || strings.Contains(all, "推理") {
		s.Capabilities = add(s.Capabilities, "reasoning")
	}
	if strings.Contains(all, "工具") {
		s.Capabilities = add(s.Capabilities, "tool_calling")
	}
	if strings.Contains(all, "realtime") {
		s.Capabilities = add(s.Capabilities, "realtime")
		s.InputModalities = add(s.InputModalities, "audio")
		s.OutputModalities = add(s.OutputModalities, "audio")
	}
	if strings.Contains(all, "语音转文字") {
		s.Category = "audio"
		s.InputModalities = []string{"audio"}
		s.Capabilities = add(s.Capabilities, "speech_to_text")
	}
	if strings.Contains(all, "文本转语音") || strings.Contains(all, "语音合成") {
		s.Category = "audio"
		s.OutputModalities = []string{"audio"}
		s.Capabilities = add(s.Capabilities, "text_to_speech")
	}
	if strings.Contains(all, "image-generation") || strings.Contains(all, "生图") {
		s.Category = "image"
		s.OutputModalities = []string{"image"}
		s.Capabilities = add(s.Capabilities, "image_generation")
	}
	if strings.Contains(all, "视频") {
		s.Category = "video"
		s.OutputModalities = add(s.OutputModalities, "video")
		s.Capabilities = add(s.Capabilities, "video_generation")
	}
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
