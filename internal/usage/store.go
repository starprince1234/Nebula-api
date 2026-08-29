package usage

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/domain"
)

var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

type Store struct{ db *sql.DB }

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func Month(value time.Time) time.Time {
	local := value.In(shanghai)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, shanghai).UTC()
}

func ParseMonth(value string, now time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return Month(now), nil
	}
	parsed, err := time.Parse("2006-01", value)
	if err != nil {
		return time.Time{}, domain.NewError(domain.CodeValidation, "month must use YYYY-MM")
	}
	month := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, shanghai).UTC()
	if month.After(Month(now)) {
		return time.Time{}, domain.NewError(domain.CodeValidation, "future month is not allowed")
	}
	return month, nil
}

type Reservation struct {
	CallID, APIKeyID, ProjectID, OrganizationID, UserID, ModelID              uuid.UUID
	RequestID, APIKeyName, ProjectName, OrganizationName, UserName, ModelName string
	MultiplierMilli                                                           int64
	Month                                                                     time.Time
}

type QuotaError struct {
	Code, Name string
	EndsAt     time.Time
}

func (e *QuotaError) Error() string { return e.Code }

func (s *Store) Reserve(ctx context.Context, keyID, modelID uuid.UUID, requestID, protocol, path, prompt, promptSource string) (Reservation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Reservation{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var r Reservation
	r.CallID = uuid.Must(uuid.NewV7())
	r.APIKeyID, r.ModelID, r.RequestID, r.Month = keyID, modelID, requestID, Month(time.Now())
	var multiplier sql.NullInt64
	err = tx.QueryRowContext(ctx, `
SELECT k.project_id,p.organization_id,k.owner_user_id,k.name,p.name,o.name,u.name,m.display_name,m.credit_multiplier_milli
FROM api_keys k
JOIN projects p ON p.id=k.project_id JOIN organizations o ON o.id=p.organization_id
JOIN users u ON u.id=k.owner_user_id JOIN api_key_models km ON km.api_key_id=k.id
JOIN models m ON m.id=km.model_id
WHERE k.id=$1 AND k.status='active' AND m.id=$2 AND m.status='active'
FOR UPDATE OF p,k,m`, keyID, modelID).Scan(&r.ProjectID, &r.OrganizationID, &r.UserID, &r.APIKeyName, &r.ProjectName, &r.OrganizationName, &r.UserName, &r.ModelName, &multiplier)
	if errors.Is(err, sql.ErrNoRows) {
		return Reservation{}, domain.NewError(domain.CodeNotFound, "active API key or model is unavailable")
	}
	if err != nil {
		return Reservation{}, err
	}
	if !multiplier.Valid {
		return Reservation{}, domain.NewError(domain.CodeModelNotReady, "model credit multiplier is not configured")
	}
	r.MultiplierMilli = multiplier.Int64
	now := time.Now().UTC()
	if _, err = tx.ExecContext(ctx, `INSERT INTO project_month_credit_buckets(id,project_id,month,quota_milli,allocated_milli,charged_milli,pending_milli,created_at,updated_at)
SELECT $1,p.id,$2,p.monthly_credit_quota_milli,COALESCE((SELECT sum(k.monthly_credit_quota_milli) FROM api_keys k WHERE k.project_id=p.id AND k.status IN ('approved','active')),0),0,0,$3,$3 FROM projects p WHERE p.id=$4 ON CONFLICT(project_id,month) DO NOTHING`, uuid.Must(uuid.NewV7()), r.Month, now, r.ProjectID); err != nil {
		return Reservation{}, err
	}
	var projectQuota, projectCharged, projectPending int64
	if err = tx.QueryRowContext(ctx, `SELECT quota_milli,charged_milli,pending_milli FROM project_month_credit_buckets WHERE project_id=$1 AND month=$2 FOR UPDATE`, r.ProjectID, r.Month).Scan(&projectQuota, &projectCharged, &projectPending); err != nil {
		return Reservation{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO api_key_month_credit_buckets(id,api_key_id,project_id,month,quota_milli,allocation_active,charged_milli,pending_milli,created_at,updated_at)
SELECT $1,k.id,k.project_id,$2,k.monthly_credit_quota_milli,true,0,0,$3,$3 FROM api_keys k WHERE k.id=$4 ON CONFLICT(api_key_id,month) DO NOTHING`, uuid.Must(uuid.NewV7()), r.Month, now, keyID); err != nil {
		return Reservation{}, err
	}
	var keyQuota, keyCharged, keyPending int64
	if err = tx.QueryRowContext(ctx, `SELECT quota_milli,charged_milli,pending_milli FROM api_key_month_credit_buckets WHERE api_key_id=$1 AND month=$2 FOR UPDATE`, keyID, r.Month).Scan(&keyQuota, &keyCharged, &keyPending); err != nil {
		return Reservation{}, err
	}
	ends := r.Month.In(shanghai).AddDate(0, 1, 0).UTC()
	if r.MultiplierMilli > 0 && keyCharged+keyPending >= keyQuota {
		return Reservation{}, &QuotaError{Code: "api_key_quota_exceeded", Name: r.APIKeyName, EndsAt: ends}
	}
	if r.MultiplierMilli > 0 && projectCharged+projectPending >= projectQuota {
		return Reservation{}, &QuotaError{Code: "project_quota_exceeded", Name: r.ProjectName, EndsAt: ends}
	}
	lease := now.Add(15 * time.Minute)
	_, err = tx.ExecContext(ctx, `INSERT INTO gateway_calls(id,request_id,month,organization_id,project_id,user_id,api_key_id,model_id,organization_name,project_name,user_name,api_key_name,model_name,protocol,request_path,multiplier_milli,credit_milli,billing_state,outcome,lease_expires_at,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16,'pending','pending',$17,$18)`, r.CallID, r.RequestID, r.Month, r.OrganizationID, r.ProjectID, r.UserID, r.APIKeyID, r.ModelID, r.OrganizationName, r.ProjectName, r.UserName, r.APIKeyName, r.ModelName, protocol, path, r.MultiplierMilli, lease, now)
	if err != nil {
		return Reservation{}, err
	}
	if prompt != "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO monitored_inputs(id,call_id,project_id,user_id,source,content,content_bytes,visible,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,false,$8)`, uuid.Must(uuid.NewV7()), r.CallID, r.ProjectID, r.UserID, promptSource, prompt, len([]byte(prompt)), now)
		if err != nil {
			return Reservation{}, err
		}
	}
	if r.MultiplierMilli > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE project_month_credit_buckets SET pending_milli=pending_milli+$1,updated_at=$2 WHERE project_id=$3 AND month=$4`, r.MultiplierMilli, now, r.ProjectID, r.Month); err != nil {
			return Reservation{}, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE api_key_month_credit_buckets SET pending_milli=pending_milli+$1,updated_at=$2 WHERE api_key_id=$3 AND month=$4`, r.MultiplierMilli, now, r.APIKeyID, r.Month); err != nil {
			return Reservation{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Reservation{}, err
	}
	return r, nil
}

func (s *Store) MarkSent(ctx context.Context, callID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE gateway_calls SET sent_at=COALESCE(sent_at,now()),lease_expires_at=now()+interval '15 minutes' WHERE id=$1 AND billing_state='pending'`, callID)
	return err
}
func (s *Store) RenewLease(ctx context.Context, callID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE gateway_calls SET lease_expires_at=now()+interval '15 minutes' WHERE id=$1 AND billing_state='pending'`, callID)
	return err
}

