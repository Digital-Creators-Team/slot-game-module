# Architecture Diagrams (Mermaid)

## 1. System Architecture Overview

```mermaid
graph TB
    Client[Client/Frontend] -->|HTTP/SSE| Server[HTTP Server Layer]
    
    Server --> Middleware[Middleware Stack]
    Middleware -->|Auth, Logging, Recovery| Handlers[Handlers]
    
    Handlers --> GameHandler[GameHandler]
    Handlers --> JackpotHandler[JackpotHandler]
    
    GameHandler --> GameService[GameService]
    JackpotHandler --> JackpotService[JackpotService]
    
    GameService --> GameModule[Game Module]
    GameService --> Providers[Providers]
    
    Providers --> StateProvider[StateProvider]
    Providers --> WalletProvider[WalletProvider]
    Providers --> RewardProvider[RewardProvider]
    Providers --> LogProvider[LogProvider]
    
    style Client fill:#e1f5ff
    style Server fill:#fff4e1
    style GameService fill:#e8f5e9
    style GameModule fill:#f3e5f5
    style Providers fill:#fce4ec
```

## 2. Spin Flow (Normal Spin)

```mermaid
sequenceDiagram
    participant C as Client
    participant GH as GameHandler
    participant GS as GameService
    participant GM as GameModule
    participant WP as WalletProvider
    participant SP as StateProvider
    participant RP as RewardProvider
    participant LP as LogProvider
    
    C->>GH: POST /api/games/{code}/spin
    GH->>GH: Extract JWT, Validate
    GH->>GS: ExecuteSpin()
    
    GS->>SP: GetPlayerState()
    SP-->>GS: PlayerState
    
    GS->>WP: Withdraw(bet)
    WP-->>GS: OK
    
    GS->>GM: PlayNormalSpin()
    GM->>GM: Generate reels
    GM->>GM: Calculate winlines
    GM-->>GS: SpinResult
    
    Note over GS,RP: Progressive amount before spin<br/>Contribute first, then claim if won
    GS->>RP: Contribute(poolID, amount)
    RP-->>GS: OK
    
    alt IsGetJackpot
        GS->>RP: Claim(poolID)
        RP-->>GS: ClaimResult
    end
    
    GS->>WP: Deposit(winnings)
    WP-->>GS: OK
    
    GS->>SP: SavePlayerState()
    SP-->>GS: OK
    
    GS->>LP: LogSpin()
    LP-->>GS: SessionID
    
    GS-->>GH: SpinServiceResponse
    GH-->>C: SpinResponse
```

## 3. Free Spin Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant GH as GameHandler
    participant GS as GameService
    participant GM as GameModule
    participant SP as StateProvider
    
    C->>GH: POST /api/games/{code}/spin
    GH->>GS: ExecuteSpin()
    
    GS->>SP: GetPlayerState()
    SP-->>GS: PlayerState (IsFreeSpin=true)
    
    GS->>GS: Get pre-generated free spin
    Note over GS: playerState.FreeSpins[playedIndex]
    
    GS->>GS: Update player state
    Note over GS: Increment PlayedFreeSpin<br/>Decrement RemainingFreeSpin
    
    alt Last Free Spin
        GS->>GS: Deposit total free spin winnings
        GS->>GS: Reset player state
    end
    
    GS->>SP: SavePlayerState()
    SP-->>GS: OK
    
    GS-->>GH: SpinServiceResponse
    GH-->>C: SpinResponse
```

## 4. Free Spin Trigger Flow

```mermaid
sequenceDiagram
    participant GS as GameService
    participant GM as GameModule
    participant SP as StateProvider
    
    GS->>GM: PlayNormalSpin()
    GM-->>GS: SpinResult (IsGetFreeSpin=true)
    
    GS->>GM: GenerateFreeSpins(count)
    loop For each free spin
        GM->>GM: PlayFreeSpin()
        GM-->>GM: SpinResult
    end
    GM-->>GS: []SpinResult (pre-generated)
    
    GS->>GS: Update player state
    Note over GS: IsFreeSpin = true<br/>RemainingFreeSpin = count<br/>FreeSpins = [results]
    
    GS->>SP: SavePlayerState()
    SP-->>GS: OK
