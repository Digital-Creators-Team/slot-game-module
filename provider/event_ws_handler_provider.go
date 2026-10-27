package provider

import (
	"context"

	"github.com/Digital-Creators-Team/slot-game-module/server"
)

type eventWsHandlerProvider struct {
	EventsWSHandler *server.EventsWSHandler
}

func NewEventWsHandlerProvider(ws *server.EventsWSHandler) *eventWsHandlerProvider {
	return &eventWsHandlerProvider{
		EventsWSHandler: ws,
	}
}

func (p *eventWsHandlerProvider) SendExtraJackpotSubscribe(ctx context.Context, userID string, betMultiplier float32) error {
	return p.EventsWSHandler.SendExtraJackpotSubscribe(ctx, userID, betMultiplier)
}
