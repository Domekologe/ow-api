package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/Domekologe/ow-api/ovrstat"
	"github.com/redis/go-redis/v9"
)

// RedisCache wraps the Redis client for caching player stats
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
	ctx    context.Context
}

// NewRedisCache creates a new Redis cache client
func NewRedisCache(host string, port int, password string, db int, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		Password: password,
		DB:       db,
	})

	ctx := context.Background()

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Printf("Connected to Redis at %s:%d", host, port)

	return &RedisCache{
		client: client,
		ttl:    ttl,
		ctx:    ctx,
	}, nil
}

// makeKey generates a cache key for a player
func makeKey(platform, tag string) string {
	return fmt.Sprintf("ow:stats:%s:%s", platform, tag)
}

// Get retrieves cached player stats
func (c *RedisCache) Get(platform, tag string) (*ovrstat.PlayerStats, error) {
	key := makeKey(platform, tag)
	data, err := c.client.Get(c.ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("failed to get from cache: %w", err)
	}

	var stats ovrstat.PlayerStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached data: %w", err)
	}

	return &stats, nil
}

// Set stores player stats in cache
func (c *RedisCache) Set(platform, tag string, stats *ovrstat.PlayerStats) error {
	key := makeKey(platform, tag)
	data, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	if err := c.client.Set(c.ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to set cache: %w", err)
	}

	return nil
}

// GetProfile retrieves cached player profile stats
func (c *RedisCache) GetProfile(platform, tag string) (*ovrstat.PlayerStatsProfile, error) {
	key := makeKey(platform, tag) + ":profile"
	data, err := c.client.Get(c.ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // Cache miss
		}
		return nil, fmt.Errorf("failed to get profile from cache: %w", err)
	}

	var stats ovrstat.PlayerStatsProfile
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached profile data: %w", err)
	}

	return &stats, nil
}

// SetProfile stores player profile stats in cache
func (c *RedisCache) SetProfile(platform, tag string, stats *ovrstat.PlayerStatsProfile) error {
	key := makeKey(platform, tag) + ":profile"
	data, err := json.Marshal(stats)
	if err != nil {
		return fmt.Errorf("failed to marshal profile stats: %w", err)
	}

	if err := c.client.Set(c.ctx, key, data, c.ttl).Err(); err != nil {
		return fmt.Errorf("failed to set profile cache: %w", err)
	}

	return nil
}

// GetKeys returns all cached player keys matching the pattern
func (c *RedisCache) GetKeys(pattern string) ([]string, error) {
	keys, err := c.client.Keys(c.ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get keys: %w", err)
	}
	return keys, nil
}

// Delete removes a cached entry
func (c *RedisCache) Delete(platform, tag string) error {
	key := makeKey(platform, tag)
	if err := c.client.Del(c.ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to delete from cache: %w", err)
	}
	return nil
}

// FlushAll clears the entire cache
func (c *RedisCache) FlushAll() error {
	if err := c.client.FlushDB(c.ctx).Err(); err != nil {
		return fmt.Errorf("failed to flush cache: %w", err)
	}
	return nil
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// Ping checks if Redis is still connected
func (c *RedisCache) Ping() error {
	return c.client.Ping(c.ctx).Err()
}
