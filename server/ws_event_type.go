package server

type WSEventType string

const (
	WSEventPing             WSEventType = "ping"
	WSEventPong             WSEventType = "pong"
	WSEventJackpotSubscribe WSEventType = "jp-subscribe"

	WSEventAuthorizeGame  WSEventType = "authorize-game"
	WSEventSpin           WSEventType = "spin"
	WSEventGetPlayerState WSEventType = "get-player-state"
	WSEventBetHistory     WSEventType = "bet-history"

	WSEventJackpotConnected WSEventType = "connected"
	WSEventJackpotUpdated   WSEventType = "updated"
	WSEventJackpotHeartbeat WSEventType = "heartbeat"
)
