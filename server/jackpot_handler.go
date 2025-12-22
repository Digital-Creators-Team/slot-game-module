package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/game"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/pkg/jackpot"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

const (
	EventTypeConnected = "connected"
	EventTypeUpdated   = "updated"
	EventTypeHeartbeat = "heartbeat"
)

// JackpotHandler bridges jackpot.Service to HTTP routes (SSE + WebSocket).
type JackpotHandler struct {
	svc             *jackpot.Service
	app             *App
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
			CheckOrigin: func(r *http.Request) bool { return true },
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// Response represents the payload sent to clients.
type Response struct {
	Type      string       `json:"type"`
	Timestamp int64        `json:"timestamp"`
	Pools     []PoolUpdate `json:"pools,omitempty"` // Batch updates
}

// PoolUpdate represents a single pool update.
type PoolUpdate struct {
	PoolID    string  `json:"pool_id"`
	Amount    float64 `json:"amount"`
	Timestamp int64   `json:"timestamp"`
}

// streamConfig holds configuration for streaming updates.
type streamConfig struct {
	betMultiplier float32
	targetPoolIDs []string
	isTargetPool  func(string) bool
	ctx           context.Context
}

// StreamUpdates opens SSE connection and streams jackpot updates.
// Route: GET /api/games/{game_code}/jackpot/updates?bet_multiplier=1.5
func (h *JackpotHandler) StreamUpdates(c *gin.Context) {
	config, err := h.prepareStreamConfig(c)
	if err != nil {
		return
	}

	// Setup SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
	c.Writer.WriteHeader(http.StatusOK)

	sender := &sseSender{writer: c.Writer}
	h.streamUpdates(config, sender)
}

// StreamUpdatesWebSocket opens WebSocket connection and streams jackpot updates.
// Route: GET /api/games/{game_code}/jackpot/updates/ws?bet_multiplier=1.5
func (h *JackpotHandler) StreamUpdatesWebSocket(c *gin.Context) {
	config, err := h.prepareStreamConfig(c)
	if err != nil {
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to upgrade to WebSocket")
		return
	}
	defer conn.Close()

	// Handle ping/pong
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				if !websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.Debug().Err(err).Msg("WebSocket closed")
				}
				return
			}
		}
	}()

	sender := &wsSender{conn: conn}
	h.streamUpdates(config, sender)
}

// prepareStreamConfig extracts and validates stream configuration.
func (h *JackpotHandler) prepareStreamConfig(c *gin.Context) (*streamConfig, error) {
	betMultiplierStr := c.Query("bet_multiplier")
	if betMultiplierStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bet_multiplier query parameter is required"})
		return nil, fmt.Errorf("missing bet_multiplier")
	}

	var betMultiplier float32
	if _, err := fmt.Sscanf(betMultiplierStr, "%f", &betMultiplier); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bet_multiplier format"})
		return nil, err
	}

	gameModule := h.app.GetGame()
	if gameModule == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "game module not registered"})
		return nil, fmt.Errorf("game module not registered")
	}

	var targetPoolIDs []string
	gameCode := gameModule.GetGameCode()
	if handler, ok := gameModule.(game.JackpotHandler); ok {
		poolIDs, err := handler.GetPoolID(c.Request.Context(), gameCode, betMultiplier)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to get pool IDs")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get pool IDs"})
			return nil, err
		}
		targetPoolIDs = poolIDs
	}

	isTargetPool := func(poolID string) bool {
		return len(targetPoolIDs) == 0 || lo.Contains(targetPoolIDs, poolID)
	}

	return &streamConfig{
		betMultiplier: betMultiplier,
		targetPoolIDs: targetPoolIDs,
		isTargetPool:  isTargetPool,
		ctx:           c.Request.Context(),
	}, nil
}

// streamUpdates handles the common streaming logic for both SSE and WebSocket.
func (h *JackpotHandler) streamUpdates(config *streamConfig, sender messageSender) {
	updates, cancel := h.svc.Listen(config.ctx)
	defer cancel()

	// Send connected event
	_ = sender.Send(&Response{
		Type:      EventTypeConnected,
		Timestamp: time.Now().Unix(),
	})

	// Send initial pool data
	h.sendInitialPools(config, sender)

	// Setup heartbeat and update loop
	heartbeat := time.NewTicker(h.heartbeatPeriod)
	defer heartbeat.Stop()

	for {
		select {
		case <-config.ctx.Done():
			return
		case <-heartbeat.C:
			_ = sender.Send(&Response{
				Type:      EventTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			})
		case update, ok := <-updates:
			if !ok {
				return
			}
			if !config.isTargetPool(update.PoolID) {
				continue
			}

			// Send update immediately (service already batches by spin_id)
			_ = sender.Send(&Response{
				Type:      EventTypeUpdated,
				Timestamp: update.Timestamp.Unix(),
				Pools: []PoolUpdate{{
					PoolID:    update.PoolID,
					Amount:    update.Amount.InexactFloat64(),
					Timestamp: update.Timestamp.Unix(),
				}},
			})
		}
	}
}

// sendInitialPools sends current pool values to the client.
func (h *JackpotHandler) sendInitialPools(config *streamConfig, sender messageSender) {
	gameModule := h.app.GetGame()
	var currentPools []jackpot.Update
	var err error

	if len(config.targetPoolIDs) > 0 {
		var initValueGetter func(poolID string) (decimal.Decimal, error)
		if handler, ok := gameModule.(game.JackpotHandler); ok {
			initValueGetter = func(poolID string) (decimal.Decimal, error) {
				return handler.GetInitialPoolValue(config.ctx, poolID, config.betMultiplier)
			}
		}
		currentPools, err = h.svc.GetPoolsByIDs(config.ctx, config.targetPoolIDs, initValueGetter)
	} else {
		currentPools, err = h.svc.GetCurrentPools(config.ctx)
	}

	if err != nil {
		h.logger.Error().Err(err).Msg("Failed to get current pools")
		return
	}

	// Filter and send pools
	pools := make([]PoolUpdate, 0, len(currentPools))
	for _, pool := range currentPools {
		if config.isTargetPool(pool.PoolID) {
			pools = append(pools, PoolUpdate{
				PoolID:    pool.PoolID,
				Amount:    pool.Amount.InexactFloat64(),
				Timestamp: pool.Timestamp.Unix(),
			})
		}
	}

	if len(pools) > 0 {
		_ = sender.Send(&Response{
			Type:      EventTypeUpdated,
			Timestamp: time.Now().Unix(),
			Pools:     pools,
		})
	}
}

// messageSender interface for sending messages (SSE or WebSocket).
type messageSender interface {
	Send(*Response) error
}

// sseSender sends messages via SSE.
type sseSender struct {
	writer http.ResponseWriter
}

func (s *sseSender) Send(resp *Response) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = s.writer.Write([]byte("data: " + string(payload) + "\n\n"))
	if err != nil {
		return err
	}
	s.writer.(http.Flusher).Flush()
	return nil
}

// wsSender sends messages via WebSocket.
type wsSender struct {
	conn *websocket.Conn
}

func (s *wsSender) Send(resp *Response) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, payload)
}
