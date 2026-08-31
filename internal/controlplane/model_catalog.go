package controlplane

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/starprince1234/Nebula-api/internal/catalog"
	"github.com/starprince1234/Nebula-api/internal/domain"
)

type ModelCatalogCandidate struct {
	ModelID                string             `json:"model_id"`
	Description            *string            `json:"description"`
	OwnedBy                []string           `json:"owned_by"`
	ModelTypes             []string           `json:"model_types"`
	SupportedEndpointTypes []string           `json:"supported_endpoint_types"`
	Tags                   []string           `json:"tags"`
	Suggestion             catalog.Suggestion `json:"suggestion"`
}
type ModelCatalogPage struct {
	Items    []ModelCatalogCandidate `json:"items"`
	Page     int                     `json:"page"`
	PageSize int                     `json:"page_size"`
	Total    int                     `json:"total"`
}

func (s *Service) ModelCatalogCandidates(ctx context.Context, query string, page, pageSize int) (ModelCatalogPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 50 {
		pageSize = 50
	}
	query = strings.TrimSpace(query)
	pattern := "%" + query + "%"
	var total int
	if err := s.sqlDB.QueryRowContext(ctx, `SELECT count(*) FROM matrix_model_catalog WHERE status='active' AND ($1='' OR model_id ILIKE $2)`, query, pattern).Scan(&total); err != nil {
		return ModelCatalogPage{}, domain.WrapError(domain.CodeDependencyUnavailable, "count model catalog candidates", err)
	}
	rows, err := s.sqlDB.QueryContext(ctx, `SELECT model_id,description,owned_by,model_types,supported_endpoint_types,tags FROM matrix_model_catalog WHERE status='active' AND ($1='' OR model_id ILIKE $2) ORDER BY lower(model_id),model_id LIMIT $3 OFFSET $4`, query, pattern, pageSize, (page-1)*pageSize)
	if err != nil {
		return ModelCatalogPage{}, domain.WrapError(domain.CodeDependencyUnavailable, "list model catalog candidates", err)
	}
	defer rows.Close()
	result := ModelCatalogPage{Items: []ModelCatalogCandidate{}, Page: page, PageSize: pageSize, Total: total}
	for rows.Next() {
		var item ModelCatalogCandidate
		var description *string
		var owned, types, endpoints, tags []byte
		if err := rows.Scan(&item.ModelID, &description, &owned, &types, &endpoints, &tags); err != nil {
			return result, err
		}
		item.Description = description
		_ = json.Unmarshal(owned, &item.OwnedBy)
		_ = json.Unmarshal(types, &item.ModelTypes)
		_ = json.Unmarshal(endpoints, &item.SupportedEndpointTypes)
		_ = json.Unmarshal(tags, &item.Tags)
		item.Suggestion = catalog.SuggestConfiguration(catalog.Item{ModelTypes: item.ModelTypes, SupportedEndpointTypes: item.SupportedEndpointTypes, Tags: item.Tags})
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