func (s *Store) StartAttempt(ctx context.Context, callID, providerID, bindingID uuid.UUID, providerName string) (uuid.UUID, error) {
	id := uuid.Must(uuid.NewV7())
	_, err := s.db.ExecContext(ctx, `INSERT INTO gateway_call_attempts(id,call_id,provider_id,binding_id,provider_name,status,created_at) VALUES($1,$2,$3,$4,$5,'connecting',now())`, id, callID, providerID, bindingID, providerName)
	return id, err
}
func (s *Store) FinishAttempt(ctx context.Context, id uuid.UUID, succeeded bool, status *int, category, message string, started time.Time) error {
	state := "failed"
	if succeeded {
		state = "succeeded"
	}
	_, err := s.db.ExecContext(ctx, `UPDATE gateway_call_attempts SET status=$1,http_status=$2,error_category=NULLIF($3,''),error_message=NULLIF($4,''),latency_ms=$5,completed_at=now() WHERE id=$6`, state, status, category, sanitize(message), time.Since(started).Milliseconds(), id)
	return err
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > 500 {
		value = string([]rune(value)[:500])
	}
	return value
}

func (s *Store) Finalize(ctx context.Context, r Reservation, success bool, outcome, category, message string, providerID *uuid.UUID, providerName string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	state := "failed"
	if success {
		state = "charged"
	}
	result, err := tx.ExecContext(ctx, `UPDATE gateway_calls SET billing_state=$1,outcome=$2,error_category=NULLIF($3,''),error_message=NULLIF($4,''),provider_id=$5,provider_name=NULLIF($6,''),finalized_at=now(),lease_expires_at=NULL WHERE id=$7 AND billing_state='pending'`, state, outcome, category, sanitize(message), providerID, providerName, r.CallID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	if r.MultiplierMilli > 0 {
		if success {
			_, err = tx.ExecContext(ctx, `UPDATE project_month_credit_buckets SET pending_milli=pending_milli-$1,charged_milli=charged_milli+$1,updated_at=now() WHERE project_id=$2 AND month=$3 AND pending_milli>=$1`, r.MultiplierMilli, r.ProjectID, r.Month)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE api_key_month_credit_buckets SET pending_milli=pending_milli-$1,charged_milli=charged_milli+$1,updated_at=now() WHERE api_key_id=$2 AND month=$3 AND pending_milli>=$1`, r.MultiplierMilli, r.APIKeyID, r.Month)
			if err != nil {
				return err
			}
		} else {
			_, err = tx.ExecContext(ctx, `UPDATE project_month_credit_buckets SET pending_milli=pending_milli-$1,updated_at=now() WHERE project_id=$2 AND month=$3 AND pending_milli>=$1`, r.MultiplierMilli, r.ProjectID, r.Month)
			if err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, `UPDATE api_key_month_credit_buckets SET pending_milli=pending_milli-$1,updated_at=now() WHERE api_key_id=$2 AND month=$3 AND pending_milli>=$1`, r.MultiplierMilli, r.APIKeyID, r.Month)
			if err != nil {
				return err
			}
		}
	}
	if success {
		_, err = tx.ExecContext(ctx, `INSERT INTO monthly_usage_cube(id,month,organization_id,project_id,user_id,api_key_id,model_id,organization_name,project_name,user_name,api_key_name,model_name,charged_milli,charged_count,zero_cost_count,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,1,$14,now(),now()) ON CONFLICT(month,project_id,user_id,api_key_id,model_id) DO UPDATE SET charged_milli=monthly_usage_cube.charged_milli+EXCLUDED.charged_milli,charged_count=monthly_usage_cube.charged_count+1,zero_cost_count=monthly_usage_cube.zero_cost_count+EXCLUDED.zero_cost_count,organization_name=EXCLUDED.organization_name,project_name=EXCLUDED.project_name,user_name=EXCLUDED.user_name,api_key_name=EXCLUDED.api_key_name,model_name=EXCLUDED.model_name,updated_at=now()`, uuid.Must(uuid.NewV7()), r.Month, r.OrganizationID, r.ProjectID, r.UserID, r.APIKeyID, r.ModelID, r.OrganizationName, r.ProjectName, r.UserName, r.APIKeyName, r.ModelName, r.MultiplierMilli, boolInt(r.MultiplierMilli == 0))
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE monitored_inputs SET visible=true WHERE call_id=$1`, r.CallID)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM monitored_inputs WHERE call_id=$1`, r.CallID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

type UsageSlice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Credits string `json:"credits"`
	Calls   int64  `json:"calls,omitempty"`
}
type KeyUsage struct {
	ID, Name, Status, Quota, Used, Pending, Overage string
	Models                                          []UsageSlice
}
type StudentUsage struct {
	Month string     `json:"month"`
	Keys  []KeyUsage `json:"keys"`
}

func (s *Store) StudentUsage(ctx context.Context, userID uuid.UUID, month time.Time) (StudentUsage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT k.id,k.name,k.status,b.quota_milli,b.charged_milli,b.pending_milli FROM api_keys k JOIN api_key_month_credit_buckets b ON b.api_key_id=k.id AND b.month=$2 WHERE k.owner_user_id=$1 AND (k.status IN ('approved','active') OR b.charged_milli>0) ORDER BY k.created_at DESC`, userID, month)
	if err != nil {
		return StudentUsage{}, err
	}
	defer rows.Close()
	result := StudentUsage{Month: month.In(shanghai).Format("2006-01"), Keys: []KeyUsage{}}
	for rows.Next() {
		var id uuid.UUID
		var item KeyUsage
		var q, u, p int64
		if err := rows.Scan(&id, &item.Name, &item.Status, &q, &u, &p); err != nil {
			return result, err
		}
		item.ID = id.String()
		item.Quota = domain.FormatCredits(q)
		item.Used = domain.FormatCredits(u)
		item.Pending = domain.FormatCredits(p)
		if u > q {
			item.Overage = domain.FormatCredits(u - q)
		} else {
			item.Overage = "0.000"
		}
		mr, err := s.db.QueryContext(ctx, `SELECT model_id,model_name,charged_milli,charged_count FROM monthly_usage_cube WHERE api_key_id=$1 AND month=$2 ORDER BY charged_milli DESC,model_name`, id, month)
		if err != nil {
			return result, err
		}
		for mr.Next() {
			var mid uuid.UUID
			var sl UsageSlice
			var credits int64
			if err := mr.Scan(&mid, &sl.Name, &credits, &sl.Calls); err != nil {
				mr.Close()
				return result, err
			}
			sl.ID = mid.String()
			sl.Credits = domain.FormatCredits(credits)
			item.Models = append(item.Models, sl)
		}
		mr.Close()
		result.Keys = append(result.Keys, item)
	}
	return result, rows.Err()
}

type ProjectUsage struct {
	Month, ProjectID, ProjectName, Quota, Allocated, Charged, Pending, Available string
	Members                                                                      []MemberUsage
	Models                                                                       []UsageSlice
	FreeModels                                                                   []UsageSlice
}
type MemberUsage struct {
	UserID, UserName, Credits string
	Keys                      []KeyMemberUsage
}
type KeyMemberUsage struct{ ID, Name, Status, Quota, Used string }

func (s *Store) ProjectUsage(ctx context.Context, projectID uuid.UUID, month time.Time) (ProjectUsage, error) {
	var v ProjectUsage
	var q, a, c, p int64
	err := s.db.QueryRowContext(ctx, `SELECT p.name,b.quota_milli,b.allocated_milli,b.charged_milli,b.pending_milli FROM projects p JOIN project_month_credit_buckets b ON b.project_id=p.id AND b.month=$2 WHERE p.id=$1`, projectID, month).Scan(&v.ProjectName, &q, &a, &c, &p)
	if err != nil {
		return v, err
	}
	v.Month = month.In(shanghai).Format("2006-01")
	v.ProjectID = projectID.String()
	v.Quota = domain.FormatCredits(q)
	v.Allocated = domain.FormatCredits(a)
	v.Charged = domain.FormatCredits(c)
	v.Pending = domain.FormatCredits(p)
	available := q - a
	if available < 0 {
		available = 0
	}
	v.Available = domain.FormatCredits(available)
	rows, err := s.db.QueryContext(ctx, `SELECT u.id,u.name,COALESCE(sum(c.charged_milli),0) FROM users u JOIN api_keys k ON k.owner_user_id=u.id AND k.project_id=$1 LEFT JOIN monthly_usage_cube c ON c.api_key_id=k.id AND c.month=$2 WHERE k.status IN ('approved','active') OR c.id IS NOT NULL GROUP BY u.id,u.name ORDER BY u.name`, projectID, month)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid uuid.UUID
		var m MemberUsage
		var credit int64
		if err := rows.Scan(&uid, &m.UserName, &credit); err != nil {
			return v, err
		}
		m.UserID = uid.String()
		m.Credits = domain.FormatCredits(credit)
		kr, err := s.db.QueryContext(ctx, `SELECT k.id,k.name,k.status,b.quota_milli,b.charged_milli FROM api_keys k JOIN api_key_month_credit_buckets b ON b.api_key_id=k.id AND b.month=$3 WHERE k.project_id=$1 AND k.owner_user_id=$2 AND (k.status IN ('approved','active') OR b.charged_milli>0) ORDER BY k.name`, projectID, uid, month)
		if err != nil {
			return v, err
		}
		for kr.Next() {
			var kid uuid.UUID
			var ki KeyMemberUsage
			var kq, ku int64
			if err := kr.Scan(&kid, &ki.Name, &ki.Status, &kq, &ku); err != nil {
				kr.Close()
				return v, err
			}
			ki.ID = kid.String()
			ki.Quota = domain.FormatCredits(kq)
			ki.Used = domain.FormatCredits(ku)
			m.Keys = append(m.Keys, ki)
		}
		kr.Close()
		v.Members = append(v.Members, m)
	}
	mr, err := s.db.QueryContext(ctx, `SELECT model_id,model_name,sum(charged_milli),sum(charged_count),sum(zero_cost_count) FROM monthly_usage_cube WHERE project_id=$1 AND month=$2 GROUP BY model_id,model_name ORDER BY sum(charged_milli) DESC`, projectID, month)
	if err != nil {
		return v, err
	}
	defer mr.Close()
	for mr.Next() {
		var id uuid.UUID
		var name string
		var credits, calls, zero int64
		if err := mr.Scan(&id, &name, &credits, &calls, &zero); err != nil {
			return v, err
		}
		item := UsageSlice{ID: id.String(), Name: name, Credits: domain.FormatCredits(credits), Calls: calls}
		if credits > 0 {
			v.Models = append(v.Models, item)
		}
		if zero > 0 {
			item.Calls = zero
			v.FreeModels = append(v.FreeModels, item)
		}
	}
	return v, nil
}

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

