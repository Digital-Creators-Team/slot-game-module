package server

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/auth"
	apperrors "github.com/Digital-Creators-Team/slot-game-module/errors"
	"github.com/Digital-Creators-Team/slot-game-module/types"
	"github.com/golang-jwt/jwt/v5"
)

func newTestWSConn() *WSConn {
	return &WSConn{
		send:   make(chan []byte, 16),
		closed: make(chan struct{}),
	}
}

func recvPayload(t *testing.T, c *WSConn) []byte {
	t.Helper()

	select {
	case b := <-c.send:
		t.Logf("ws payload: %s", string(b))
		return b
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for ws payload")
		return nil
	}
}

func TestEventsWSHandler_SendSuccess(t *testing.T) {
	h := &EventsWSHandler{}
	c := newTestWSConn()

	req := WSRequest{ID: "1", Type: WSEventSpin}
	h.sendSuccess(c, req, http.StatusOK, map[string]any{"a": 1})
	b := recvPayload(t, c)

	var resp WSResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal WSResponse: %v", err)
	}
	if resp.ID != "1" {
		t.Fatalf("expected id=1, got %q", resp.ID)
	}
	if resp.Type != WSEventSpin {
		t.Fatalf("expected type=%q, got %q", WSEventSpin, resp.Type)
	}
	if len(resp.Data) == 0 {
		t.Fatalf("expected data not empty")
	}

	var wrapped types.SuccessResponse[map[string]any]
	if err := json.Unmarshal(resp.Data, &wrapped); err != nil {
		t.Fatalf("unmarshal success wrapper: %v", err)
	}
	if wrapped.IsSuccess != true {
		t.Fatalf("expected is_success=true, got %v", wrapped.IsSuccess)
	}
	if wrapped.StatusCode != http.StatusOK {
		t.Fatalf("expected status_code=%d, got %d", http.StatusOK, wrapped.StatusCode)
	}
	if wrapped.Data["a"] != float64(1) {
		t.Fatalf("expected data.a=1, got %v", wrapped.Data["a"])
	}
}

func TestEventsWSHandler_SendError(t *testing.T) {
	h := &EventsWSHandler{}
	c := newTestWSConn()

	req := WSRequest{ID: "1", Type: WSEventSpin}
	h.sendError(c, req, "/ws", http.StatusBadRequest, apperrors.New(apperrors.ErrInvalidRequest, "bad request"))
	b := recvPayload(t, c)

	var resp WSResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal WSResponse: %v", err)
	}
	if resp.ID != "1" {
		t.Fatalf("expected id=1, got %q", resp.ID)
	}
	if resp.Type != WSEventSpin {
		t.Fatalf("expected type=%q, got %q", WSEventSpin, resp.Type)
	}
	if len(resp.Data) == 0 {
		t.Fatalf("expected data not empty")
	}

	var wrapped types.ErrorResponse
	if err := json.Unmarshal(resp.Data, &wrapped); err != nil {
		t.Fatalf("unmarshal error wrapper: %v", err)
	}
	if wrapped.IsSuccess != false {
		t.Fatalf("expected is_success=false, got %v", wrapped.IsSuccess)
	}
	if wrapped.Error.ErrorCode != apperrors.ErrInvalidRequest {
		t.Fatalf("expected error_code=%d, got %d", apperrors.ErrInvalidRequest, wrapped.Error.ErrorCode)
	}
	if wrapped.Error.Path != "/ws" {
		t.Fatalf("expected path=/ws, got %q", wrapped.Error.Path)
	}
}

func TestEventsWSHandler_HandleMessage_Ping(t *testing.T) {
	h := &EventsWSHandler{}
	c := newTestWSConn()

	h.handleMessage(c, nil, WSRequest{Type: WSEventPing}, "/ws")
	b := recvPayload(t, c)

	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["type"] != "pong" {
		t.Fatalf("expected type=pong, got %v", out["type"])
	}
	if out["timestamp"] == nil {
		t.Fatalf("expected timestamp not nil")
	}
}

func TestEventsWSHandler_HandleMessage_UnknownWithID(t *testing.T) {
	h := &EventsWSHandler{}
	c := newTestWSConn()

	claims := &auth.Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	h.handleMessage(c, claims, WSRequest{ID: "1", Type: WSEventType("unknown")}, "/ws")
	b := recvPayload(t, c)

	var resp WSResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal WSResponse: %v", err)
	}
	if resp.ID != "1" {
		t.Fatalf("expected id=1, got %q", resp.ID)
	}
	if resp.Type != WSEventType("unknown") {
		t.Fatalf("expected type=unknown, got %q", resp.Type)
	}
	var wrapped types.ErrorResponse
	if err := json.Unmarshal(resp.Data, &wrapped); err != nil {
		t.Fatalf("unmarshal error wrapper: %v", err)
	}
	if wrapped.Error.ErrorCode != apperrors.ErrInvalidRequest {
		t.Fatalf("expected error_code=%d, got %d", apperrors.ErrInvalidRequest, wrapped.Error.ErrorCode)
	}
}

func TestEventsWSHandler_Dispatch_ExpiredToken(t *testing.T) {
	h := &EventsWSHandler{}
	c := newTestWSConn()

	claims := &auth.Claims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}

	h.handleMessage(c, claims, WSRequest{ID: "1", Type: WSEventGetPlayerState}, "/ws")
	b := recvPayload(t, c)

	var resp WSResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal WSResponse: %v", err)
	}
	if resp.ID != "1" {
		t.Fatalf("expected id=1, got %q", resp.ID)
	}

	var wrapped types.ErrorResponse
	if err := json.Unmarshal(resp.Data, &wrapped); err != nil {
		t.Fatalf("unmarshal error wrapper: %v", err)
	}
	if wrapped.IsSuccess != false {
		t.Fatalf("expected is_success=false, got %v", wrapped.IsSuccess)
	}
	// Client maps error_code 401 (and status_code 401) to InvalidToken.
	if wrapped.Error.ErrorCode != apperrors.ErrUnauthorized {
		t.Fatalf("expected error_code=%d, got %d", apperrors.ErrUnauthorized, wrapped.Error.ErrorCode)
	}
	if wrapped.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status_code=%d, got %d", http.StatusUnauthorized, wrapped.StatusCode)
	}
}

func TestJackpotWSSender_Send_Updated(t *testing.T) {
	c := newTestWSConn()
	s := &jackpotWSSender{conn: c}

	resp := &Response{
		Type:      EventTypeUpdated,
		Timestamp: 123,
		Pools: map[string]PoolUpdate{
			"pool-1": {Amount: 10.5, Timestamp: 122},
		},
	}

	if err := s.Send(resp); err != nil {
		t.Fatalf("send: %v", err)
	}
	b := recvPayload(t, c)

	var out Response
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal Response: %v", err)
	}
	if out.Type != EventTypeUpdated {
		t.Fatalf("expected type=%q, got %q", EventTypeUpdated, out.Type)
	}
	if out.Timestamp != 123 {
		t.Fatalf("expected timestamp=123, got %d", out.Timestamp)
	}
	if len(out.Pools) != 1 || out.Pools["pool-1"].Amount != 10.5 {
		t.Fatalf("unexpected pools: %+v", out.Pools)
	}
}
