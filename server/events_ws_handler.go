package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/auth"
	apperrors "github.com/Digital-Creators-Team/slot-game-module/errors"
	"github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/Digital-Creators-Team/slot-game-module/types"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

const wsTimeout = 5 * time.Second

type EventsWSHandler struct {
	app        *App
	logger     zerolog.Logger
	connMgr    *WSConnManager
	upgrader   websocket.Upgrader
	sessionTTL time.Duration
}

func NewEventsWSHandler(app *App, connMgr *WSConnManager) *EventsWSHandler {
	return &EventsWSHandler{
		app:     app,
		logger:  app.logger.With().Str("handler", "events-ws").Logger(),
		connMgr: connMgr,
		upgrader: websocket.Upgrader{
			CheckOrigin:     func(r *http.Request) bool { return true },
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		sessionTTL: 60 * time.Second,
	}
}

func (h *EventsWSHandler) buildWSBaseContext(reqCtx context.Context, claims *auth.Claims) context.Context {
	if reqCtx != nil {
		if mc := game.FromContext(reqCtx); mc != nil {
			return game.WithContext(context.Background(), mc)
		}
	}
	user := game.NewUser(claims.TenantID, claims.UserID, claims.Username, claims.CurrencyID)
	mc := game.NewModuleContext(user, h.app.logger, h.app.stateProvider, h.app.walletProvider, h.app.rewardProvider, h.app.logProvider)
	return game.WithContext(context.Background(), mc)
}

func (h *EventsWSHandler) timeoutReplyIfNeeded(ctx context.Context, err error) *wsReply {
	if err == nil && ctx != nil && ctx.Err() == context.DeadlineExceeded {
		return errReply(http.StatusRequestTimeout, apperrors.New(apperrors.ErrRequestTimeout, "request timeout"))
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || (ctx != nil && ctx.Err() == context.DeadlineExceeded) {
		return errReply(http.StatusRequestTimeout, apperrors.New(apperrors.ErrRequestTimeout, "request timeout"))
	}
	return nil
}

type WSConn struct {
	ID       string
	TenantID string
	UserID   string

	conn   *websocket.Conn
	logger zerolog.Logger
	mgr    *WSConnManager

	send chan []byte

	closeOnce sync.Once
	closed    chan struct{}

	jackpotMu     sync.Mutex
	jackpotCancel context.CancelFunc

	baseCtx context.Context
}

func (c *WSConn) Done() <-chan struct{} {
	return c.closed
}

func (c *WSConn) Context() context.Context {
	if c.baseCtx != nil {
		return c.baseCtx
	}
	return context.Background()
}

func (c *WSConn) NewCtxWithTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	base := c.Context()
	if _, ok := base.Deadline(); ok {
		return base, func() {}
	}
	return context.WithTimeout(base, timeout)
}

func (c *WSConn) Send(payload []byte) error {
	select {
	case <-c.closed:
		return errors.New("connection closed")
	case c.send <- payload:
		return nil
	default:
		return errors.New("send queue full")
	}
}

func (c *WSConn) CloseWithReason(reason string) {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason), time.Now().Add(2*time.Second))
		_ = c.conn.Close()
	})
}

func (c *WSConn) Close() {
	c.CloseWithReason("closed")
}

