package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/Digital-Creators-Team/slot-game-module/events/kafka"
	"github.com/Digital-Creators-Team/slot-game-module/server"
	"github.com/Digital-Creators-Team/slot-game-module/types"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
)

// SpinDetails represents spin log details for mapstructure decoding
type SpinDetails struct {
	SessionID  string      `mapstructure:"sessionId" json:"sessionId"`
	Username   string      `mapstructure:"username" json:"username"`
	GameCode   string      `mapstructure:"gameCode" json:"gameCode"`
	BetAmount  float64     `mapstructure:"betAmount" json:"betAmount"`
	WinAmount  float64     `mapstructure:"winAmount" json:"winAmount"`
	SpinType   int         `mapstructure:"spinType" json:"spinType"`
	SpinResult interface{} `mapstructure:"spinResult" json:"spinResult"`
}

// JackpotDetails represents jackpot log details for mapstructure decoding
type JackpotDetails struct {
	SessionID       string      `mapstructure:"sessionId" json:"sessionId"`
	Username        string      `mapstructure:"username" json:"username"`
	GameCode        string      `mapstructure:"gameCode" json:"gameCode"`
	Tier            string      `mapstructure:"tier" json:"tier"`
	BetAmount       float64     `mapstructure:"betAmount" json:"betAmount"`
	WinAmount       float64     `mapstructure:"winAmount" json:"winAmount"`
	TotalWinJackpot float64     `mapstructure:"totalWinJackpot" json:"totalWinJackpot"`
	Currency        string      `mapstructure:"currency" json:"currency"`
	SpinType        int         `mapstructure:"spinType" json:"spinType"`
	SpinResult      interface{} `mapstructure:"spinResult" json:"spinResult"`
}

// LogProvider implements server.LogProvider using Kafka and HTTP
type LogProvider struct {
	baseURL       string
	httpClient    *http.Client
	kafkaProducer *kafka.Producer
	auditTopic    string
	logger        zerolog.Logger
}

// NewLogProvider creates a new log provider
func NewLogProvider(cfg *config.Config, kafkaProducer *kafka.Producer, logger zerolog.Logger) *LogProvider {
	timeout := cfg.ExternalServices.LogService.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	auditTopic := "game.audit"
	if cfg.Kafka.Topics != nil {
		if t, ok := cfg.Kafka.Topics["audit"]; ok {
			auditTopic = t
		}
	}

	return &LogProvider{
		baseURL: cfg.ExternalServices.LogService.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		kafkaProducer: kafkaProducer,
		auditTopic:    auditTopic,
		logger:        logger.With().Str("component", "log_provider").Logger(),
	}
}

// AuditEvent represents an audit event for Kafka
type AuditEvent struct {
	Timestamp     time.Time   `json:"timestamp"`
	UserID        string      `json:"user_id"`
	SessionID     string      `json:"session_id,omitempty"`
	SourceService string      `json:"source_service"`
	Action        string      `json:"action"`
	Details       interface{} `json:"details"`
	Result        string      `json:"result"`
	TraceID       string      `json:"trace_id,omitempty"`
}

// LogSpin logs a spin event and returns sessionID
func (p *LogProvider) LogSpin(ctx context.Context, log *server.SpinLog) (string, error) {
	sessionID, _ := ctx.Value(server.SessionIDKey).(string)
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	if p.kafkaProducer == nil {
		p.logger.Warn().Msg("Kafka producer not configured, skipping spin log")
		return sessionID, nil
	}

	event := AuditEvent{
		Timestamp:     log.Timestamp,
		UserID:        log.UserID,
		SessionID:     sessionID,
		SourceService: log.GameCode,
		Action:        "normal", // Default action for spin
		Details: SpinDetails{
			SessionID:  sessionID,
			Username:   log.Username,
			GameCode:   log.GameCode,
			BetAmount:  log.BetAmount,
			WinAmount:  log.WinAmount,
			SpinType:   log.SpinType,
			SpinResult: log.SpinResult,
		},
		Result:  "success",
		TraceID: sessionID,
	}

	// Set action based on spin type
	// if log.SpinType == 1 {
	// 	event.Action = "free_spin"
	// }

	if err := p.kafkaProducer.SendMessage(p.auditTopic, sessionID, event); err != nil {
		p.logger.Error().Err(err).Msg("Failed to send spin log to Kafka")
		return "", fmt.Errorf("failed to log spin: %w", err)
	}

	return sessionID, nil
}

// LogJackpot logs a jackpot win event and returns sessionID
func (p *LogProvider) LogJackpot(ctx context.Context, log *server.JackpotLog) (string, error) {
	sessionID, _ := ctx.Value(server.SessionIDKey).(string)
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	if p.kafkaProducer == nil {
		p.logger.Warn().Msg("Kafka producer not configured, skipping jackpot log")
		return sessionID, nil
	}

	event := AuditEvent{
		Timestamp:     log.Timestamp,
		UserID:        log.UserID,
		SessionID:     sessionID,
		SourceService: log.GameCode,
		Action:        "jackpot",
		Details: JackpotDetails{
			SessionID:       sessionID,
			Username:        log.Username,
			GameCode:        log.GameCode,
			Tier:            log.Tier,
			BetAmount:       log.BetAmount,
			WinAmount:       log.WinAmount,
			Currency:        log.Currency,
			SpinType:        log.SpinType,
			TotalWinJackpot: log.TotalWinJackpot,
			SpinResult:      log.SpinResult,
		},
		Result:  "success",
		TraceID: sessionID,
	}

	if err := p.kafkaProducer.SendMessage(p.auditTopic, sessionID, event); err != nil {
		p.logger.Error().Err(err).Msg("Failed to send jackpot log to Kafka")
		return "", fmt.Errorf("failed to log jackpot: %w", err)
	}

	return sessionID, nil
}

