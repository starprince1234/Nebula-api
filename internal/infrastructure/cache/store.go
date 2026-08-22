package cache

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrRateLimited         = errors.New("rate limited")
	ErrVerificationMissing = errors.New("verification missing")
	ErrVerificationInvalid = errors.New("verification invalid")
	ErrVerificationLocked  = errors.New("verification locked")
	ErrSessionMissing      = errors.New("session missing")
	ErrSessionReused       = errors.New("session reused")
	ErrSessionRevoked      = errors.New("session revoked")
	ErrInvitationMissing   = errors.New("invitation missing")
)

type Store struct {
	client          *redis.Client
	streamMaxLength int64
}

type verificationState struct {
	Hash        string    `json:"hash"`
	Attempts    int       `json:"attempts"`
	LockedUntil time.Time `json:"locked_until,omitempty"`
}

type Session struct {
	UserID   string `json:"user_id"`
	FamilyID string `json:"family_id"`
}

type Invitation struct {
	Email     string `json:"email"`
	InviterID string `json:"inviter_id"`
}

type Event struct {
	ID   string
	Type string
	Data string
}

func New(redisURL string, streamMaxLength int64) (*Store, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	return &Store{client: redis.NewClient(options), streamMaxLength: streamMaxLength}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping Redis: %w", err)
	}
	return nil
}

func (s *Store) PutVerification(
	ctx context.Context,
	email, purpose string,
	hash []byte,
	ttl, cooldown time.Duration,
) error {
	cooldownKey := verificationCooldownKey(email, purpose)
	ok, err := s.client.SetNX(ctx, cooldownKey, "1", cooldown).Result()
	if err != nil {
		return fmt.Errorf("set verification cooldown: %w", err)
	}
	if !ok {
		return ErrRateLimited
	}
	state := verificationState{Hash: base64.RawURLEncoding.EncodeToString(hash)}
	payload, err := json.Marshal(state)
	if err != nil {
		_ = s.client.Del(ctx, cooldownKey).Err()
		return fmt.Errorf("encode verification state: %w", err)
	}
	if err := s.client.Set(ctx, verificationKey(email, purpose), payload, ttl).Err(); err != nil {
		_ = s.client.Del(ctx, cooldownKey).Err()
		return fmt.Errorf("store verification state: %w", err)
	}
	return nil
}

func (s *Store) DeleteVerification(ctx context.Context, email, purpose string) error {
	return s.client.Del(ctx, verificationKey(email, purpose), verificationCooldownKey(email, purpose)).Err()
}

func (s *Store) ConsumeVerification(
	ctx context.Context,
	email, purpose string,
	hash []byte,
	maxAttempts int,
	lockout time.Duration,
) error {
	key := verificationKey(email, purpose)
	expected := base64.RawURLEncoding.EncodeToString(hash)
	for attempt := 0; attempt < 5; attempt++ {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, key).Bytes()
			if errors.Is(err, redis.Nil) {
				return ErrVerificationMissing
			}
			if err != nil {
				return err
			}
			var state verificationState
			if err := json.Unmarshal(raw, &state); err != nil {
				return fmt.Errorf("decode verification state: %w", err)
			}
			now := time.Now().UTC()
			if state.LockedUntil.After(now) {
				return ErrVerificationLocked
			}
			if subtle.ConstantTimeCompare([]byte(state.Hash), []byte(expected)) == 1 {
				_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
					pipe.Del(ctx, key)
					return nil
				})
				return err
			}
			state.Attempts++
			ttl, err := tx.TTL(ctx, key).Result()
			if err != nil || ttl <= 0 {
				return ErrVerificationMissing
			}
			if state.Attempts >= maxAttempts {
				state.LockedUntil = now.Add(lockout)
				if lockout > ttl {
					ttl = lockout
				}
			}
			updated, err := json.Marshal(state)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, updated, ttl)
				return nil
			})
			if err != nil {
				return err
			}
			if state.LockedUntil.After(now) {
				return ErrVerificationLocked
			}
			return ErrVerificationInvalid
		}, key)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return fmt.Errorf("verification state changed concurrently")
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, session Session, ttl time.Duration) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("encode refresh session: %w", err)
	}
	_, err = s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, refreshKey(tokenHash), payload, ttl)
		pipe.SAdd(ctx, userFamiliesKey(session.UserID), session.FamilyID)
		pipe.Expire(ctx, userFamiliesKey(session.UserID), ttl)
		return nil
	})
	return err
}