// Unsubscribe from jackpot events
func (c *WSConn) StopJackpot() {
	c.jackpotMu.Lock()
	cancel := c.jackpotCancel
	c.jackpotCancel = nil
	c.jackpotMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (h *EventsWSHandler) Stream(g *gin.Context) {
	if h.connMgr == nil || !h.connMgr.HasRedis() {
		ErrorWithMessage(g, http.StatusInternalServerError, "redis client not configured")
		return
	}

	tokenString := g.Query("token")
	if tokenString == "" {
		ErrorWithMessage(g, http.StatusUnauthorized, "missing token")
		return
	}

	claims, err := parseToken(tokenString, h.app.config.JWT.Secret)
	if err != nil {
		ErrorWithMessage(g, http.StatusUnauthorized, "invalid or expired token")
		return
	}

	if claims.UserID == "" {
		ErrorWithMessage(g, http.StatusUnauthorized, "invalid token claims")
		return
	}

	if claims.TenantID == "" {
		claims.TenantID = "fgs"
	}
	if claims.CurrencyID == "" {
		claims.CurrencyID = "gold"
	}

	conn, err := h.upgrader.Upgrade(g.Writer, g.Request, nil)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to upgrade to websocket")
		return
	}

	wsConn := &WSConn{
		ID:       uuid.NewString(),
		TenantID: claims.TenantID,
		UserID:   claims.UserID,
		conn:     conn,
		logger: h.logger.With().
			Str("conn_id", uuid.NewString()).
			Str("tenant_id", claims.TenantID).
			Str("user_id", claims.UserID).
			Logger(),
		mgr:    h.connMgr,
		send:   make(chan []byte, 256),
		closed: make(chan struct{}),
	}

	wsConn.logger = h.logger.With().
		Str("conn_id", wsConn.ID).
		Str("tenant_id", claims.TenantID).
		Str("user_id", claims.UserID).
		Logger()

	wsConn.baseCtx = h.buildWSBaseContext(g.Request.Context(), claims)

	h.connMgr.Register(wsConn)

	ctx := context.Background()
	oldConnID, err := h.connMgr.AcquireSession(ctx, wsConn.TenantID, wsConn.UserID, wsConn.ID, h.sessionTTL)
	if err != nil {
		h.connMgr.Unregister(wsConn.ID)
		_ = conn.Close()
		ErrorWithMessage(g, http.StatusInternalServerError, "failed to acquire session")
		return
	}
	if oldConnID != "" && oldConnID != wsConn.ID {
		_ = h.connMgr.PublishKick(ctx, WSKickMessage{
			TenantID: wsConn.TenantID,
			UserID:   wsConn.UserID,
			ConnID:   oldConnID,
			Reason:   "duplicate_connection",
		})
	}

	conn.SetReadLimit(1 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	})

	go h.writePump(wsConn)
	defer func() {
		wsConn.StopJackpot()
		wsConn.Close()
		h.connMgr.Unregister(wsConn.ID)
		_ = h.connMgr.ReleaseSession(context.Background(), wsConn.TenantID, wsConn.UserID, wsConn.ID)
	}()

	h.autoSubscribeJackpot(wsConn, claims)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var req WSRequest
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}
		h.handleMessage(wsConn, claims, req, g.Request.URL.Path)
	}
}

func (h *EventsWSHandler) writePump(c *WSConn) {
	pingTicker := time.NewTicker(25 * time.Second)
	defer pingTicker.Stop()

	refreshTicker := time.NewTicker(20 * time.Second)
	defer refreshTicker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case payload := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				c.Close()
				return
			}
		case <-pingTicker.C:
			_ = c.conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
		case <-refreshTicker.C:
			_ = h.connMgr.TouchSession(context.Background(), c.TenantID, c.UserID, c.ID, h.sessionTTL)
		}
	}
}

type wsReply struct {
	status int
	data   any
	err    error
}

func okReply(status int, data any) *wsReply {
	return &wsReply{status: status, data: data}
}

func errReply(status int, err error) *wsReply {
	return &wsReply{status: status, err: err}
}

func (h *EventsWSHandler) writeReply(c *WSConn, req WSRequest, path string, reply *wsReply) {
	if req.ID == "" || reply == nil {
		return
	}
	if reply.err != nil {
		h.sendError(c, req, path, reply.status, reply.err)
		return
	}
	h.sendSuccess(c, req, reply.status, reply.data)
}

func (h *EventsWSHandler) handleMessage(c *WSConn, claims *auth.Claims, req WSRequest, path string) {
	switch req.Type {
	case WSEventPing:
		resp := map[string]interface{}{
			"type":      "pong",
			"timestamp": time.Now().Unix(),
		}
		b, _ := json.Marshal(resp)
		_ = c.Send(b)
		return
	case WSEventJackpotSubscribe:
		h.handleJackpotSubscribe(c, req)
		//h.writeReply(c, req, path, okReply(http.StatusOK, map[string]bool{"ok": true}))
		return
	}

	h.writeReply(c, req, path, h.dispatch(c, claims, req))
}