func EncodeCursor(c Cursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}
func DecodeCursor(raw string) (*Cursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, domain.NewError(domain.CodeValidation, "invalid cursor")
	}
	var c Cursor
	if json.Unmarshal(data, &c) != nil || c.ID == uuid.Nil || c.CreatedAt.IsZero() {
		return nil, domain.NewError(domain.CodeValidation, "invalid cursor")
	}
	return &c, nil
}

type LogFilter struct {
	ProjectID, UserID, APIKeyID, ModelID *uuid.UUID
	Status                               string
	Start, End                           *time.Time
	Cursor                               *Cursor
	Limit                                int
}
type CallLog struct {
	ID, RequestID, ProjectID, ProjectName, UserID, UserName, APIKeyID, APIKeyName, ModelID, ModelName, ProviderName, Protocol, Path, Multiplier, Credits, BillingState, Outcome, ErrorCategory, ErrorMessage string
	CreatedAt                                                                                                                                                                                                time.Time
	Attempts                                                                                                                                                                                                 []AttemptLog
}
type AttemptLog struct {
	ProviderName, Status, ErrorCategory, ErrorMessage string
	HTTPStatus                                        *int
	LatencyMS                                         int64
}
type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func (s *Store) MentorCallLogs(ctx context.Context, mentorID uuid.UUID, f LogFilter) (Page[CallLog], error) {
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 50
	}
	args := []any{mentorID}
	where := []string{`EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=c.project_id AND pm.user_id=$1)`}
	add := func(condition string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(condition, len(args)))
	}
	if f.ProjectID != nil {
		add("c.project_id=$%d", *f.ProjectID)
	}
	if f.UserID != nil {
		add("c.user_id=$%d", *f.UserID)
	}
	if f.APIKeyID != nil {
		add("c.api_key_id=$%d", *f.APIKeyID)
	}
	if f.ModelID != nil {
		add("c.model_id=$%d", *f.ModelID)
	}
	if f.Status != "" {
		add("c.outcome=$%d", f.Status)
	}
	if f.Start != nil {
		add("c.created_at>=$%d", *f.Start)
	}
	if f.End != nil {
		add("c.created_at<$%d", *f.End)
	}
	if f.Cursor != nil {
		args = append(args, f.Cursor.CreatedAt, f.Cursor.ID)
		where = append(where, fmt.Sprintf("(c.created_at,c.id)<($%d,$%d)", len(args)-1, len(args)))
	}
	args = append(args, f.Limit+1)
	query := `SELECT c.id,c.request_id,c.project_id,c.project_name,c.user_id,c.user_name,c.api_key_id,c.api_key_name,c.model_id,c.model_name,COALESCE(c.provider_name,''),c.protocol,c.request_path,c.multiplier_milli,c.credit_milli,c.billing_state,c.outcome,COALESCE(c.error_category,''),COALESCE(c.error_message,''),c.created_at FROM gateway_calls c WHERE ` + strings.Join(where, " AND ") + fmt.Sprintf(" ORDER BY c.created_at DESC,c.id DESC LIMIT $%d", len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return Page[CallLog]{}, err
	}
	defer rows.Close()
	page := Page[CallLog]{Items: []CallLog{}}
	for rows.Next() {
		var ids [4]uuid.UUID
		var multiplier, credits int64
		var item CallLog
		if err := rows.Scan(&ids[0], &item.RequestID, &ids[1], &item.ProjectName, &ids[2], &item.UserName, &ids[3], &item.APIKeyName, &item.ModelID, &item.ModelName, &item.ProviderName, &item.Protocol, &item.Path, &multiplier, &credits, &item.BillingState, &item.Outcome, &item.ErrorCategory, &item.ErrorMessage, &item.CreatedAt); err != nil {
			return page, err
		}
		item.ID = ids[0].String()
		item.ProjectID = ids[1].String()
		item.UserID = ids[2].String()
		item.APIKeyID = ids[3].String()
		item.Multiplier = domain.FormatCredits(multiplier)
		item.Credits = domain.FormatCredits(credits)
		page.Items = append(page.Items, item)
	}
	if len(page.Items) > f.Limit {
		last := page.Items[f.Limit-1]
		id, _ := uuid.Parse(last.ID)
		cursor := EncodeCursor(Cursor{CreatedAt: last.CreatedAt, ID: id})
		page.NextCursor = &cursor
		page.Items = page.Items[:f.Limit]
	}
	return page, rows.Err()
}

