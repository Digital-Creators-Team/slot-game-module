package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/jackpot"
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
		heartbeatPeriod: 30 * time.Second,
		upgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

type Response struct {
	Type      string                `json:"type"`
	Timestamp int64                 `json:"timestamp"`
	Pools     map[string]PoolUpdate `json:"pools,omitempty"`
}

type PoolUpdate struct {
	Amount    float64 `json:"amount"`
	Timestamp int64   `json:"timestamp"`
}

func extractPoolType(poolID string) string {
	if idx := strings.LastIndex(poolID, ":"); idx >= 0 {
		return poolID[idx+1:]
	}
	parts := strings.Split(poolID, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return poolID
}

func validatePoolIDMatch(poolID string, betMultiplier float32) bool {
	if strings.Contains(poolID, ":") {
		return true
	}
	betMultiplierStr := fmt.Sprintf("%g", betMultiplier)
	betMultiplierStrAlt := fmt.Sprintf("%.1f", betMultiplier)
	containsBetMultiplier := strings.Contains(poolID, betMultiplierStr) || strings.Contains(poolID, betMultiplierStrAlt)
	if !containsBetMultiplier {
		return true
	}
	return containsBetMultiplier
}

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
	defer conn.Close() //nolint:errcheck

	writeDeadline := 10 * time.Second
	conn.SetWriteDeadline(time.Now().Add(writeDeadline)) //nolint:errcheck

	// Set ping handler to automatically respond to WebSocket PingMessage control frames
	// This handles standard WebSocket ping/pong protocol
	conn.SetPingHandler(func(appData string) error {
		deadline := time.Now().Add(5 * time.Second)
		return conn.WriteControl(websocket.PongMessage, []byte(appData), deadline)
	})

	done := make(chan struct{})

	// Handle incoming messages from client
	go func() {
		defer close(done)
		for {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			messageType, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
						h.logger.Warn().Err(err).Msg("WebSocket connection closed unexpectedly (EOF)")
					} else {
						h.logger.Warn().Err(err).Msg("WebSocket connection closed unexpectedly")
					}
				} else {
					h.logger.Debug().Err(err).Msg("WebSocket closed normally")
				}
				return
			}

			// Handle client messages (ping/pong and other message types)
			if h.handleClientMessage(conn, messageType, message) {
				// If message was handled, continue to next message
				continue
			}
		}
	}()

	sender := &wsSender{
		conn:          conn,
		done:          done,
		logger:        h.logger,
		writeDeadline: writeDeadline,
	}
	h.streamUpdates(config, sender)
}

// handleClientMessage handles incoming messages from client using switch case.
// Returns true if message was handled, false otherwise.
// This allows easy extension for other message types in the future.
func (h *JackpotHandler) handleClientMessage(conn *websocket.Conn, messageType int, message []byte) bool {
	// Only handle text and binary messages
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return false
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(message, &msg); err != nil {
		return false
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		return false
	}

	// Switch case for different message types
	switch msgType {
	case "ping":
		return h.handlePingPong(conn, messageType)
	// Add more message types here in the future:
	// case "subscribe":
	//     return h.handleSubscribe(conn, messageType, msg)
	// case "unsubscribe":
	//     return h.handleUnsubscribe(conn, messageType, msg)
	default:
		// Unknown message type, not handled
		return false
	}
}

// handlePingPong handles ping messages from client (both text and binary).
// Returns true if a ping message was handled, false otherwise.
// Client sends: {"type":"ping"} (text or binary)
// Server responds: {"type":"pong", "timestamp": <unix_timestamp>} (same message type as received)
func (h *JackpotHandler) handlePingPong(conn *websocket.Conn, messageType int) bool {
	// Respond with pong including timestamp
	pongResponse := map[string]interface{}{
		"type":      "pong",
		"timestamp": time.Now().Unix(),
	}
	pongPayload, err := json.Marshal(pongResponse)
	if err != nil {
		h.logger.Debug().Err(err).Msg("Failed to marshal pong response")
		return true
	}

	deadline := time.Now().Add(5 * time.Second)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		h.logger.Debug().Err(err).Msg("Failed to set write deadline for pong")
		return true
	}

	// Send pong with the same message type as received (text or binary)
	if err := conn.WriteMessage(messageType, pongPayload); err != nil {
		h.logger.Debug().Err(err).Msg("Failed to send pong")
		return true
	}

	return true
}