func (h *EventsWSHandler) dispatch(c *WSConn, claims *auth.Claims, req WSRequest) *wsReply {
	switch req.Type {
	case WSEventAuthorizeGame:
		return h.handleAuthorize(c, claims, req)
	case WSEventSpin:
		return h.handleSpin(c, claims, req)
	case WSEventGetPlayerState:
		return h.handleGetState(c, claims, req)
	case WSEventBetHistory:
		return h.handleBetHistory(c, claims, req)
	default:
		return errReply(http.StatusBadRequest, apperrors.New(apperrors.ErrInvalidRequest, "unknown event type"))
	}
}

func (h *EventsWSHandler) sendSuccess(c *WSConn, req WSRequest, status int, data any) {
	wrapped := types.SuccessResponse[any]{
		StatusCode: status,
		IsSuccess:  true,
		Data:       data,
	}
	raw, _ := json.Marshal(wrapped)
	envelope := WSResponse{ID: req.ID, Type: req.Type, Data: raw}
	b, _ := json.Marshal(envelope)
	_ = c.Send(b)
}

func (h *EventsWSHandler) sendError(c *WSConn, req WSRequest, path string, status int, err error) {
	msg := err.Error()
	code := ErrUndefinedErrorCode
	if appErr, ok := err.(*apperrors.AppError); ok {
		msg = appErr.Message
		code = appErr.Code
		status = apperrors.HTTPStatusFromCode(appErr.Code)
	}

	wrapped := types.ErrorResponse{
		StatusCode: status,
		IsSuccess:  false,
		Error: types.ErrorDetail{
			Timestamp:    time.Now().Format(time.RFC3339),
			Path:         path,
			ErrorMessage: msg,
			ErrorCode:    code,
		},
	}
	raw, _ := json.Marshal(wrapped)
	envelope := WSResponse{ID: req.ID, Type: req.Type, Data: raw}
	b, _ := json.Marshal(envelope)
	_ = c.Send(b)
}

func (h *EventsWSHandler) handleAuthorize(c *WSConn, claims *auth.Claims, req WSRequest) *wsReply {
	ctx, cancel := c.NewCtxWithTimeout(wsTimeout)
	defer cancel()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrGameModuleNotFound, "Game not configured")}
	}

	cfg, err := gameModule.GetConfig(ctx)
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrInternalServerError, "Failed to retrieve game configuration")}
	}

	if h.app.stateProvider == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrInternalServerError, "State provider not configured")}
	}

	playerState, err := h.app.stateProvider.GetPlayerState(ctx, claims.UserID, gameModule.GetGameCode())
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.Wrap(err, apperrors.ErrPlayerStateError, "Failed to get player state")}
	}

	s, ok := playerState.(*game.PlayerState)
	if ok {
		s.FreeSpins = nil
	}

	if h.app.walletProvider == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrInternalServerError, "Wallet provider not configured")}
	}

	balance, err := h.app.walletProvider.CheckBalance(ctx, h.app.gameModule.GetProductId(), claims.TenantID, claims.Username, claims.CurrencyID)
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.Wrap(err, apperrors.ErrWalletError, "Failed to get balance")}
	}

	response := game.AuthorizeResponse{
		LastState:  s,
		GameConfig: buildConfig(cfg),
		Player: game.Player{
			TenantID: claims.TenantID,
			UserID:   claims.UserID,
			Username: claims.Username,
			Balance:  balance,
			Currency: claims.CurrencyID,
		},
	}

	return &wsReply{status: http.StatusOK, data: response}
}

