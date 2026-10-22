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

func (p *authProvider) ClaimUserTenantToken(ctx context.Context, userID string) (string, error) {
	url := fmt.Sprintf("%s/api/v2/user/tenant-token/%s/claim", p.baseURL, userID)

	req, err := utils.MakeRequest[any](ctx, p.logger.For(ctx), url, nil)
	if err != nil {
		return "", err
	}

	respData, err := utils.DoInternalRequest[string](p.logger.For(ctx), p.httpClient, req)
	if err != nil {
		return "", err
	}

	return respData.Data, nil
}