```

## 5. Jackpot Flow

```mermaid
flowchart TD
    Start[Execute Spin] --> GetContrib["Get Contributions<br/>(Progressive amount before spin)"]
    
    GetContrib --> Loop{For each pool}
    Loop --> Contribute[Contribute Amount]
    Contribute --> Loop
    Loop -->|Done| Check{IsGetJackpot?}
    
    Check -->|Yes| GetWin[Get Win Info]
    GetWin --> Claim[Claim Jackpot]
    Claim --> UpdateWin[Update SpinResult]
    UpdateWin --> LogJackpot[Log Jackpot Win]
    LogJackpot --> End[End]
    
    Check -->|No| End
    
    style Check fill:#fff4e1
    style GetWin fill:#e8f5e9
    style Claim fill:#e8f5e9
    style GetContrib fill:#fce4ec
    style Contribute fill:#fce4ec
```

## 6. Jackpot SSE Stream Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant JH as JackpotHandler
    participant GM as GameModule
    participant JS as JackpotService
    
    C->>JH: GET /api/games/{code}/jackpot/stream
    Note over C,JH: Server-Sent Events
    
    JH->>GM: GetPoolID()
    GM-->>JH: ["game:mini", "game:minor", "game:grand"]
    
    loop For each pool
        JH->>GM: GetInitialPoolValue(poolID)
        GM-->>JH: Initial value
    end
    
    JH->>C: SSE: Initial values
    
    JH->>JS: Subscribe(poolIDs)
    
    loop On pool update
        JS->>JH: Pool updated
        JH->>C: SSE: Update event
    end
```

## 7. Module Interface Structure

```mermaid
classDiagram
    class Module {
        <<interface>>
        +GetConfig(ctx) ConfigNormalizer
        +PlayNormalSpin(ctx, betMultiplier, cheatPayout) SpinResult
        +PlayFreeSpin(ctx, betMultiplier) SpinResult
        +GenerateFreeSpins(ctx, betMultiplier, count) []SpinResult
        +GetGameCode() string
    }
    
    class BaseModule {
        +GameCode string
        +Config ConfigNormalizer
        +GetConfig(ctx) ConfigNormalizer
        +GetGameCode() string
    }
    
    class JackpotHandler {
        <<interface>>
        +GetContributions(ctx, spinResult, totalBet, config) []JackpotContribution
        +GetWin(ctx, spinResult, totalBet, config) JackpotWin
        +GetPoolID(ctx, gameCode, betMultiplier, config) []string
        +GetInitialPoolValue(ctx, poolID, betMultiplier, config) decimal
    }
    
    class GameModule {
        +BaseModule
        +JackpotHandler
        +rng *rand.Rand
        +gameConfig *GameConfig
        +PlayNormalSpin(...) SpinResult
        +PlayFreeSpin(...) SpinResult
    }
    
    Module <|.. BaseModule
    Module <|.. GameModule
    JackpotHandler <|.. GameModule
    BaseModule <|-- GameModule
```

## 8. Provider Interfaces

```mermaid
classDiagram
    class StateProvider {
        <<interface>>
        +GetPlayerState(ctx, userID, gameCode) any
        +SavePlayerState(ctx, userID, gameCode, state) error
    }
    
    class WalletProvider {
        <<interface>>
        +GetBalance(ctx, userID, currencyID) float64
        +Withdraw(ctx, userID, currencyID, amount) error
        +Deposit(ctx, userID, currencyID, amount) error
    }
    
    class RewardProvider {
        <<interface>>
        +Contribute(ctx, poolID, amount, gameCode) error
        +Claim(ctx, poolID, userID, gameCode, initValue) ClaimResult
    }
    
    class LogProvider {
        <<interface>>
        +LogSpin(ctx, log) string
        +LogJackpot(ctx, log) string
        +GetBetHistory(ctx, query) BetHistoryResponse
    }
    
    class GameService {
        -stateProvider StateProvider
        -walletProvider WalletProvider
        -rewardProvider RewardProvider
        -logProvider LogProvider
        +ExecuteSpin(ctx, req) SpinServiceResponse
    }
    
    GameService --> StateProvider
    GameService --> WalletProvider
    GameService --> RewardProvider
    GameService --> LogProvider
```

## 9. Data Flow

