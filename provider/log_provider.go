package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/Digital-Creators-Team/slot-game-module/events/kafka"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
	"github.com/Digital-Creators-Team/slot-game-module/types"
	"github.com/mitchellh/mapstructure"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

// SpinDetails represents spin log details for mapstructure decoding
type SpinDetails struct {
	SessionID         string      `mapstructure:"sessionId" json:"sessionId"`
	Username          string      `mapstructure:"username" json:"username"`
	GameCode          string      `mapstructure:"gameCode" json:"gameCode"`
	BetAmount         float64     `mapstructure:"betAmount" json:"betAmount"`
	WinAmount         float64     `mapstructure:"winAmount" json:"winAmount"`
	Currency          string      `mapstructure:"currency" json:"currency"`
	SpinType          int         `mapstructure:"spinType" json:"spinType"`
	SpinResult        interface{} `mapstructure:"spinResult" json:"spinResult"`
	SplitRoundHistory bool        `mapstructure:"splitRoundHistory" json:"splitRoundHistory"`
}

// RoundDetails represents round log details for mapstructure decoding
type RoundDetails struct {
	// data from parent bet
	SessionID string `mapstructure:"sessionId" json:"sessionId"`
	Username  string `mapstructure:"username" json:"username"`
	GameCode  string `mapstructure:"gameCode" json:"gameCode"`
	Currency  string `mapstructure:"currency" json:"currency"`
	SpinType  int    `mapstructure:"spinType" json:"spinType"`
	SubReel   any    `mapstructure:"subReel" json:"subReel,omitempty"`

	RoundIndex  int `mapstructure:"roundIndex" json:"roundIndex"`
	RoundResult any `mapstructure:"roundResult" json:"roundResult"`
}

// SpinErrorDetails represents spin error details for mapstructure decoding
type SpinErrorDetails struct {
	SessionID  string          `mapstructure:"sessionId" json:"sessionId"`
	Username   string          `mapstructure:"username" json:"username"`
	GameCode   string          `mapstructure:"gameCode" json:"gameCode"`
	BetAmount  decimal.Decimal `mapstructure:"betAmount" json:"betAmount"`
	WinAmount  decimal.Decimal `mapstructure:"winAmount" json:"winAmount"`
	Currency   string          `mapstructure:"currency" json:"currency"`
	SpinType   int             `mapstructure:"spinType" json:"spinType"`
	Status     string          `mapstructure:"status" json:"status"`
	Error      string          `mapstructure:"error" json:"error"`
	SpinResult interface{}     `mapstructure:"spinResult" json:"spinResult"`
}

// JackpotDetails represents jackpot log details for mapstructure decoding
type JackpotDetails struct {
	SessionID       string  `mapstructure:"sessionId" json:"sessionId"`
	Username        string  `mapstructure:"username" json:"username"`
	Name            string  `mapstructure:"name" json:"name"`
	GameCode        string  `mapstructure:"gameCode" json:"gameCode"`
	Tier            string  `mapstructure:"tier" json:"tier"`
	BetAmount       float64 `mapstructure:"betAmount" json:"betAmount"`
	WinAmount       float64 `mapstructure:"winAmount" json:"winAmount"`
	TotalWinJackpot float64 `mapstructure:"totalWinJackpot" json:"totalWinJackpot"`
	Currency        string  `mapstructure:"currency" json:"currency"`
	SpinType        int     `mapstructure:"spinType" json:"spinType"`
	//SpinResult      interface{} `mapstructure:"spinResult" json:"spinResult"`
}

type EventListDetails struct {
	ActionList  []string      `mapstructure:"actionList" json:"actionList"`
	DetailsList []interface{} `mapstructure:"detailsList" json:"detailsList"`
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
	TenantID      string      `json:"tenant_id"`
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
	if p.kafkaProducer == nil {
		p.logger.Warn().Msg("Kafka producer not configured, skipping spin log")
		return log.SessionID, nil
	}

	var (
		action = "normal" // Default action for spin
		event  = AuditEvent{
			Timestamp:     log.Timestamp,
			TenantID:      log.TenantID,
			UserID:        log.UserID,
			SessionID:     log.SessionID,
			SourceService: log.GameCode,
			Details: SpinDetails{
				SessionID:         log.SessionID,
				Username:          log.Username,
				GameCode:          log.GameCode,
				BetAmount:         log.BetAmount,
				WinAmount:         log.WinAmount,
				Currency:          log.Currency,
				SpinType:          log.SpinType,
				SpinResult:        log.SpinResult,
				SplitRoundHistory: log.SplitRoundHistory,
			},
			Result:  "success",
			TraceID: log.SessionID,
		}
	)

	// Set action based on spin type
	// if log.SpinType == 1 {
	// 	action = "free_spin"
	// }

	if log.SplitRoundHistory && len(log.Rounds) > 1 {
		eventListDetails := &EventListDetails{
			ActionList:  make([]string, len(log.Rounds)),
			DetailsList: make([]interface{}, len(log.Rounds)),
		}

		// set first action to have the full spin details
		eventListDetails.ActionList[0] = action
		eventListDetails.DetailsList[0] = event.Details

		// subsequent actions are round details
		for i, round := range log.Rounds {
			if i == 0 {
				continue
			}

			eventListDetails.ActionList[i] = action + "_round"
			eventListDetails.DetailsList[i] = &RoundDetails{
				SessionID:   log.SessionID,
				Username:    log.Username,
				GameCode:    log.GameCode,
				Currency:    log.Currency,
				SpinType:    log.SpinType,
				SubReel:     log.SubReel,
				RoundIndex:  i,
				RoundResult: round,
			}
		}

		event.Action = "event_list"
		event.Details = eventListDetails
	} else {
		event.Action = action
	}

	if err := p.kafkaProducer.SendMessage(p.auditTopic, log.SessionID, event); err != nil {
		p.logger.Error().Err(err).Msg("Failed to send spin log to Kafka")
		return "", fmt.Errorf("failed to log spin: %w", err)
	}

	return log.SessionID, nil
}

