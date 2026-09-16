package identity

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListForUser returns all live sessions for a user, newest first.
func (s *SessionStore) ListForUser(ctx context.Context, userID int64) ([]*Session, error) {
	rdb, err := s.client()
	if err != nil {
		s.memMu.RLock()
		defer s.memMu.RUnlock()
		var list []*Session
		if set, ok := s.memUserSessions[userID]; ok {
			for tok := range set {
				if sess, exists := s.memSessions[tok]; exists {
					list = append(list, sess)
				}
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
		return list, nil
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, apperr.Unavailable("redis", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	keys := make([]string, len(tokens))
	for i, tok := range tokens {
		keys[i] = sessionKey(tok)
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, apperr.Unavailable("redis", err)
	}

	var list []*Session
	var deadTokens []any
	now := time.Now().UTC()
	idleLimit := s.GetIdleTimeout()

	for i, v := range vals {
		if v == nil {
			deadTokens = append(deadTokens, tokens[i])
			continue
		}
		var data []byte
		switch val := v.(type) {
		case string:
			data = []byte(val)
		case []byte:
			data = val
		default:
			deadTokens = append(deadTokens, tokens[i])
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			deadTokens = append(deadTokens, tokens[i])
			continue
		}

		lastActive := sess.LastActiveAt
		if lastActive.IsZero() {
			lastActive = sess.CreatedAt
		}
		if idleLimit > 0 && now.Sub(lastActive) > idleLimit {
			deadTokens = append(deadTokens, tokens[i])
			continue
		}

		list = append(list, &sess)
	}

	if len(deadTokens) > 0 {
		_ = rdb.SRem(ctx, userSessionsKey(userID), deadTokens...).Err()
	}

	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

// ListForOrg returns all live sessions across an entire organization, newest first.
func (s *SessionStore) ListForOrg(ctx context.Context, orgID int64) ([]*Session, error) {
	rdb, err := s.client()
	if err != nil {
		s.memMu.RLock()
		defer s.memMu.RUnlock()
		var list []*Session
		if set, ok := s.memOrgSessions[orgID]; ok {
			for tok := range set {
				if sess, exists := s.memSessions[tok]; exists {
					list = append(list, sess)
				}
			}
		}
		sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
		return list, nil
	}
	tokens, err := rdb.SMembers(ctx, orgSessionsKey(orgID)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, apperr.Unavailable("redis", err)
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	keys := make([]string, len(tokens))
	for i, tok := range tokens {
		keys[i] = sessionKey(tok)
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, apperr.Unavailable("redis", err)
	}

	var list []*Session
	var deadTokens []any
	now := time.Now().UTC()
	idleLimit := s.GetIdleTimeout()

	for i, v := range vals {
		if v == nil {
			deadTokens = append(deadTokens, tokens[i])
			continue
		}
		var data []byte
		switch val := v.(type) {
		case string:
			data = []byte(val)
		case []byte:
			data = val
		default:
			deadTokens = append(deadTokens, tokens[i])
			continue
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			deadTokens = append(deadTokens, tokens[i])
			continue
		}

		lastActive := sess.LastActiveAt
		if lastActive.IsZero() {
			lastActive = sess.CreatedAt
		}
		if idleLimit > 0 && now.Sub(lastActive) > idleLimit {
			deadTokens = append(deadTokens, tokens[i])
			continue
		}

		list = append(list, &sess)
	}

	if len(deadTokens) > 0 {
		_ = rdb.SRem(ctx, orgSessionsKey(orgID), deadTokens...).Err()
	}

	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}
