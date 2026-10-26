package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"

	"github.com/Digital-Creators-Team/slot-game-module/config"
	"github.com/Digital-Creators-Team/slot-game-module/logging"
	"github.com/Digital-Creators-Team/slot-game-module/pkg/utils"
	"github.com/Digital-Creators-Team/slot-game-module/server"
)

// authProvider implements server.AuthProvider using HTTP client
type authProvider struct {
	baseURL    string
	httpClient *http.Client
	logger     logging.LoggerProvider
}

// NewAuthProvider creates a new auth provider
func NewAuthProvider(
	cfg *config.Config,
	logger zerolog.Logger,
) server.AuthProvider {
	authConfig := cfg.ExternalServices.AuthService
	timeout := authConfig.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	p := &authProvider{
		baseURL: authConfig.BaseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logging.NewLoggerProvider(logger.With().
			Str("component", "auth_provider").
			Logger(),
		),
	}

	return p
}

func (p *authProvider) VerifyExternalToken(
	ctx context.Context,
	gameCode string,
	tenant *server.ResponseTenant,
	token string,
) (*server.VerifyExternalTokenResponse, error) {
	switch tenant.WalletVersion {
	case 3:
		respV3, err := p.AuthV3(ctx, gameCode, tenant.ID, token)
		if err != nil {
			return nil, err
		}

		return respV3, nil
	default:
		return nil, server.ErrInvalidWalletAPIVersion
	}
}

type VerifyExternalTokenV3Request struct {
	TenantID string `json:"tenant_id" binding:"required"`
	GameCode string `json:"game_code" binding:"required"`
	ExToken  string `json:"ex_token" form:"ex-token" binding:"required"`
}

func (p *authProvider) AuthV3(
	ctx context.Context,
	gameCode string,
	tenantID string,
	token string,
) (*server.VerifyExternalTokenResponse, error) {
	url := fmt.Sprintf("%s/api/v3/user/verify-external-token", p.baseURL)

	req, err := utils.MakeRequest(ctx, p.logger.For(ctx), url, &VerifyExternalTokenV3Request{
		TenantID: tenantID,
		GameCode: gameCode,
		ExToken:  token,
	})
	if err != nil {
		return nil, err
	}

	respData, err := utils.DoInternalRequest[server.VerifyExternalTokenResponse](p.logger.For(ctx), p.httpClient, req)
	if err != nil {
		return nil, err
	}

	return &respData.Data, nil
}
