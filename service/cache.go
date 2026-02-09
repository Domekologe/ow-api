package service

import (
	"time"

	"github.com/Domekologe/ow-api/cache"
	"github.com/Domekologe/ow-api/ovrstat"
)

// RedisCache wraps the cache.RedisCache for service use
type RedisCache struct {
	*cache.RedisCache
}

// Get retrieves cached player stats (wrapper for compatibility)
func (r *RedisCache) Get(platform, tag string) (*ovrstat.PlayerStats, error) {
	return r.RedisCache.Get(platform, tag)
}

// Set stores player stats in cache (wrapper for compatibility)
func (r *RedisCache) Set(platform, tag string, stats *ovrstat.PlayerStats) error {
	return r.RedisCache.Set(platform, tag, stats)
}

// GetProfile retrieves cached player profile stats
func (r *RedisCache) GetProfile(platform, tag string) (*ovrstat.PlayerStatsProfile, error) {
	return r.RedisCache.GetProfile(platform, tag)
}

// SetProfile stores player profile stats in cache
func (r *RedisCache) SetProfile(platform, tag string, stats *ovrstat.PlayerStatsProfile) error {
	return r.RedisCache.SetProfile(platform, tag, stats)
}

// NewRedisCache creates a new Redis cache for the service
func NewRedisCache(host string, port int, password string, db int, ttl time.Duration) (*RedisCache, error) {
	c, err := cache.NewRedisCache(host, port, password, db, ttl)
	if err != nil {
		return nil, err
	}
	return &RedisCache{c}, nil
}