func (s *Store) MentorCallLog(ctx context.Context, mentorID, callID uuid.UUID) (CallLog, error) {
	page, err := s.MentorCallLogs(ctx, mentorID, LogFilter{Limit: 100})
	if err != nil {
		return CallLog{}, err
	}
	var item *CallLog
	for i := range page.Items {
		if page.Items[i].ID == callID.String() {
			item = &page.Items[i]
			break
		}
	}
	if item == nil {
		return CallLog{}, domain.NewError(domain.CodeNotFound, "call log not found")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT provider_name,status,http_status,COALESCE(error_category,''),COALESCE(error_message,''),latency_ms FROM gateway_call_attempts WHERE call_id=$1 ORDER BY created_at`, callID)
	if err != nil {
		return CallLog{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var a AttemptLog
		if err := rows.Scan(&a.ProviderName, &a.Status, &a.HTTPStatus, &a.ErrorCategory, &a.ErrorMessage, &a.LatencyMS); err != nil {
			return CallLog{}, err
		}
		item.Attempts = append(item.Attempts, a)
	}
	return *item, nil
}

type InputMonitorItem struct {
	CallID, ProjectID, ProjectName, UserID, UserName, APIKeyName, ModelName, ProviderName, Credits, Outcome, Preview string
	ContentBytes                                                                                                     int64
	Truncated                                                                                                        bool
	CreatedAt                                                                                                        time.Time
	Content                                                                                                          *string `json:"content,omitempty"`
}
type InputFilter struct {
	LogFilter
	Query string
}

func (s *Store) MentorInputs(ctx context.Context, mentorID uuid.UUID, f InputFilter) (Page[InputMonitorItem], error) {
	if utf8.RuneCountInString(f.Query) > 0 && utf8.RuneCountInString(f.Query) < 3 {
		return Page[InputMonitorItem]{}, domain.NewError(domain.CodeValidation, "query must contain at least three characters")
	}
	if f.Limit < 1 || f.Limit > 100 {
		f.Limit = 50
	}
	args := []any{mentorID}
	where := []string{`i.visible=true`, `i.created_at>=now()-interval '90 days'`, `EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=i.project_id AND pm.user_id=$1)`}
	if f.Query != "" {
		args = append(args, "%"+f.Query+"%")
		where = append(where, fmt.Sprintf("i.content ILIKE $%d", len(args)))
	}
	args = append(args, f.Limit+1)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Page[InputMonitorItem]{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.ExecContext(ctx, `SET LOCAL statement_timeout='3s'`); err != nil {
		return Page[InputMonitorItem]{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.id,c.project_id,c.project_name,c.user_id,c.user_name,c.api_key_name,c.model_name,COALESCE(c.provider_name,''),c.credit_milli,c.outcome,left(i.content,200),i.content_bytes,length(i.content)>200,c.created_at,i.id FROM monitored_inputs i JOIN gateway_calls c ON c.id=i.call_id WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(" ORDER BY c.created_at DESC,c.id DESC LIMIT $%d", len(args)), args...)
	if err != nil {
		return Page[InputMonitorItem]{}, err
	}
	page := Page[InputMonitorItem]{Items: []InputMonitorItem{}}
	ids := []string{}
	for rows.Next() {
		var call, project, user, input uuid.UUID
		var credit int64
		var item InputMonitorItem
		if err := rows.Scan(&call, &project, &item.ProjectName, &user, &item.UserName, &item.APIKeyName, &item.ModelName, &item.ProviderName, &credit, &item.Outcome, &item.Preview, &item.ContentBytes, &item.Truncated, &item.CreatedAt, &input); err != nil {
			rows.Close()
			return page, err
		}
		item.CallID = call.String()
		item.ProjectID = project.String()
		item.UserID = user.String()
		item.Credits = domain.FormatCredits(credit)
		page.Items = append(page.Items, item)
		ids = append(ids, input.String())
	}
	rows.Close()
	scope, _ := json.Marshal(map[string]any{"q": f.Query})
	resultIDs, _ := json.Marshal(ids)
	_, err = tx.ExecContext(ctx, `INSERT INTO prompt_access_audits(id,actor_user_id,access_type,query_scope,result_ids,result_count,created_at) VALUES($1,$2,'list',$3,$4,$5,now())`, uuid.Must(uuid.NewV7()), mentorID, scope, resultIDs, len(ids))
	if err != nil {
		return Page[InputMonitorItem]{}, err
	}
	if err = tx.Commit(); err != nil {
		return Page[InputMonitorItem]{}, err
	}
	return page, nil
}
func (s *Store) MentorInput(ctx context.Context, mentorID, callID uuid.UUID) (InputMonitorItem, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InputMonitorItem{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var item InputMonitorItem
	var project, user, input uuid.UUID
	var credit int64
	var content string
	err = tx.QueryRowContext(ctx, `SELECT c.project_id,c.project_name,c.user_id,c.user_name,c.api_key_name,c.model_name,COALESCE(c.provider_name,''),c.credit_milli,c.outcome,i.content,i.content_bytes,c.created_at,i.id FROM monitored_inputs i JOIN gateway_calls c ON c.id=i.call_id WHERE c.id=$1 AND i.visible=true AND i.created_at>=now()-interval '90 days' AND EXISTS(SELECT 1 FROM project_members pm WHERE pm.project_id=i.project_id AND pm.user_id=$2)`, callID, mentorID).Scan(&project, &item.ProjectName, &user, &item.UserName, &item.APIKeyName, &item.ModelName, &item.ProviderName, &credit, &item.Outcome, &content, &item.ContentBytes, &item.CreatedAt, &input)
	if errors.Is(err, sql.ErrNoRows) {
		return InputMonitorItem{}, domain.NewError(domain.CodeNotFound, "monitored input not found")
	}
	if err != nil {
		return InputMonitorItem{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO prompt_access_audits(id,actor_user_id,project_id,monitored_input_id,access_type,result_count,created_at) VALUES($1,$2,$3,$4,'detail',1,now())`, uuid.Must(uuid.NewV7()), mentorID, project, input)
	if err != nil {
		return InputMonitorItem{}, err
	}
	if err = tx.Commit(); err != nil {
		return InputMonitorItem{}, err
	}
	item.CallID = callID.String()
	item.ProjectID = project.String()
	item.UserID = user.String()
	item.Credits = domain.FormatCredits(credit)
	item.Content = &content
	return item, nil
}

