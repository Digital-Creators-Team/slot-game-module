package cache

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrExpired  = errors.New("expired")
)

type Item[T any] struct {
	Value     T
	ExpiresAt time.Time
}

type Cache[T any] interface {
	Set(ctx context.Context, key string, value T, ttl time.Duration) error
	MSet(ctx context.Context, valueMap map[string]T, ttl time.Duration) error
	Get(ctx context.Context, key string) (T, error)
	MGet(ctx context.Context, keys []string) ([]T, error)
	Delete(ctx context.Context, key string) error
}
