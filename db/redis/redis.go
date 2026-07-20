package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/go-redis/redis/v8"
)

// Client provides Redis operations with connection pooling
type Client struct {
	client *redis.Client
}

// New creates a new Redis client
func New(cfg config.RedisConfig) (*Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.GetAddr(),
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Client{
		client: client,
	}, nil
}

// Get retrieves a value from Redis by key
func (r *Client) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("key not found: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("failed to get key %s: %w", key, err)
	}
	return val, nil
}

// GetJSON retrieves and unmarshals JSON value from Redis
func (r *Client) GetJSON(ctx context.Context, key string, dest interface{}) error {
	val, err := r.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

// Set stores a value in Redis with optional expiration
func (r *Client) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	err := r.client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}
	return nil
}

// SetJSON marshals and stores a value as JSON in Redis
func (r *Client) SetJSON(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	return r.Set(ctx, key, data, expiration)
}

// Delete removes a key from Redis
func (r *Client) Delete(ctx context.Context, key string) error {
	err := r.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}
	return nil
}

// Exists checks if a key exists in Redis
func (r *Client) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check existence of key %s: %w", key, err)
	}
	return count > 0, nil
}

// SetNX sets a key only if it doesn't exist (for distributed locking)
func (r *Client) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) (bool, error) {
	ok, err := r.client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return false, fmt.Errorf("failed to setnx key %s: %w", key, err)
	}
	return ok, nil
}

// Incr increments a key
func (r *Client) Incr(ctx context.Context, key string) (int64, error) {
	val, err := r.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to incr key %s: %w", key, err)
	}
	return val, nil
}

// IncrBy increments a key by a specific amount
func (r *Client) IncrBy(ctx context.Context, key string, value int64) (int64, error) {
	val, err := r.client.IncrBy(ctx, key, value).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to incrby key %s: %w", key, err)
	}
	return val, nil
}

// IncrByFloat increments a key by a float value
func (r *Client) IncrByFloat(ctx context.Context, key string, value float64) (float64, error) {
	val, err := r.client.IncrByFloat(ctx, key, value).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to incrbyfloat key %s: %w", key, err)
	}
	return val, nil
}

// Expire sets a timeout on key
func (r *Client) Expire(ctx context.Context, key string, expiration time.Duration) error {
	err := r.client.Expire(ctx, key, expiration).Err()
	if err != nil {
		return fmt.Errorf("failed to expire key %s: %w", key, err)
	}
	return nil
}

// TTL returns the remaining time to live of a key
func (r *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get ttl for key %s: %w", key, err)
	}
	return ttl, nil
}

// HSet sets a hash field
func (r *Client) HSet(ctx context.Context, key, field string, value interface{}) error {
	err := r.client.HSet(ctx, key, field, value).Err()
	if err != nil {
		return fmt.Errorf("failed to hset %s.%s: %w", key, field, err)
	}
	return nil
}

// HGet gets a hash field
func (r *Client) HGet(ctx context.Context, key, field string) (string, error) {
	val, err := r.client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("field not found: %s.%s", key, field)
	}
	if err != nil {
		return "", fmt.Errorf("failed to hget %s.%s: %w", key, field, err)
	}
	return val, nil
}

// HGetAll gets all fields in a hash
func (r *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	val, err := r.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to hgetall %s: %w", key, err)
	}
	return val, nil
}

// Close closes the Redis connection
func (r *Client) Close() error {
	return r.client.Close()
}

// GetClient returns the underlying Redis client for advanced operations
func (r *Client) GetClient() *redis.Client {
	return r.client
}

// Ping checks Redis connection
func (r *Client) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
