package redisutil

import (
	"strings"

	"github.com/redis/go-redis/v9"
	"task-queue-system/internal/config"
)

// NewClient returns a redis.UniversalClient (which could be a single node, 
// Sentinel, or Cluster client) based on the provided configuration.
func NewClient(cfg *config.Config) redis.UniversalClient {
	var addrs []string
	
	if cfg.RedisClusterAddrs != "" {
		addrs = strings.Split(cfg.RedisClusterAddrs, ",")
	} else if cfg.RedisSentinelAddrs != "" {
		addrs = strings.Split(cfg.RedisSentinelAddrs, ",")
	} else {
		addrs = []string{cfg.RedisHost}
	}

	opts := &redis.UniversalOptions{
		Addrs:    addrs,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
		PoolSize: 128,
	}
	
	if cfg.RedisSentinelMaster != "" {
		opts.MasterName = cfg.RedisSentinelMaster
	}

	return redis.NewUniversalClient(opts)
}