func (s *Store) RotateSession(
	ctx context.Context,
	oldHash, newHash string,
	newSession Session,
	ttl time.Duration,
) error {
	oldKey := refreshKey(oldHash)
	for attempt := 0; attempt < 5; attempt++ {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			raw, err := tx.Get(ctx, oldKey).Bytes()
			if errors.Is(err, redis.Nil) {
				usedRaw, usedErr := tx.Get(ctx, usedRefreshKey(oldHash)).Bytes()
				if usedErr == nil {
					var used Session
					if json.Unmarshal(usedRaw, &used) == nil {
						_ = s.client.Set(ctx, revokedFamilyKey(used.FamilyID), "1", ttl).Err()
					}
					return ErrSessionReused
				}
				return ErrSessionMissing
			}
			if err != nil {
				return err
			}
			var current Session
			if err := json.Unmarshal(raw, &current); err != nil {
				return fmt.Errorf("decode refresh session: %w", err)
			}
			if current.UserID != newSession.UserID || current.FamilyID != newSession.FamilyID {
				return ErrSessionMissing
			}
			if exists, err := tx.Exists(ctx, revokedFamilyKey(current.FamilyID)).Result(); err != nil {
				return err
			} else if exists > 0 {
				return ErrSessionRevoked
			}
			newPayload, err := json.Marshal(newSession)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Del(ctx, oldKey)
				pipe.Set(ctx, usedRefreshKey(oldHash), raw, ttl)
				pipe.Set(ctx, refreshKey(newHash), newPayload, ttl)
				pipe.SAdd(ctx, userFamiliesKey(newSession.UserID), newSession.FamilyID)
				pipe.Expire(ctx, userFamiliesKey(newSession.UserID), ttl)
				return nil
			})
			return err
		}, oldKey)
		if errors.Is(err, redis.TxFailedErr) {
			continue
		}
		return err
	}
	return fmt.Errorf("refresh session changed concurrently")
}

func (s *Store) GetSession(ctx context.Context, tokenHash string) (Session, error) {
	raw, err := s.client.Get(ctx, refreshKey(tokenHash)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Session{}, ErrSessionMissing
	}
	if err != nil {
		return Session{}, err
	}
	var session Session
	if err := json.Unmarshal(raw, &session); err != nil {
		return Session{}, fmt.Errorf("decode refresh session: %w", err)
	}
	if exists, err := s.client.Exists(ctx, revokedFamilyKey(session.FamilyID)).Result(); err != nil {
		return Session{}, err
	} else if exists > 0 {
		return Session{}, ErrSessionRevoked
	}
	return session, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	return s.client.Del(ctx, refreshKey(tokenHash)).Err()
}

func (s *Store) RevokeUserSessions(ctx context.Context, userID string, ttl time.Duration) error {
	families, err := s.client.SMembers(ctx, userFamiliesKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	_, err = s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, family := range families {
			pipe.Set(ctx, revokedFamilyKey(family), "1", ttl)
		}
		pipe.Del(ctx, userFamiliesKey(userID))
		return nil
	})
	return err
}

func (s *Store) PutInvitation(ctx context.Context, tokenHash string, invitation Invitation, ttl time.Duration) error {
	payload, err := json.Marshal(invitation)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, invitationKey(tokenHash), payload, ttl).Err()
}