// prepareStreamConfig extracts and validates stream configuration.
func (h *JackpotHandler) prepareStreamConfig(c *gin.Context) (*streamConfig, error) {
	betMultiplierStr := c.Query("bet_multiplier")
	if betMultiplierStr == "" {
		ErrorWithMessage(c, http.StatusBadRequest, "bet_multiplier query parameter is required")
		return nil, fmt.Errorf("missing bet_multiplier")
	}

	var betMultiplier float32
	if _, err := fmt.Sscanf(betMultiplierStr, "%f", &betMultiplier); err != nil {
		ErrorWithMessage(c, http.StatusBadRequest, "invalid bet_multiplier format")
		return nil, err
	}

	gameModule := h.app.GetGame()
	if gameModule == nil {
		ErrorWithMessage(c, http.StatusInternalServerError, "game module not registered")
		return nil, fmt.Errorf("game module not registered")
	}

	var targetPoolIDs []string
	gameCode := gameModule.GetGameCode()
	if handler, ok := gameModule.(game.JackpotHandler); ok {
		poolIDs, err := handler.GetPoolID(c.Request.Context(), gameCode, betMultiplier)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to get pool IDs")
			ErrorWithMessage(c, http.StatusInternalServerError, "failed to get pool IDs")
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
	if err := sender.Send(&Response{
		Type:      EventTypeConnected,
		Timestamp: time.Now().Unix(),
	}); err != nil {
		h.logger.Warn().Err(err).Msg("Failed to send connected event, stopping stream")
		return
	}

	// Send initial pool data
	h.sendInitialPools(config, sender)

	// Setup heartbeat and update loop
	heartbeat := time.NewTicker(h.heartbeatPeriod)
	defer heartbeat.Stop()

	// Batch updates that arrive close together (from same spin flush)
	// Use a small window to collect updates from the same flush
	batchWindow := 5 * time.Millisecond
	batchTimer := time.NewTimer(batchWindow)
	batchTimer.Stop()
	pendingPools := make(map[string]PoolUpdate)

	flushBatch := func() bool {
		if len(pendingPools) == 0 {
			return true
		}
		if err := sender.Send(&Response{
			Type:      EventTypeUpdated,
			Timestamp: time.Now().Unix(),
			Pools:     pendingPools,
		}); err != nil {
			h.logger.Warn().
				Err(err).
				Int("pool_count", len(pendingPools)).
				Msg("Failed to send batch update, stopping stream")
			return false
		}
		pendingPools = make(map[string]PoolUpdate)
		return true
	}

	// Check if sender has a done channel (for WebSocket)
	var doneChan <-chan struct{}
	if wsSender, ok := sender.(*wsSender); ok {
		doneChan = wsSender.done
	}

	for {
		select {
		case <-config.ctx.Done():
			flushBatch()
			return
		case <-doneChan:
			// WebSocket connection closed
			h.logger.Debug().Msg("WebSocket connection closed, stopping stream")
			flushBatch()
			return
		case <-heartbeat.C:
			if !flushBatch() {
				return
			}
			if err := sender.Send(&Response{
				Type:      EventTypeHeartbeat,
				Timestamp: time.Now().Unix(),
			}); err != nil {
				h.logger.Warn().Err(err).Msg("Failed to send heartbeat, stopping stream")
				return
			}
		case <-batchTimer.C:
			if !flushBatch() {
				return
			}
			batchTimer.Stop()
		case update, ok := <-updates:
			if !ok {
				flushBatch()
				return
			}
			if !config.isTargetPool(update.PoolID) {
				continue
			}

			if !validatePoolIDMatch(update.PoolID, config.betMultiplier) {
				h.logger.Warn().
					Str("pool_id", update.PoolID).
					Float32("expected_bet_multiplier", config.betMultiplier).
					Msg("Pool ID does not match expected bet multiplier, skipping")
				continue
			}

			poolType := extractPoolType(update.PoolID)
			pendingPools[poolType] = PoolUpdate{
				Amount:    update.Amount.InexactFloat64(),
				Timestamp: update.Timestamp.Unix(),
			}

			// Try to collect more updates from the same flush immediately
			// by checking if there are more updates available without blocking
			collected := false
			for {
				select {
				case nextUpdate, nextOk := <-updates:
					if !nextOk {
						if !flushBatch() {
							return
						}
						return
					}
					if config.isTargetPool(nextUpdate.PoolID) {
						if !validatePoolIDMatch(nextUpdate.PoolID, config.betMultiplier) {
							h.logger.Warn().
								Str("pool_id", nextUpdate.PoolID).
								Float32("expected_bet_multiplier", config.betMultiplier).
								Msg("Pool ID does not match expected bet multiplier, skipping")
							continue
						}
						nextPoolType := extractPoolType(nextUpdate.PoolID)
						pendingPools[nextPoolType] = PoolUpdate{
							Amount:    nextUpdate.Amount.InexactFloat64(),
							Timestamp: nextUpdate.Timestamp.Unix(),
						}
						collected = true
					}
				default:
					// No more updates immediately available
					goto doneCollecting
				}
			}
		doneCollecting:
			// If we collected multiple updates, send immediately
			// Otherwise, start timer to wait for more
			if collected {
				if !flushBatch() {
					return
				}
			} else {
				// Start/reset timer to batch updates from same spin
				if !batchTimer.Stop() {
					select {
					case <-batchTimer.C:
					default:
					}
				}
				batchTimer.Reset(batchWindow)
			}
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

	pools := make(map[string]PoolUpdate)
	for _, pool := range currentPools {
		if config.isTargetPool(pool.PoolID) {
			poolType := extractPoolType(pool.PoolID)
			pools[poolType] = PoolUpdate{
				Amount:    pool.Amount.InexactFloat64(),
				Timestamp: pool.Timestamp.Unix(),
			}
		}
	}

	if len(pools) > 0 {
		if err := sender.Send(&Response{
			Type:      EventTypeUpdated,
			Timestamp: time.Now().Unix(),
			Pools:     pools,
		}); err != nil {
			h.logger.Warn().Err(err).Int("pool_count", len(pools)).Msg("Failed to send initial pools")
		}
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
	conn          *websocket.Conn
	done          <-chan struct{}
	logger        zerolog.Logger
	writeDeadline time.Duration
}

func (s *wsSender) Send(resp *Response) error {
	// Check if connection is already closed
	select {
	case <-s.done:
		s.logger.Debug().Str("event_type", resp.Type).Msg("Connection already closed, skipping send")
		return io.EOF
	default:
	}

	// Set write deadline before each write
	deadline := time.Now().Add(s.writeDeadline)
	if err := s.conn.SetWriteDeadline(deadline); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to set write deadline")
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		s.logger.Error().Err(err).Str("event_type", resp.Type).Msg("Failed to marshal response")
		return err
	}

	err = s.conn.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		// Log detailed error information
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			s.logger.Warn().
				Err(err).
				Str("event_type", resp.Type).
				Int("payload_size", len(payload)).
				Msg("WebSocket WriteMessage failed: connection closed (EOF)")
		} else if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			s.logger.Warn().
				Err(err).
				Str("event_type", resp.Type).
				Int("payload_size", len(payload)).
				Msg("WebSocket WriteMessage failed: unexpected close error")
		} else {
			s.logger.Warn().
				Err(err).
				Str("event_type", resp.Type).
				Int("payload_size", len(payload)).
				Msg("WebSocket WriteMessage failed")
		}
		return err
	}

	s.logger.Debug().
		Str("event_type", resp.Type).
		Int("payload_size", len(payload)).
		Msg("WebSocket message sent successfully")

	return nil
}
