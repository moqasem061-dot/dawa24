package identity

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/redis/go-redis/v9"
)

// Enforcing the concurrent-sign-in limit.
//
// This is what made signing in as an administrator hang until the server's
// 30-second write timeout fired and the proxy answered 502, while pharmacy and
// vendor accounts signed in normally.
//
// The old implementation walked the token set one token at a time, calling
// SessionStore.Get for each — a Redis round trip per token, and TWO for a token
// whose session had expired, because a miss also looks up the eviction reason.
// That alone is only slow in proportion to the set.
//
// What made the set grow without bound is that it was never cleaned. A token
// whose session key had expired made Get return an error, so the loop SKIPPED
// it — leaving it a member of the set forever, since only tokens that resolve
// to a live session are ever removed. Meanwhile Create SAdds the new token
// BEFORE enforcement runs and refreshes the set's TTL on every login, so the
// set outlives every session inside it and gains a permanent entry each time.
//
// The result is a ratchet, and it turns fastest on exactly the account that
// signs in most — the administrator's. Every sign-in, successful or timed out,
// added one more dead token that every later sign-in then had to pay two round
// trips to discover. Once the set is a few thousand entries, login exceeds any
// timeout, and each failed attempt makes the next one worse.
//
// Both limits are now enforced with a fixed number of round trips regardless of
// set size, and dead tokens are pruned out of the set as they are found, so a
// set that has already ballooned collapses back to the live sessions on the
// next sign-in and stays there.

// sessionSetEntry is one live session found while enforcing a limit.
type sessionSetEntry struct {
	token   string
	userID  int64
	orgID   int64
	created time.Time
}

// fetchSessionBlobs reads the stored session for each token.
//
// One MGET per batch rather than one GET per token: the round trips are the
// whole cost, and this is what turns enforcement from proportional-to-set-size
// into a fixed handful of calls. A nil entry means the token has no session.
func (s *SessionStore) fetchSessionBlobs(
	ctx context.Context, rdb redisCmdable, tokens []string,
) ([][]byte, error) {
	blobs := make([][]byte, 0, len(tokens))
	for _, batch := range chunk(tokens, sessionPipelineBatch) {
		keys := make([]string, len(batch))
		for i, tok := range batch {
			keys[i] = sessionKey(tok)
		}
		vals, err := rdb.MGet(ctx, keys...).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return nil, err
		}
		for i := range batch {
			if i >= len(vals) || vals[i] == nil {
				blobs = append(blobs, nil)
				continue
			}
			switch v := vals[i].(type) {
			case string:
				blobs = append(blobs, []byte(v))
			case []byte:
				blobs = append(blobs, v)
			default:
				blobs = append(blobs, nil)
			}
		}
	}
	return blobs, nil
}

// classifySessions splits a token set into the sessions that still exist and
// the tokens that do not.
//
// The dead half is the important one. The previous implementation had no
// concept of it: a token whose session had expired simply failed to load and
// was skipped, and since only live tokens were ever removed from the set, dead
// ones accumulated in it permanently. Returning them is what lets the caller
// prune the set and stop it growing forever.
func classifySessions(tokens []string, blobs [][]byte) (live []sessionSetEntry, dead []string) {
	for i, tok := range tokens {
		if i >= len(blobs) || blobs[i] == nil {
			dead = append(dead, tok)
			continue
		}
		var sess Session
		if json.Unmarshal(blobs[i], &sess) != nil {
			// Unreadable is as good as gone: it can never authenticate anyone.
			dead = append(dead, tok)
			continue
		}
		created := sess.CreatedAt
		if created.IsZero() {
			created = sess.LastActiveAt
		}
		live = append(live, sessionSetEntry{token: tok, userID: sess.UserID, orgID: sess.ActiveOrgID, created: created})
	}
	return live, dead
}

// readSessionSet resolves a token set into its live sessions and dead tokens.
func (s *SessionStore) readSessionSet(
	ctx context.Context, rdb redisCmdable, tokens []string,
) (live []sessionSetEntry, dead []string, err error) {
	if len(tokens) == 0 {
		return nil, nil, nil
	}
	blobs, err := s.fetchSessionBlobs(ctx, rdb, tokens)
	if err != nil {
		return nil, nil, err
	}
	live, dead = classifySessions(tokens, blobs)
	return live, dead, nil
}