func (h *EventsWSHandler) handleSpin(c *WSConn, claims *auth.Claims, req WSRequest) *wsReply {
	ctx, cancel := c.NewCtxWithTimeout(wsTimeout)
	defer cancel()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrGameModuleNotFound, "Game not configured")}
	}

	cfg, err := h.app.gameModule.GetConfig(ctx)
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrConfigError, "Fail to get game config")}
	}

	tier, mul := cfg.GetConfig().Tier, cfg.GetConfig().Multiplier
	if len(tier) == 0 || len(mul) == 0 {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrConfigError, "Bet setting not found")}
	}

	var spinReq SpinRequest
	if err := json.Unmarshal(req.Data, &spinReq); err != nil {
		return &wsReply{status: http.StatusBadRequest, err: apperrors.New(apperrors.ErrInvalidRequest, "Invalid request payload")}
	}

	if !lo.Contains(tier, spinReq.Tier) || !lo.Contains(mul, spinReq.Multiplier) {
		return &wsReply{status: http.StatusBadRequest, err: apperrors.New(apperrors.ErrInvalidRequest, "Bet setting not found")}
	}

	if claims.Username == "" {
		return &wsReply{status: http.StatusBadRequest, err: apperrors.New(apperrors.ErrInvalidRequest, "Username not found in JWT")}
	}
	if claims.Name == "" {
		return &wsReply{status: http.StatusBadRequest, err: apperrors.New(apperrors.ErrInvalidRequest, "Name not found in JWT")}
	}

	betMul := float32(decimal.NewFromFloat32(spinReq.Tier).Mul(decimal.NewFromFloat32(spinReq.Multiplier)).InexactFloat64())

	gameService := h.app.newGameService(
		gameModule,
		h.app.stateProvider,
		h.app.walletProvider,
		h.app.rewardProvider,
		h.app.logProvider,
		h.logger,
	)

	result, err := gameService.ExecuteSpinV2(ctx, &SpinServiceRequest{
		TenantID:      claims.TenantID,
		UserID:        claims.UserID,
		Username:      claims.Username,
		Name:          claims.Name,
		CurrencyID:    claims.CurrencyID,
		BetMultiplier: betMul,
		Tier:          spinReq.Tier,
		Multiplier:    spinReq.Multiplier,
		CheatPayout:   spinReq.CheatPayout,
		ExtraData:     spinReq.ExtraData,
	})
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: err}
	}

	response := result.SpinResult.ToSpinResponse()
	return &wsReply{status: http.StatusOK, data: response}
}

func (h *EventsWSHandler) handleGetState(c *WSConn, claims *auth.Claims, req WSRequest) *wsReply {
	ctx, cancel := c.NewCtxWithTimeout(wsTimeout)
	defer cancel()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrGameModuleNotFound, "Game not configured")}
	}

	if h.app.stateProvider == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrInternalServerError, "State provider not configured")}
	}

	playerState, err := h.app.stateProvider.GetPlayerState(ctx, claims.UserID, gameModule.GetGameCode())
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.Wrap(err, apperrors.ErrPlayerStateError, "Failed to get player state")}
	}

	return &wsReply{status: http.StatusOK, data: playerState}
}

type betHistoryWSRequest struct {
	Type     BetType `json:"type"`
	GameCode string  `json:"gameCode"`
	Limit    int     `json:"limit"`
	Page     int     `json:"page"`
}

func (h *EventsWSHandler) handleBetHistory(c *WSConn, claims *auth.Claims, req WSRequest) *wsReply {
	ctx, cancel := c.NewCtxWithTimeout(wsTimeout)
	defer cancel()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrGameModuleNotFound, "Game not configured")}
	}

	if h.app.logProvider == nil {
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.New(apperrors.ErrInternalServerError, "Log provider not configured")}
	}

	var params betHistoryWSRequest
	if err := json.Unmarshal(req.Data, &params); err != nil {
		return &wsReply{status: http.StatusBadRequest, err: apperrors.New(apperrors.ErrInvalidRequest, "Invalid request payload")}
	}

	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	if params.Page < 0 {
		params.Page = 0
	}
	if params.GameCode == "" {
		params.GameCode = gameModule.GetGameCode()
	}

	var userID string
	if params.Type != BetTypeJackpot {
		userID = claims.UserID
	}

	query := &BetHistoryQuery{
		UserID:   userID,
		GameCode: params.GameCode,
		Type:     params.Type,
		Page:     params.Page,
		Limit:    params.Limit,
	}

	result, err := h.app.logProvider.GetBetHistory(ctx, query)
	if err != nil {
		if r := h.timeoutReplyIfNeeded(ctx, err); r != nil {
			return r
		}
		return &wsReply{status: http.StatusInternalServerError, err: apperrors.Wrap(err, apperrors.ErrInternalServerError, "Failed to get bet history")}
	}

	return &wsReply{status: http.StatusOK, data: result}
}