func (s *Store) RecoverAndClean(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,request_id,month,organization_id,project_id,user_id,api_key_id,model_id,organization_name,project_name,user_name,api_key_name,model_name,multiplier_milli FROM gateway_calls WHERE billing_state='pending' AND lease_expires_at<now() ORDER BY lease_expires_at LIMIT 100`)
	if err != nil {
		return err
	}
	list := []Reservation{}
	for rows.Next() {
		var r Reservation
		if err := rows.Scan(&r.CallID, &r.RequestID, &r.Month, &r.OrganizationID, &r.ProjectID, &r.UserID, &r.APIKeyID, &r.ModelID, &r.OrganizationName, &r.ProjectName, &r.UserName, &r.APIKeyName, &r.ModelName, &r.MultiplierMilli); err != nil {
			rows.Close()
			return err
		}
		list = append(list, r)
	}
	rows.Close()
	for _, r := range list {
		if err := s.Finalize(ctx, r, true, "outcome_unknown", "recovered_after_crash", "request outcome is unknown after process recovery", nil, ""); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM monitored_inputs WHERE created_at<now()-interval '90 days' LIMIT 1000`)
	if err != nil {
		_, err = s.db.ExecContext(ctx, `DELETE FROM monitored_inputs WHERE id IN(SELECT id FROM monitored_inputs WHERE created_at<now()-interval '90 days' LIMIT 1000)`)
	}
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM gateway_call_attempts WHERE call_id IN(SELECT id FROM gateway_calls WHERE created_at<now()-interval '365 days' LIMIT 1000)`)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM gateway_calls WHERE id IN(SELECT id FROM gateway_calls WHERE created_at<now()-interval '365 days' AND NOT EXISTS(SELECT 1 FROM monitored_inputs i WHERE i.call_id=gateway_calls.id) LIMIT 1000)`)
	return err
}