// LogEntry represents an audit log entry from the log service
type LogEntry struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	UserID        string                 `json:"user_id"`
	SessionID     string                 `json:"session_id,omitempty"`
	SourceService string                 `json:"source_service"`
	Action        string                 `json:"action"`
	Details       map[string]interface{} `json:"details"`
	Result        string                 `json:"result"`
	TraceID       string                 `json:"trace_id,omitempty"`
}

// DataAuditEvent represents the response from log service
type DataAuditEvent struct {
	Logs  []LogEntry `json:"logs"`
	Total int        `json:"total"`
}

// LogServiceResponse wraps the log service response (can be success or error)
type LogServiceResponse struct {
	StatusCode int               `json:"status_code"`
	IsSuccess  bool              `json:"is_success"`
	Data       DataAuditEvent    `json:"data,omitempty"`
	Error      types.ErrorDetail `json:"error,omitempty"`
}

// GetBetHistory gets bet history for a user
func (p *LogProvider) GetBetHistory(ctx context.Context, query *server.BetHistoryQuery) (*server.BetHistoryResponse, error) {
	// Map BetType to action string
	var action string
	switch query.Type {
	case server.BetTypeNormal:
		action = "normal"
	case server.BetTypeFreeSpin:
		action = "free_spin"
	case server.BetTypeJackpot:
		action = "jackpot"
	default:
		action = string(query.Type)
	}

	// Build URL for log service API
	url := fmt.Sprintf("%s/logs/search?source_service=%s&action=%s&offset=%d&limit=%d",
		p.baseURL, query.GameCode, action, query.Page, query.Limit)

	// Add user_id for non-jackpot queries
	if query.UserID != "" && query.Type != server.BetTypeJackpot {
		url += fmt.Sprintf("&user_id=%s", query.UserID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get bet history: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("log service returned status %d", resp.StatusCode)
	}

	var result LogServiceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.IsSuccess {
		errMsg := "unknown error"
		if result.Error.ErrorMessage != "" {
			errMsg = result.Error.ErrorMessage
		}
		return nil, fmt.Errorf("log service error: %s", errMsg)
	}

	// Convert to Bet format
	bets := make([]server.Bet, 0, len(result.Data.Logs))
	for _, entry := range result.Data.Logs {
		bet := p.convertToBet(entry, query.Type)
		if bet != nil {
			bets = append(bets, *bet)
		}
	}

	return &server.BetHistoryResponse{
		Total: result.Data.Total,
		Items: bets,
	}, nil
}

// convertToBet converts a LogEntry to Bet format
func (p *LogProvider) convertToBet(entry LogEntry, betType server.BetType) *server.Bet {
	bet := &server.Bet{
		SessionID: entry.SessionID,
		Time:      entry.Timestamp,
	}

	switch betType {
	case server.BetTypeNormal, server.BetTypeFreeSpin:
		var details SpinDetails
		if err := mapstructure.Decode(entry.Details, &details); err != nil {
			p.logger.Warn().Err(err).Msg("Failed to decode spin details")
			return nil
		}
		bet.TotalBet = details.BetAmount
		bet.TotalWin = details.WinAmount
		bet.IsFreeSpin = details.SpinType == 1

		// Extract reels and winLines from spinResult if available
		if details.SpinResult != nil {
			if resultMap, ok := details.SpinResult.(map[string]interface{}); ok {
				if reels, ok := resultMap["reels"]; ok {
					bet.Reels = reels
				}
				if winLines, ok := resultMap["winlines"]; ok {
					bet.WinLines = winLines
				}
				if subReel, ok := resultMap["subReel"]; ok {
					bet.SubReel = subReel
				}
			}
		}
	case server.BetTypeJackpot:
		var details JackpotDetails
		if err := mapstructure.Decode(entry.Details, &details); err != nil {
			p.logger.Warn().Err(err).Msg("Failed to decode jackpot details")
			return nil
		}
		bet.TotalBet = details.BetAmount
		bet.TotalWin = details.WinAmount
		bet.Username = &details.Username
		bet.TotalWinJackpot = details.TotalWinJackpot
		bet.IsFreeSpin = details.SpinType == 1

		if details.SpinResult != nil {
			if resultMap, ok := details.SpinResult.(map[string]interface{}); ok {
				if reels, ok := resultMap["reels"]; ok {
					bet.Reels = reels
				}
				if winLines, ok := resultMap["winlines"]; ok {
					bet.WinLines = winLines
				}
				if subReel, ok := resultMap["subReel"]; ok {
					bet.SubReel = subReel
				}

			}
		}
	}

	return bet
}
