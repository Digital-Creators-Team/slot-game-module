package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	dbredis "github.com/Digital-Creators-Team/slot-game-module/db/redis"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type WSConnManager struct {
	logger zerolog.Logger
	nodeID string

	mu    sync.RWMutex
	conns map[string]*WSConn

	redisMu sync.RWMutex
	redis   *dbredis.Client

	subOnce sync.Once
	subErr  error
	pubsub  *redis.PubSub

	closeOnce sync.Once
	closeCh   chan struct{}
}

func NewWSConnManager(logger zerolog.Logger) *WSConnManager {
	return &WSConnManager{
		logger:  logger.With().Str("component", "ws-conn-manager").Logger(),
		nodeID:  uuid.NewString(),
		conns:   make(map[string]*WSConn),
		closeCh: make(chan struct{}),
	}
}

func (m *WSConnManager) SetRedisClient(client *dbredis.Client) {
	m.redisMu.Lock()
	m.redis = client
	m.redisMu.Unlock()

	m.subOnce.Do(func() {
		m.subErr = m.startSubscriber()
	})
}

func (m *WSConnManager) HasRedis() bool {
	m.redisMu.RLock()
	defer m.redisMu.RUnlock()
	return m.redis != nil
}

func (m *WSConnManager) Register(conn *WSConn) {
	m.mu.Lock()
	m.conns[conn.ID] = conn
	m.mu.Unlock()
}

func (m *WSConnManager) Unregister(connID string) {
	m.mu.Lock()
	delete(m.conns, connID)
	m.mu.Unlock()
}

func (m *WSConnManager) Get(connID string) (*WSConn, bool) {
	m.mu.RLock()
	conn, ok := m.conns[connID]
	m.mu.RUnlock()
	return conn, ok
}

func (m *WSConnManager) Close() {
	m.closeOnce.Do(func() {
		close(m.closeCh)
		if m.pubsub != nil {
			_ = m.pubsub.Close()
		}
	})
}

func (m *WSConnManager) startSubscriber() error {
	m.redisMu.RLock()
	r := m.redis
	m.redisMu.RUnlock()
	if r == nil {
		return fmt.Errorf("redis client is nil")
	}

	m.pubsub = r.GetClient().Subscribe(context.Background(), "presence:kick")
	ch := m.pubsub.Channel()

	go func() {
		for {
			select {
			case <-m.closeCh:
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var kick WSKickMessage
				if err := json.Unmarshal([]byte(msg.Payload), &kick); err != nil {
					m.logger.Warn().Err(err).Msg("failed to unmarshal kick message")
					continue
				}
				conn, ok := m.Get(kick.ConnID)
				if !ok {
					continue
				}
				conn.CloseWithReason(kick.Reason)
			}
		}
	}()

	return nil
}

func (m *WSConnManager) AcquireSession(ctx context.Context, tenantID string, userID string, connID string, ttl time.Duration) (string, error) {
	m.redisMu.RLock()
	r := m.redis
	m.redisMu.RUnlock()
	if r == nil {
		return "", fmt.Errorf("redis client is nil")
	}

	key := fmt.Sprintf("presence:ws:%s:%s", tenantID, userID)
	script := redis.NewScript(`
local old = redis.call('GET', KEYS[1])
redis.call('SET', KEYS[1], ARGV[1], 'EX', tonumber(ARGV[2]))
return old
`)
	res, err := script.Run(ctx, r.GetClient(), []string{key}, connID, int(ttl.Seconds())).Result()
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	old, _ := res.(string)
	return old, nil
}

func (m *WSConnManager) TouchSession(ctx context.Context, tenantID string, userID string, connID string, ttl time.Duration) error {
	m.redisMu.RLock()
	r := m.redis
	m.redisMu.RUnlock()
	if r == nil {
		return fmt.Errorf("redis client is nil")
	}

	key := fmt.Sprintf("presence:ws:%s:%s", tenantID, userID)
	script := redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if cur == ARGV[1] then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
  return 1
end
return 0
`)
	_, err := script.Run(ctx, r.GetClient(), []string{key}, connID, int(ttl.Seconds())).Result()
	return err
}

func (m *WSConnManager) ReleaseSession(ctx context.Context, tenantID string, userID string, connID string) error {
	m.redisMu.RLock()
	r := m.redis
	m.redisMu.RUnlock()
	if r == nil {
		return fmt.Errorf("redis client is nil")
	}

	key := fmt.Sprintf("presence:ws:%s:%s", tenantID, userID)
	script := redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if cur == ARGV[1] then
  redis.call('DEL', KEYS[1])
  return 1
end
return 0
`)
	_, err := script.Run(ctx, r.GetClient(), []string{key}, connID).Result()
	return err
}

func (m *WSConnManager) PublishKick(ctx context.Context, msg WSKickMessage) error {
	m.redisMu.RLock()
	r := m.redis
	m.redisMu.RUnlock()
	if r == nil {
		return fmt.Errorf("redis client is nil")
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return r.GetClient().Publish(ctx, "presence:kick", payload).Err()
}

