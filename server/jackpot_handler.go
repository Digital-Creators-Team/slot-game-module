package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/game"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/jackpot"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

// SSE event types
const (
	EventTypeConnected = "connected"
	EventTypeUpdated   = "updated"
	EventTypeHeartbeat = "heartbeat"
)

// JackpotHandler bridges jackpot.Service to HTTP routes (SSE + WebSocket + update intake).
// It is transport-agnostic: other projects can reuse pkg/jackpot and wire their own handlers.
type JackpotHandler struct {
	svc             *jackpot.Service
	app             *App // Reference to App to access game module
	logger          zerolog.Logger
	heartbeatPeriod time.Duration
	upgrader        websocket.Upgrader
}

// NewJackpotHandler creates a jackpot handler.
func NewJackpotHandler(app *App, svc *jackpot.Service) *JackpotHandler {
	return &JackpotHandler{
		svc:             svc,
		app:             app,
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
// Route: GET /api/games/{game_code}/jackpot/updates?bet_multiplier=1.5
// Query params:
//   - bet_multiplier (required): Bet multiplier to filter pools (e.g., 1.5, 2.0, 5.0)
func (h *JackpotHandler) StreamUpdates(c *gin.Context) {
	// Get bet multiplier from query param
	betMultiplierStr := c.Query("bet_multiplier")
	if betMultiplierStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "bet_multiplier query parameter is required",
		})
		return
	}

	var betMultiplier float32
	if _, err := fmt.Sscanf(betMultiplierStr, "%f", &betMultiplier); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid bet_multiplier format",
		})
		return
	}

	// Get game module to retrieve pool IDs for this bet multiplier
	gameModule := h.app.GetGame()
	if gameModule == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "game module not registered",
		})
		return
	}

	// Get pool IDs for this bet multiplier using game module's GetPoolID
	var targetPoolIDs []string
	if handler, ok := gameModule.(game.JackpotHandler); ok {
		gameCode := gameModule.GetGameCode()
		poolIDs, err := handler.GetPoolID(c.Request.Context(), gameCode, betMultiplier)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to get pool IDs")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to get pool IDs",
			})
			return
		}
		targetPoolIDs = poolIDs
	} else {
		// Fallback: if game doesn't implement JackpotHandler, use all pools
		h.logger.Warn().Msg("Game module does not implement JackpotHandler, streaming all pools")
	}

	// Setup SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.WriteHeader(http.StatusOK)

	ctx := c.Request.Context()
	updates, cancel := h.svc.Listen(ctx)
	defer cancel()

	// Helper function to check if pool ID matches target pools
	isTargetPool := func(poolID string) bool {
		if len(targetPoolIDs) == 0 {
			return true // No filter, stream all
		}
		for _, targetID := range targetPoolIDs {
			if poolID == targetID {
				return true
			}
		}
		return false
	}

	// Send connected event
	_ = h.sendSSEEvent(c, &JackpotSSEResponse{
		Type:      EventTypeConnected,
		Timestamp: time.Now().Unix(),
	})

	// Send initial data with EventTypeUpdated for pools matching bet multiplier
	if currentPools, err := h.svc.GetCurrentPools(ctx); err == nil {
		for _, pool := range currentPools {
			if isTargetPool(pool.PoolID) {
				_ = h.sendSSEEvent(c, &JackpotSSEResponse{
					Type:      EventTypeUpdated,
					PoolID:    pool.PoolID,
					Amount:    pool.Amount.InexactFloat64(),
					Timestamp: pool.Timestamp.Unix(),
				})
			}
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
			// Only send updates for pools matching bet multiplier
			if isTargetPool(update.PoolID) {
				_ = h.sendSSEEvent(c, &JackpotSSEResponse{
					Type:      EventTypeUpdated,
					PoolID:    update.PoolID,
					Amount:    update.Amount.InexactFloat64(),
					Timestamp: update.Timestamp.Unix(),
				})
			}
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
// Route: GET /api/games/{game_code}/jackpot/updates/ws?bet_multiplier=1.5
// Query params:
//   - bet_multiplier (required): Bet multiplier to filter pools (e.g., 1.5, 2.0, 5.0)
func (h *JackpotHandler) StreamUpdatesWebSocket(c *gin.Context) {
	// Get bet multiplier from query param
	betMultiplierStr := c.Query("bet_multiplier")
	if betMultiplierStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "bet_multiplier query parameter is required",
		})
		return
	}

	var betMultiplier float32
	if _, err := fmt.Sscanf(betMultiplierStr, "%f", &betMultiplier); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid bet_multiplier format",
		})
		return
	}

	// Get game module to retrieve pool IDs for this bet multiplier
	gameModule := h.app.GetGame()
	if gameModule == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "game module not registered",
		})
		return
	}

	// Get pool IDs for this bet multiplier using game module's GetPoolID
	var targetPoolIDs []string
	if handler, ok := gameModule.(game.JackpotHandler); ok {
		gameCode := gameModule.GetGameCode()
		poolIDs, err := handler.GetPoolID(c.Request.Context(), gameCode, betMultiplier)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to get pool IDs")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to get pool IDs",
			})
			return
		}
		targetPoolIDs = poolIDs
	} else {
		// Fallback: if game doesn't implement JackpotHandler, use all pools
		h.logger.Warn().Msg("Game module does not implement JackpotHandler, streaming all pools")
	}

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

	// Helper function to check if pool ID matches target pools
	isTargetPool := func(poolID string) bool {
		if len(targetPoolIDs) == 0 {
			return true // No filter, stream all
		}
		for _, targetID := range targetPoolIDs {
			if poolID == targetID {
				return true
			}
		}
		return false
	}

	// Send connected event
	_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
		Type:      EventTypeConnected,
		Timestamp: time.Now().Unix(),
	})

	// Send initial data with EventTypeUpdated for pools matching bet multiplier
	if currentPools, err := h.svc.GetCurrentPools(ctx); err == nil {
		for _, pool := range currentPools {
			if isTargetPool(pool.PoolID) {
				_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
					Type:      EventTypeUpdated,
					PoolID:    pool.PoolID,
					Amount:    pool.Amount.InexactFloat64(),
					Timestamp: pool.Timestamp.Unix(),
				})
			}
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
				// Only log unexpected close errors (not normal closures)
				// CloseNormalClosure (1000) = normal closure (e.g., page closing)
				// CloseGoingAway (1001) = server going down or browser navigating away
				// CloseAbnormalClosure (1006) = connection lost without close frame
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
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
			// Only send updates for pools matching bet multiplier
			if isTargetPool(update.PoolID) {
				_ = h.sendWebSocketMessage(conn, &JackpotSSEResponse{
					Type:      EventTypeUpdated,
					PoolID:    update.PoolID,
					Amount:    update.Amount.InexactFloat64(),
					Timestamp: update.Timestamp.Unix(),
				})
			}
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