func (p *LogProvider) LogSpinError(ctx context.Context, log *server.SpinErrorLog) (sessionID string, err error) {
	if p.kafkaProducer == nil {
		p.logger.Warn().Msg("Kafka producer not configured, skipping spin log")
		return log.SessionID, nil
	}

	event := AuditEvent{
		Timestamp:     log.Timestamp,
		TenantID:      log.TenantID,
		UserID:        log.UserID,
		SessionID:     log.SessionID,
		SourceService: log.GameCode,
		Action:        "spin_error",
		Details: SpinErrorDetails{
			SessionID:  log.SessionID,
			Username:   log.Username,
			GameCode:   log.GameCode,
			BetAmount:  log.BetAmount,
			WinAmount:  log.WinAmount,
			Currency:   log.Currency,
			SpinType:   log.SpinType,
			Status:     log.Status,
			Error:      log.Error,
			SpinResult: log.SpinResult,
		},
		Result:  "success",
		TraceID: log.SessionID,
	}

	if err := p.kafkaProducer.SendMessage(p.auditTopic, log.SessionID, event); err != nil {
		p.logger.Error().Err(err).Msg("Failed to send spin error log to Kafka")
		return "", fmt.Errorf("failed to log spin error: %w", err)
	}

	return log.SessionID, nil
}

// LogJackpot logs a jackpot win event and returns sessionID
func (p *LogProvider) LogJackpot(ctx context.Context, log *server.JackpotLog) (string, error) {
	if p.kafkaProducer == nil {
		p.logger.Warn().Msg("Kafka producer not configured, skipping jackpot log")
		return log.SessionID, nil
	}

	event := AuditEvent{
		Timestamp:     log.Timestamp,
		TenantID:      log.TenantID,
		UserID:        log.UserID,
		SessionID:     log.SessionID,
		SourceService: log.GameCode,
		Action:        "jackpot",
		Details: JackpotDetails{
			SessionID:       log.SessionID,
			Username:        log.Username,
			Name:            log.Name,
			GameCode:        log.GameCode,
			Tier:            log.Tier,
			BetAmount:       log.BetAmount,
			WinAmount:       log.WinAmount,
			Currency:        log.Currency,
			SpinType:        log.SpinType,
			TotalWinJackpot: log.TotalWinJackpot,
			//SpinResult:      log.SpinResult,
		},
		Result:  "success",
		TraceID: log.SessionID,
	}

	if err := p.kafkaProducer.SendMessage(p.auditTopic, log.SessionID, event); err != nil {
		p.logger.Error().Err(err).Msg("Failed to send jackpot log to Kafka")
		return "", fmt.Errorf("failed to log jackpot: %w", err)
	}

	return log.SessionID, nil
}

