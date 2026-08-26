package cache

import (
	"context"
	"sync"
	"time"
)

type ttlMap[T any] struct {
	mu    sync.RWMutex
	items map[string]Item[T]
}

func NewTTLMap[T any]() Cache[T] {
	return &ttlMap[T]{
		items: make(map[string]Item[T]),
	}
}

func (m *ttlMap[T]) Set(_ context.Context, key string, value T, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items[key] = Item[T]{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}

	return nil
}

func (m *ttlMap[T]) MSet(ctx context.Context, valueMap map[string]T, ttl time.Duration) error {
	for key, value := range valueMap {
		err := m.Set(ctx, key, value, ttl)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *ttlMap[T]) Get(_ context.Context, key string) (T, error) {
	m.mu.RLock()
	item, ok := m.items[key]
	m.mu.RUnlock()

	if !ok {
		var zero T
		return zero, ErrNotFound
	}

	if time.Now().After(item.ExpiresAt) {
		return item.Value, ErrExpired
	}

	return item.Value, nil
}

func (m *ttlMap[T]) MGet(ctx context.Context, keys []string) ([]T, error) {
	var items = make([]T, len(keys))
	for i, key := range keys {
		item, err := m.Get(ctx, key)
		if err != nil {
			return nil, err
		}

		items[i] = item
	}

	return items, nil
}

func (m *ttlMap[T]) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.items, key)

	return nil
}
