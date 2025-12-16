package server

import (
	"encoding/json"
	"net/http"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/jackpot"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// SSE event types
const (
	EventTypeConnected = "connected"
	EventTypeInitial   = "initial"
	EventTypeUpdated   = "updated"
	EventTypeHeartbeat = "heartbeat"
)

// JackpotHandler bridges jackpot.Service to HTTP routes (SSE + WebSocket + update intake).
// It is transport-agnostic: other projects can reuse pkg/jackpot and wire their own handlers.
type JackpotHandler struct {
	svc             *jackpot.Service
	logger          zerolog.Logger
	heartbeatPeriod time.Duration
	upgrader        websocket.Upgrader
}

// NewJackpotHandler creates a jackpot handler.
func NewJackpotHandler(app *App, svc *jackpot.Service) *JackpotHandler {
	return &JackpotHandler{
		svc:             svc,
		logger:          app.logger.With().Str("handler", "jackpot").Logger(),
		heartbeatPeriod: 60 * time.Second,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now (can be restricted in production)
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// JackpotSSEResponse represents SSE payload.
type JackpotSSEResponse struct {
	PoolID    string  `json:"pool_id,omitempty"`
	Amount    float64 `json:"amount,omitempty"`
	Type      string  `json:"type"`
	Timestamp int64   `json:"timestamp"`
}

// JackpotUpdateRequest is used by Kafka/any publisher to buffer updates.
type JackpotUpdateRequest struct {
	PoolID string  `json:"poolId" binding:"required"`
	Amount float64 `json:"amount" binding:"required"`
}

// StreamUpdates opens SSE and streams buffered jackpot updates.
// Route: GET /api/games/{game_code}/jackpot/updates
func (h *JackpotHandler) StreamUpdates(c *gin.Context) {
	// Setup SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.WriteHeader(http.StatusOK)

	ctx := c.Request.Context()
	updates, cancel := h.svc.Listen(ctx)
	defer cancel()

	// Send connected event
	_ = h.sendSSEEvent(c, &JackpotSSEResponse{
		Type:      EventTypeConnected,
		Timestamp: time.Now().Unix(),
	})

	// Send initial data for all pools
	currentPools, err := h.svc.GetCurrentPools(ctx)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get current pools for initial data")
	} else {
		for _, pool := range currentPools {
			_ = h.sendSSEEvent(c, &JackpotSSEResponse{
				Type:      EventTypeInitial,
				PoolID:    pool.PoolID,
				Amount:    pool.Amount.InexactFloat64(),
				Timestamp: pool.Timestamp.Unix(),
			})
		}
	}

	heartbeat := time.NewTicker(h.heartbeatPeriod)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			_ = h.sendSSEEvent(c, &JackpotSSEResponse{
				Type:      EventTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			})
		case update, ok := <-updates:
			if !ok {
				return
			}
			_ = h.sendSSEEvent(c, &JackpotSSEResponse{
				Type:      EventTypeUpdated,
				PoolID:    update.PoolID,
				Amount:    update.Amount.InexactFloat64(),
				Timestamp: update.Timestamp.Unix(),
			})
		}
	}
}

// sendSSEEvent encodes + flushes SSE data.
func (h *JackpotHandler) sendSSEEvent(c *gin.Context, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := c.Writer.Write([]byte("data: " + string(payload) + "\n\n")); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// StreamUpdatesWebSocket opens WebSocket connection and streams buffered jackpot updates.
// Route: GET /api/games/{game_code}/jackpot/updates/ws
func (h *JackpotHandler) StreamUpdatesWebSocket(c *gin.Context) {
	// Upgrade HTTP connection to WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to upgrade connection to WebSocket")
		return
	}
	defer conn.Close() //nolint:errcheck

	ctx := c.Request.Context()
	updates, cancel := h.svc.Listen(ctx)
	defer cancel()

	// Send connected event
	_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
		Type:      EventTypeConnected,
		Timestamp: time.Now().Unix(),
	})

	// Send initial data for all pools
	currentPools, err := h.svc.GetCurrentPools(ctx)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get current pools for initial data")
	} else {
		for _, pool := range currentPools {
			_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
				Type:      EventTypeInitial,
				PoolID:    pool.PoolID,
				Amount:    pool.Amount.InexactFloat64(),
				Timestamp: pool.Timestamp.Unix(),
			})
		}
	}

	heartbeat := time.NewTicker(h.heartbeatPeriod)
	defer heartbeat.Stop()

	// Channel to handle ping/pong
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.Debug().Err(err).Msg("WebSocket read error")
				}
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-heartbeat.C:
			_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
				Type:      EventTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			})
		case update, ok := <-updates:
			if !ok {
				return
			}
			_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
				Type:      EventTypeUpdated,
				PoolID:    update.PoolID,
				Amount:    update.Amount.InexactFloat64(),
				Timestamp: update.Timestamp.Unix(),
			})
		}
	}
}

// sendWebSocketMessage encodes and sends WebSocket message.
func (h *JackpotHandler) sendWebSocketMessage(conn *websocket.Conn, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}