```mermaid
flowchart LR
    A[Client Request] --> B[GameHandler]
    B --> C[GameService]
    C --> D[GameModule]
    D --> E[SpinResult]
    E --> C
    C --> F[WalletProvider]
    C --> G[StateProvider]
    C --> H[RewardProvider]
    C --> I[LogProvider]
    F --> C
    G --> C
    H --> C
    I --> C
    C --> J[SpinServiceResponse]
    J --> B
    B --> K[Client Response]
    
    style A fill:#e1f5ff
    style K fill:#e1f5ff
    style D fill:#f3e5f5
    style C fill:#e8f5e9
```

## 10. Error Handling Flow

```mermaid
flowchart TD
    Start[Error Occurs] --> Check{Error Type?}
    
    Check -->|400| BadRequest[Bad Request]
    Check -->|401| Unauthorized[Unauthorized]
    Check -->|500| ServerError[Server Error]
    Check -->|Custom| CustomError[Custom Error]
    
    BadRequest --> Format[Format Error Response]
    Unauthorized --> Format
    ServerError --> Format
    CustomError --> Format
    
    Format --> Response{Error Response}
    Response -->|JSON| Client[Client]
    
    style Check fill:#fff4e1
    style Format fill:#fce4ec
    style Response fill:#e8f5e9
```

## 11. Configuration Loading

```mermaid
flowchart TD
    Start[Load Config] --> Load[LoadGameConfig]
    Load --> Parse[Parse YAML]
    Parse --> Validate{Valid?}
    
    Validate -->|No| Error[Return Error]
    Validate -->|Yes| Base[Load Base Config]
    Base --> Custom{Has Custom Fields?}
    
    Custom -->|Yes| CustomConfig[Load Custom Config]
    Custom -->|No| BaseConfig[Use Base Config Only]
    
    CustomConfig --> Merge[Merge Configs]
    BaseConfig --> Merge
    Merge --> Return[Return Config]
    
    style Validate fill:#fff4e1
    style Merge fill:#e8f5e9
```

## 12. Game Registration Flow

```mermaid
sequenceDiagram
    participant Main as main.go
    participant Factory as ModuleFactory
    participant Registry as GameRegistry
    participant Module as GameModule
    
    Main->>Factory: Create ModuleFactory
    Factory->>Module: NewGameModule(configPath)
    Module->>Module: Load config
    Module->>Module: Initialize
    Module-->>Factory: GameModule
    
    Main->>Registry: Register(gameCode, factory)
    Registry->>Registry: Store factory
    
    Note over Registry: When request comes in
    Registry->>Factory: Create module
    Factory->>Module: NewGameModule()
    Module-->>Factory: GameModule
    Factory-->>Registry: GameModule
```

## 13. CLI Flow - Create Project

```mermaid
flowchart TD
    A["Developer<br/>slotmodule create"] --> B["Parse flags<br/>name, port, module, paylines"]
    B --> C["Generate templates"]
    C --> C1["Base code<br/>main.go,<br/>module.go,<br/>config.go"]
    C --> C2["Config files<br/>config.yaml,<br/>module-base.yml,<br/>game_code.yaml"]
    C --> C3["Docker & Compose"]
    C --> C4["GitLab CI<br/>.gitlab-ci.yml"]
    C --> C5["Makefile,<br/>README,<br/>.gitignore"]
    C --> C6["Swagger placeholder<br/>docs/docs.go"]
    C --> C7["Metadata<br/>.slotmodule.json"]
    C --> D["Write files to project dir"]
    D --> E["Post-setup<br/>go mod tidy<br/>go mod vendor<br/>swag init"]
    E --> F["Project ready<br/>make run<br/>swagger UI"]
```

## 14. CLI Flow - Update Project

```mermaid
flowchart TD
    A["Developer<br/>slotmodule update"] --> B["Load .slotmodule.json"]
    B --> C["Resolve template data"]
    C --> D["List updatable files<br/>(Dockerfile, compose,<br/>.gitlab-ci.yml, Makefile,<br/>main/module/config,<br/>README)"]
    D --> E["Select files<br/>--files or all"]
    E --> F["Generate new content<br/>from templates"]
    F --> G["Diff & prompt<br/>dry-run / force / backup"]
    G --> H["Write files<br/>(with backup if enabled)"]
    H --> I["Update metadata<br/>CLI/core versions"]
    I --> J[Summary]
```

