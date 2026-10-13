package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/redis/go-redis/v9"
)

const (
	// keyPrefix namespaces every key this package owns.
	keyPrefix = "ow:stats:"

	// profileSuffix marks the profile-only variant of a player entry.
	profileSuffix = ":profile"

	// scanBatch is the COUNT hint used when iterating keys.
	scanBatch = 500

	// maxScanKeys bounds how many keys a single iteration returns, so an
	// oversized cache cannot produce an unbounded response or allocation.
	maxScanKeys = 100_000

	// maxRetention bounds how long an entry is kept alive while its refreshes keep
	// failing, so a player that is really gone eventually drops out of the cache.
	maxRetention = 7 * 24 * time.Hour
)

// Entry wraps a cached value with the time it was written, which lets callers
// implement stale-while-revalidate on top of the hard Redis TTL.
type Entry[T any] struct {
	CachedAt time.Time `json:"cachedAt"`
	Value    *T        `json:"value"`
}

// RedisCache wraps the Redis client for caching player stats.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCache creates a new Redis cache client and verifies connectivity.
func NewRedisCache(ctx context.Context, host string, port int, password string, db int, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:            net.JoinHostPort(host, fmt.Sprint(port)),
		Password:        password,
		DB:              db,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
		PoolSize:        20,
		MinIdleConns:    2,
		ConnMaxIdleTime: 5 * time.Minute,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	slog.Info("connected to Redis", "host", host, "port", port)

	return &RedisCache{client: client, ttl: ttl}, nil
}

// TTL returns the configured hard expiry of cache entries.
func (c *RedisCache) TTL() time.Duration { return c.ttl }

// makeKey generates a cache key for a player. Both components are escaped so a
// tag containing ':' cannot collide with, or forge, another key.
func makeKey(platform, tag string) string {
	return keyPrefix + url.QueryEscape(platform) + ":" + url.QueryEscape(tag)
}

// SplitKey reverses makeKey and reports whether the key refers to a profile entry.
func SplitKey(key string) (platform, tag string, isProfile bool, ok bool) {
	rest, found := strings.CutPrefix(key, keyPrefix)
	if !found {
		return "", "", false, false
	}
	if trimmed, found := strings.CutSuffix(rest, profileSuffix); found {
		rest, isProfile = trimmed, true
	}
	rawPlatform, rawTag, found := strings.Cut(rest, ":")
	if !found {
		return "", "", false, false
	}
	platform, err := url.QueryUnescape(rawPlatform)
	if err != nil {
		return "", "", false, false
	}
	tag, err = url.QueryUnescape(rawTag)
	if err != nil {
		return "", "", false, false
	}
	return platform, tag, isProfile, true
}

// getEntry loads and decodes a cache entry. A miss returns (nil, nil).
func getEntry[T any](ctx context.Context, c *RedisCache, key string) (*Entry[T], error) {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil // Cache miss.
		}
		return nil, fmt.Errorf("failed to get %q from cache: %w", key, err)
	}

	var entry Entry[T]
	if err := json.Unmarshal(data, &entry); err != nil {
		// A malformed entry is treated as a miss rather than a hard failure, so a
		// schema change cannot take the endpoint down.
		slog.Warn("discarding malformed cache entry", "key", key, "error", err)
		return nil, nil
	}
	if entry.Value == nil {
		return nil, nil
	}
	return &entry, nil
}

// setEntry encodes and stores a cache entry.
func setEntry[T any](ctx context.Context, c *RedisCache, key string, value *T) error {
	data, err := json.Marshal(Entry[T]{CachedAt: time.Now(), Value: value})
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}
	if err := c.client.Set(ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to set %q: %w", key, err)
	}
	return nil
}

// Get retrieves cached player stats. A miss returns (nil, nil).
func (c *RedisCache) Get(ctx context.Context, platform, tag string) (*Entry[ovrstat.PlayerStats], error) {
	return getEntry[ovrstat.PlayerStats](ctx, c, makeKey(platform, tag))
}

// Set stores player stats in the cache.
func (c *RedisCache) Set(ctx context.Context, platform, tag string, stats *ovrstat.PlayerStats) error {
	return setEntry(ctx, c, makeKey(platform, tag), stats)
}

// GetProfile retrieves cached player profile stats. A miss returns (nil, nil).
func (c *RedisCache) GetProfile(ctx context.Context, platform, tag string) (*Entry[ovrstat.PlayerStatsProfile], error) {
	return getEntry[ovrstat.PlayerStatsProfile](ctx, c, makeKey(platform, tag)+profileSuffix)
}

// SetProfile stores player profile stats in the cache.
func (c *RedisCache) SetProfile(ctx context.Context, platform, tag string, stats *ovrstat.PlayerStatsProfile) error {
	return setEntry(ctx, c, makeKey(platform, tag)+profileSuffix, stats)
}

// Retain extends the expiry of an existing entry after a failed refresh, so a
// flaky upstream cannot evict good data and turn it into "player not found".
// Entries older than maxRetention are left to expire.
func (c *RedisCache) Retain(ctx context.Context, platform, tag string, isProfile bool) error {
	key := makeKey(platform, tag)
	if isProfile {
		key += profileSuffix
	}
	data, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get %q from cache: %w", key, err)
	}
	var meta struct {
		CachedAt time.Time `json:"cachedAt"`
	}
	if err := json.Unmarshal(data, &meta); err != nil || time.Since(meta.CachedAt) > maxRetention {
		return nil
	}
	if err := c.client.Expire(ctx, key, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to extend %q: %w", key, err)
	}
	return nil
}

// IterKeys walks all cached player keys using SCAN and invokes fn for each one.
// SCAN is used instead of KEYS because KEYS is O(N) and blocks the whole server.
// Returning false from fn stops the iteration early.
func (c *RedisCache) IterKeys(ctx context.Context, fn func(key string) bool) error {
	var cursor uint64
	seen := 0

	for {
		keys, next, err := c.client.Scan(ctx, cursor, keyPrefix+"*", scanBatch).Result()
		if err != nil {
			return fmt.Errorf("failed to scan keys: %w", err)
		}
		for _, key := range keys {
			if !fn(key) {
				return nil
			}
			seen++
			if seen >= maxScanKeys {
				slog.Warn("stopping key scan at limit", "limit", maxScanKeys)
				return nil
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}

// CountKeys returns how many cached player entries exist.
func (c *RedisCache) CountKeys(ctx context.Context) (int, error) {
	count := 0
	err := c.IterKeys(ctx, func(string) bool {
		count++
		return true
	})
	return count, err
}

// Keys collects up to limit cached player keys. It exists for the admin endpoint,
// which must not stream an unbounded list back to the client.
func (c *RedisCache) Keys(ctx context.Context, limit int) ([]string, error) {
	keys := make([]string, 0, min(limit, 256))
	err := c.IterKeys(ctx, func(key string) bool {
		keys = append(keys, key)
		return len(keys) < limit
	})
	return keys, err
}

// Delete removes a cached entry and its profile variant.
func (c *RedisCache) Delete(ctx context.Context, platform, tag string) error {
	key := makeKey(platform, tag)
	if err := c.client.Del(ctx, key, key+profileSuffix).Err(); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}
	return nil
}

// FlushAll clears the entire cache database.
func (c *RedisCache) FlushAll(ctx context.Context) error {
	if err := c.client.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("failed to flush cache: %w", err)
	}
	return nil
}

// Close closes the Redis connection.
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// Ping checks if Redis is still reachable.
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