// LogEntry represents an audit log entry from the log service
type LogEntry struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	TenantID      string                 `json:"tenant_id"`
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

	if query.Type == server.BetTypeJackpot {
		url += fmt.Sprintf("&tenant_id=%v", query.TenantID)
	}

	// Add round filter
	if query.Type == server.BetTypeNormal || query.Type == server.BetTypeFreeSpin {
		url += fmt.Sprintf("&action=%s_round", action)
	}

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

		if bet != nil && bet.Rounds != nil && len(bet.Rounds) > 0 && bet.SplitRoundHistory {
			b := p.convertToBetEachRound(*bet, bet.Rounds[0])
			b.Round = 0
			bets = append(bets, *b)

			// for idx, round := range bet.Rounds {
			// 	b := p.convertToBetEachRound(*bet, round)

			// 	// Index round
			// 	b.Round = idx

			// 	// Sort by time
			// 	// b.Time = b.Time.Add(time.Duration(idx*1000) * time.Microsecond)

			// 	bets = append(bets, *b)
			// }
		} else if bet != nil {
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
		TenantID:  &entry.TenantID,
		SessionID: entry.SessionID,
		Time:      entry.Timestamp,
	}

	switch betType {
	case server.BetTypeNormal, server.BetTypeFreeSpin:
		var resultMap map[string]interface{}

		if strings.HasSuffix(entry.Action, "_round") {
			var details RoundDetails
			if err := mapstructure.Decode(entry.Details, &details); err != nil {
				p.logger.Warn().Err(err).Msg("Failed to decode round details")
				return nil
			}

			bet.Round = details.RoundIndex
			bet.IsFreeSpin = details.SpinType == 1
			bet.SpinType = details.SpinType
			bet.Currency = details.Currency
			bet.SubReel = details.SubReel

			// Extract reels and winLines from roundResult if available
			if details.RoundResult != nil {
				var ok bool
				resultMap, ok = details.RoundResult.(map[string]interface{})
				if !ok {
					p.logger.Error().Msg("Failed to decode round result map")
					resultMap = nil
				} else {
					bet.TotalBet, _ = resultMap["totalBet"].(float64)
					bet.TotalWin, _ = resultMap["totalWin"].(float64)
				}
			}
		} else {
			var details SpinDetails
			if err := mapstructure.Decode(entry.Details, &details); err != nil {
				p.logger.Warn().Err(err).Msg("Failed to decode spin details")
				return nil
			}

			bet.TotalBet = details.BetAmount
			bet.TotalWin = details.WinAmount
			bet.IsFreeSpin = details.SpinType == 1
			bet.SpinType = details.SpinType
			bet.Currency = details.Currency
			bet.SplitRoundHistory = details.SplitRoundHistory

			// Extract reels and winLines from spinResult if available
			if details.SpinResult != nil {
				var ok bool
				resultMap, ok = details.SpinResult.(map[string]interface{})
				if !ok {
					p.logger.Error().Msg("Failed to decode round result map")
					resultMap = nil
				}
			}
		}

		if resultMap != nil {
			if reels, ok := resultMap["reels"]; ok {
				bet.Reels = reels
			}

			if winLines, ok := resultMap["winlines"]; ok {
				bet.WinLines = winLines
			}

			if subReel, ok := resultMap["subReel"]; ok {
				bet.SubReel = subReel
			}

			bet.IsJackpot = false
			if isJackpot, ok := resultMap["isGetJackpot"]; ok {
				bet.IsJackpot, _ = isJackpot.(bool)
			}

			if extra, ok := resultMap["extraData"]; ok {
				bet.ExtraData = extra
			}

			if rounds, ok := resultMap["rounds"]; ok {
				bytes, err := json.Marshal(rounds)
				if err == nil {
					var r []server.GameRound
					if err := json.Unmarshal(bytes, &r); err == nil {
						bet.Rounds = r
					}
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
		//bet.Username = &details.Username
		bet.Username = &details.Name
		bet.Name = &details.Name
		bet.TotalWinJackpot = details.TotalWinJackpot
		bet.IsFreeSpin = details.SpinType == 1
		bet.SpinType = details.SpinType
		bet.Currency = details.Currency
		bet.JackpotType = &details.Tier
		bet.IsJackpot = true

		//if details.SpinResult != nil {
		//	if resultMap, ok := details.SpinResult.(map[string]interface{}); ok {
		//		if reels, ok := resultMap["reels"]; ok {
		//			bet.Reels = reels
		//		}
		//		if winLines, ok := resultMap["winlines"]; ok {
		//			bet.WinLines = winLines
		//		}
		//		if subReel, ok := resultMap["subReel"]; ok {
		//			bet.SubReel = subReel
		//		}
		//
		//	}
		//}
	}

	return bet
}

func (p *LogProvider) convertToBetEachRound(entry server.Bet, round server.GameRound) *server.Bet {
	bet := entry

	// Already split rounds into separate bet
	bet.Rounds = nil

	bet.TotalWin = round.TotalWin
	bet.IsJackpot = round.IsGetJackpot
	bet.WinLines = round.Winlines

	// More data for each round here
	if round.Reels != nil {
		bet.Reels = round.Reels
		bet.TotalBet = round.TotalBet
	}

	if round.ExtraData != nil {
		bet.ExtraData = round.ExtraData
	}

	return &bet
}

type SpinWrapper struct {
	Value any
}

func (s SpinWrapper) MarshalJSON() ([]byte, error) {
	b, _ := json.Marshal(s.Value)

	var m map[string]json.RawMessage
	err := json.Unmarshal(b, &m)
	if err != nil {
		return nil, err
	}

	rv := reflect.ValueOf(s.Value)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}

	field := rv.FieldByName("ExtraData")
	if field.IsValid() {
		b2, err := utils.MarshalForceFloat(field.Interface())
		if err != nil {
			return nil, err
		}

		m["extraData"] = b2
	}

	return json.Marshal(m)
}