type jackpotSubscribeWSRequest struct {
	BetMultiplier    float32 `json:"bet_multiplier"`
	BetMultiplierAlt float32 `json:"betMultiplier"`
}

type jackpotWSSender struct {
	conn *WSConn
}

func (s *jackpotWSSender) Send(resp *Response) error {
	cp := *resp
	payload, err := json.Marshal(&cp)
	if err != nil {
		return err
	}
	return s.conn.Send(payload)
}

func (h *EventsWSHandler) handleJackpotSubscribe(c *WSConn, req WSRequest) {
	var subReq jackpotSubscribeWSRequest
	if err := json.Unmarshal(req.Data, &subReq); err != nil {
		return
	}

	betMultiplier := subReq.BetMultiplier
	if betMultiplier == 0 {
		betMultiplier = subReq.BetMultiplierAlt
	}
	if betMultiplier <= 0 {
		return
	}

	h.subscribeJackpot(c, betMultiplier)
}

func (h *EventsWSHandler) autoSubscribeJackpot(c *WSConn, claims *auth.Claims) {
	if h.app.stateProvider == nil {
		return
	}
	gameModule := h.app.GetGame()
	if gameModule == nil {
		return
	}
	ctx, cancel := c.NewCtxWithTimeout(wsTimeout)
	defer cancel()
	state, err := h.app.stateProvider.GetPlayerState(ctx, claims.UserID, gameModule.GetGameCode())
	if err != nil {
		return
	}
	ps, ok := state.(*game.PlayerState)
	if !ok || ps == nil {
		return
	}
	betMultiplier := ps.BetMultiplier
	if betMultiplier <= 0 && ps.Tier > 0 && ps.Mul > 0 {
		betMultiplier = float32(decimal.NewFromFloat32(ps.Tier).Mul(decimal.NewFromFloat32(ps.Mul)).InexactFloat64())
	}
	if betMultiplier <= 0 {
		return
	}
	h.subscribeJackpot(c, betMultiplier)
}

func (h *EventsWSHandler) subscribeJackpot(c *WSConn, betMultiplier float32) {
	c.StopJackpot()

	base := c.Context()
	ctx, cancel := context.WithCancel(base)
	c.jackpotMu.Lock()
	c.jackpotCancel = cancel
	c.jackpotMu.Unlock()

	gameModule := h.app.GetGame()
	if gameModule == nil {
		cancel()
		return
	}

	gameCode := gameModule.GetGameCode()
	var targetPoolIDs []string
	if handler, ok := gameModule.(game.JackpotHandler); ok {
		poolIDs, err := handler.GetPoolID(ctx, gameCode, betMultiplier)
		if err != nil {
			cancel()
			return
		}
		targetPoolIDs = poolIDs
	}
	isTargetPool := func(poolID string) bool {
		return len(targetPoolIDs) == 0 || lo.Contains(targetPoolIDs, poolID)
	}
	config := &streamConfig{
		betMultiplier: betMultiplier,
		targetPoolIDs: targetPoolIDs,
		isTargetPool:  isTargetPool,
		ctx:           ctx,
	}

	sender := &jackpotWSSender{conn: c}
	go h.app.jackpotHandler.streamUpdates(config, sender)
}

func parseToken(tokenString string, secret string) (*auth.Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &auth.Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*auth.Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}
