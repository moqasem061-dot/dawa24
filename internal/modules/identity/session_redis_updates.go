package identity

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/moqasem061-dot/dawa24/internal/shared/apperr"
)

func deleteRedisSession(ctx context.Context, rdb *redis.Client, sess *Session) error {
	pipe := rdb.TxPipeline()
	pipe.Del(ctx, sessionKey(sess.Token))
	pipe.SRem(ctx, userSessionsKey(sess.UserID), sess.Token)
	if sess.ActiveOrgID > 0 {
		pipe.SRem(ctx, orgSessionsKey(sess.ActiveOrgID), sess.Token)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return apperr.Unavailable("redis", err)
	}
	return nil
}

// A delayed touch must neither resurrect a revoked session nor overwrite newer activity.
var touchSessionScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'XX', 'KEEPTTL')
return 1
`)

func touchRedisSession(ctx context.Context, rdb *redis.Client, sess *Session, original []byte) error {
	blob, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("session: marshal activity: %w", err)
	}
	if err := touchSessionScript.Run(ctx, rdb, []string{sessionKey(sess.Token)}, original, blob).Err(); err != nil {
		return apperr.Unavailable("redis", err)
	}
	return nil
}

// An idle check must not delete activity recorded by a concurrent request.
var expireSessionScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('DEL', KEYS[1])
redis.call('SREM', KEYS[2], ARGV[2])
if ARGV[3] ~= '0' then redis.call('SREM', KEYS[3], ARGV[2]) end
redis.call('SET', KEYS[4], 'idle_timeout', 'EX', 86400)
return 1
`)

func expireRedisSession(ctx context.Context, rdb *redis.Client, sess *Session, original []byte) (bool, error) {
	keys := []string{sessionKey(sess.Token), userSessionsKey(sess.UserID), orgSessionsKey(sess.ActiveOrgID), sessionEvictedKey(sess.Token)}
	deleted, err := expireSessionScript.Run(ctx, rdb, keys, original, sess.Token, sess.ActiveOrgID).Int64()
	if err != nil {
		return false, apperr.Unavailable("redis", err)
	}
	return deleted == 1, nil
}