// sessionPipelineBatch bounds how many keys go into one call.
//
// The round trips are what cost time, so batching is what fixes the hang; the
// cap only stops a set that has already grown to tens of thousands of dead
// tokens from being turned into one enormous request the first time it is
// cleaned up.
const sessionPipelineBatch = 500

func chunk(tokens []string, size int) [][]string {
	if len(tokens) == 0 {
		return nil
	}
	if size <= 0 || len(tokens) <= size {
		return [][]string{tokens}
	}
	out := make([][]string, 0, (len(tokens)+size-1)/size)
	for start := 0; start < len(tokens); start += size {
		end := start + size
		if end > len(tokens) {
			end = len(tokens)
		}
		out = append(out, tokens[start:end])
	}
	return out
}

// runPipelined applies queued writes in bounded batches.
func runPipelined(ctx context.Context, rdb redisCmdable, n int, add func(redis.Pipeliner, int)) error {
	for start := 0; start < n; start += sessionPipelineBatch {
		end := start + sessionPipelineBatch
		if end > n {
			end = n
		}
		pipe := rdb.Pipeline()
		for i := start; i < end; i++ {
			add(pipe, i)
		}
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
	}
	return nil
}

// enforceOrgLimit keeps the organization's total live session count at or below max by evicting
// the oldest active sessions across all members of that organization.
func (s *SessionStore) enforceOrgLimit(ctx context.Context, orgID int64, max int) error {
	rdb, err := s.client()
	if err != nil {
		return err
	}
	tokens, err := rdb.SMembers(ctx, orgSessionsKey(orgID)).Result()
	if err != nil {
		return err
	}

	live, dead, err := s.readSessionSet(ctx, rdb, tokens)
	if err != nil {
		return err
	}

	sort.Slice(live, func(i, j int) bool { return live[i].created.Before(live[j].created) })
	evict := live[:evictCount(len(live), max)]

	if err := runPipelined(ctx, rdb, len(dead), func(pipe redis.Pipeliner, i int) {
		pipe.SRem(ctx, orgSessionsKey(orgID), dead[i])
	}); err != nil {
		return err
	}
	return runPipelined(ctx, rdb, len(evict), func(pipe redis.Pipeliner, i int) {
		e := evict[i]
		pipe.Set(ctx, sessionEvictedKey(e.token), "org_concurrent_limit", 24*time.Hour)
		pipe.SRem(ctx, orgSessionsKey(orgID), e.token)
		pipe.SRem(ctx, userSessionsKey(e.userID), e.token)
		pipe.Del(ctx, sessionKey(e.token))
	})
}

// enforceLimit keeps the user's live session count at or below max by evicting
// the oldest sessions first.
func (s *SessionStore) enforceLimit(ctx context.Context, userID int64, max int) error {
	rdb, err := s.client()
	if err != nil {
		return err
	}
	tokens, err := rdb.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}

	live, dead, err := s.readSessionSet(ctx, rdb, tokens)
	if err != nil {
		return err
	}

	sort.Slice(live, func(i, j int) bool { return live[i].created.Before(live[j].created) })
	evict := live[:evictCount(len(live), max)]

	if err := runPipelined(ctx, rdb, len(dead), func(pipe redis.Pipeliner, i int) {
		pipe.SRem(ctx, userSessionsKey(userID), dead[i])
	}); err != nil {
		return err
	}
	return runPipelined(ctx, rdb, len(evict), func(pipe redis.Pipeliner, i int) {
		e := evict[i]
		pipe.Set(ctx, sessionEvictedKey(e.token), "duplicate_login", 24*time.Hour)
		pipe.SRem(ctx, userSessionsKey(userID), e.token)
		if e.orgID > 0 {
			pipe.SRem(ctx, orgSessionsKey(e.orgID), e.token)
		}
		pipe.Del(ctx, sessionKey(e.token))
	})
}

// evictCount returns how many of the oldest live sessions must go.
func evictCount(liveCount, max int) int {
	if max <= 0 || liveCount <= max {
		return 0
	}
	return liveCount - max
}
