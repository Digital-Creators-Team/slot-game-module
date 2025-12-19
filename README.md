# Slot Game Module

A public Go module for building slot game services. This module provides a complete framework for creating slot game microservices with built-in support for game logic, jackpot management, state management, and more.

A public Go module for building slot game services. This module provides a complete framework for creating slot game microservices with built-in support for game logic, jackpot management, state management, and more.

## Features

- 🎮 **Game Module Interface**: Standardized interface for implementing slot game logic
- 🎰 **Jackpot Management**: Built-in progressive jackpot system with multiple tiers
- 💾 **State Management**: Redis-based state persistence
- 🔌 **Provider System**: Pluggable providers for wallet, reward, logging, and state services
- 🚀 **CLI Tool**: `slotmodule` CLI for generating new game projects
- 📝 **Swagger Documentation**: Auto-generated API documentation
- 🐳 **Docker Support**: Ready-to-use Dockerfiles and docker-compose configurations

## Installation

```bash
go get git.futuregamestudio.net/be-shared/slot-game-module.git
```

## Updating Module Version

To update the slot-game-module to the latest version:

```bash
go get -u git.futuregamestudio.net/be-shared/slot-game-module.git
```

This will update the module to the latest version and update your `go.mod` file accordingly. After updating, run:

```bash
go mod tidy
go mod vendor  # if you're using vendoring
```

## Quick Start

### Install CLI Tool

```bash
go install git.futuregamestudio.net/be-shared/slot-game-module.git/cmd/slotmodule@latest
```

### Create a New Game

```bash
slotmodule create --name my-game --port 8080
```

This will generate a complete game project with:
- Game module implementation
- Configuration files
- Docker setup
- CI/CD configuration
- Swagger documentation

## Usage

### Basic Game Module

```go
package logic

import (
    "context"
    "git.futuregamestudio.net/be-shared/slot-game-module.git/game"
)

type MyGameModule struct {
    game.BaseModule
    // Your custom fields
}

func (m *MyGameModule) PlayNormalSpin(ctx context.Context, betMultiplier float32, cheatPayout interface{}) (*game.SpinResult, error) {
    // Implement your game logic
    return &game.SpinResult{
        Reels:    generateReels(),
        Winlines: calculateWinlines(),
        TotalWin: calculateWin(),
        // ...
    }, nil
}
```

### Server Setup

```go
import (
    "git.futuregamestudio.net/be-shared/slot-game-module.git/server"
    "git.futuregamestudio.net/be-shared/slot-game-module.git/game"
)

// Create your game module
module, _ := NewMyGameModule("config/")

// Create server with module
srv := server.NewGameServer(module, &server.Config{
    Port: 8080,
    // ... other config
})

// Start server
srv.Start()
```

## Architecture

- **game/**: Core game interfaces and base implementations
- **server/**: HTTP server with game endpoints
- **provider/**: External service providers (wallet, reward, logging, state)
- **pkg/jackpot/**: Jackpot management system
- **middleware/**: HTTP middleware (CORS, logging, recovery, etc.)
- **cmd/slotmodule/**: CLI tool for project generation

## Configuration

Games are configured using YAML files:

```yaml
# config.yaml
server:
  port: 8080

redis:
  addr: localhost:6379

game:
  pay_line: 20
  reel_rows: 3
  reel_cols: 5
```

## Documentation

- [API Documentation](docs/) - Auto-generated Swagger docs
- [Game Development Guide](docs/GUIDE.md) - How to implement a new game
- [Architecture Overview](docs/ARCHITECTURE.md) - System architecture

## Games Registry

See the list of all deployed games: [Games Registry](https://git.futuregamestudio.net/be-shared/slot-game-module/-/blob/games-registry/GAMES.md)

## License

This project is licensed under the MIT License.
