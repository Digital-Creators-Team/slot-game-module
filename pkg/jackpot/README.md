# Jackpot package (reusable)

Transport-agnostic helper to manage jackpot pools:
- register pools
- compute contributions
- buffer incoming updates (Kafka/others)
- flush on interval to a channel for SSE/WS

## Quick start
```go
svc := jackpot.NewService(jackpot.ServiceConfig{
    BroadcastInterval: 2 * time.Second,
    Logger: logger, // optional zerolog
})

// Register pools
svc.RegisterPool(jackpot.PoolConfig{
    ID:   "game:mini",
    Init: decimal.NewFromInt(1000),
    Prog: decimal.NewFromFloat(0.01), // 1% of total bet
})

// During spin: compute contributions
contribs := svc.Contribute(totalBet) // []jackpot.Contribution
// caller applies to reward provider / DB, etc.

// Push updates from Kafka/worker
svc.HandleKafkaUpdate(jackpot.Update{
    PoolID: "game:mini",
    Amount: decimal.NewFromInt(12345),
    Timestamp: time.Now(),
})

// Expose SSE/WS: subscribe to Listen()
updates, cancel := svc.Listen(ctx)
defer cancel()
for u := range updates {
    // stream to client (SSE/WS)
}

// Stop on shutdown
svc.Stop()
```

## Wiring in this repo
- `server.New()` creates a shared `jackpotService`.
- RewardProvider is wired automatically via `SetRewardProvider`; RegisterGame sets gameCode.
- Feed updates from Kafka consumer via `App.AttachJackpotUpdateFeed(ch <-chan jackpot.Update)`.
  - Topic key: `kafka.topics.jackpot_updates` (default topic name: `jackpot-updates`).
- `server.JackpotHandler` exposes: `GET /api/games/{game_code}/jackpot/updates` (SSE stream only).
- Spin logic:
  - `svc.ContributeAndApply(ctx, totalBet)` returns contributions and persists if provider is set.
  - Or manual: `svc.Contribute(totalBet)` to get contributions and handle yourself.

## Standalone usage
Configure when creating the service:
```go
svc := jackpot.NewService(jackpot.ServiceConfig{
    BroadcastInterval: 2 * time.Second,
    Logger: logger,
    RewardProvider: providers.NewRewardProvider(cfg, logger), // Contribute/Claim/GetPool
    GameCode: "my-game",
})
```
In spin flow: `_, _ = svc.ContributeAndApply(ctx, totalBet)`.

## Notes
- Package is standalone; can be copied to other services.
- No transport or provider assumption; caller owns persistence/ledgering.