func (s *Store) ConsumeInvitation(ctx context.Context, tokenHash string) (Invitation, error) {
	raw, err := s.client.GetDel(ctx, invitationKey(tokenHash)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Invitation{}, ErrInvitationMissing
	}
	if err != nil {
		return Invitation{}, err
	}
	var invitation Invitation
	if err := json.Unmarshal(raw, &invitation); err != nil {
		return Invitation{}, fmt.Errorf("decode invitation: %w", err)
	}
	return invitation, nil
}

func (s *Store) GetInvitation(ctx context.Context, tokenHash string) (Invitation, error) {
	raw, err := s.client.Get(ctx, invitationKey(tokenHash)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Invitation{}, ErrInvitationMissing
	}
	if err != nil {
		return Invitation{}, err
	}
	var invitation Invitation
	if err := json.Unmarshal(raw, &invitation); err != nil {
		return Invitation{}, fmt.Errorf("decode invitation: %w", err)
	}
	return invitation, nil
}

func (s *Store) DeleteInvitation(ctx context.Context, tokenHash string) error {
	return s.client.Del(ctx, invitationKey(tokenHash)).Err()
}

func (s *Store) PublishUserEvent(ctx context.Context, userID, eventType string, payload any) error {
	return s.publish(ctx, userEventStream(userID), eventType, payload)
}

func (s *Store) PublishGlobalEvent(ctx context.Context, eventType string, payload any) error {
	return s.publish(ctx, globalEventStream(), eventType, payload)
}

func (s *Store) ReadEvents(
	ctx context.Context,
	userID, lastID string,
	block time.Duration,
) ([]Event, error) {
	if lastID == "" {
		lastID = "$"
	}
	streams, err := s.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{userEventStream(userID), globalEventStream(), lastID, lastID},
		Block:   block,
		Count:   100,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	events := make([]Event, 0)
	for _, stream := range streams {
		for _, message := range stream.Messages {
			eventType, _ := message.Values["type"].(string)
			data, _ := message.Values["data"].(string)
			events = append(events, Event{ID: message.ID, Type: eventType, Data: data})
		}
	}
	return events, nil
}

func (s *Store) SetGatewayResourceRoute(ctx context.Context, resourceType, resourceID, bindingID string, ttl time.Duration) error {
	return s.client.Set(ctx, gatewayResourceRouteKey(resourceType, resourceID), bindingID, ttl).Err()
}

func (s *Store) GetGatewayResourceRoute(ctx context.Context, resourceType, resourceID string) (string, error) {
	value, err := s.client.Get(ctx, gatewayResourceRouteKey(resourceType, resourceID)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrSessionMissing
	}
	return value, err
}

func (s *Store) publish(ctx context.Context, stream, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode event payload: %w", err)
	}
	return s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: s.streamMaxLength,
		Approx: true,
		Values: map[string]any{"type": eventType, "data": string(data)},
	}).Err()
}

func verificationKey(email, purpose string) string {
	return "auth:verification:" + purpose + ":" + email
}

func verificationCooldownKey(email, purpose string) string {
	return "auth:verification:cooldown:" + purpose + ":" + email
}

func refreshKey(hash string) string {
	return "auth:refresh:" + hash
}

func usedRefreshKey(hash string) string {
	return "auth:refresh:used:" + hash
}

func revokedFamilyKey(familyID string) string {
	return "auth:refresh:family:revoked:" + familyID
}

func userFamiliesKey(userID string) string {
	return "auth:refresh:user:" + userID + ":families"
}

func invitationKey(hash string) string {
	return "auth:teacher-invitation:" + hash
}

func userEventStream(userID string) string {
	return "events:user:" + userID
}

func globalEventStream() string {
	return "events:global"
}

func gatewayResourceRouteKey(resourceType, resourceID string) string {
	return "gateway:resource-route:" + resourceType + ":" + resourceID
}

func HashKey(hash []byte) string {
	return base64.RawURLEncoding.EncodeToString(hash)
}

func StringValue(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case int64:
		return strconv.FormatInt(value, 10)
	default:
		return fmt.Sprint(value)
	}
}
