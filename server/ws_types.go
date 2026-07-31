package server

import "encoding/json"

type WSRequest struct {
	ID   string          `json:"id"`
	Type WSEventType     `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type WSResponse struct {
	ID   string          `json:"id,omitempty"`
	Type WSEventType     `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

type WSKickMessage struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	ConnID   string `json:"conn_id"`
	Reason   string `json:"reason,omitempty"`
}
