package utils

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"
)

var (
	ErrServiceError = fmt.Errorf("service returned error")
)

type ErrorDetail struct {
	Timestamp    string `json:"timestamp"`
	Path         string `json:"path"`
	ErrorMessage string `json:"error_message"`
}

type ErrorResponse struct {
	StatusCode int         `json:"status_code"`
	IsSuccess  bool        `json:"is_success"`
	Error      ErrorDetail `json:"error,omitempty"`
}

type InternalResponse[T any] struct {
	ErrorResponse

	Data T `json:"data,omitempty"`
}

func DoInternalRequest[T any](
	logger zerolog.Logger,
	client *http.Client,
	req *http.Request,
) (*InternalResponse[T], error) {
	respData, err := DoRequest[InternalResponse[T]](logger, client, req)
	if err != nil {
		return nil, err
	}

	if len(respData.Error.ErrorMessage) > 0 {
		logger.Error().
			Err(ErrServiceError).
			Any("error_detail", respData.Error).
			Msg("failed to call service")
		return nil, fmt.Errorf("%w: %s", ErrServiceError, respData.Error.ErrorMessage)
	}

	return respData, nil
}

func DoRequest[T any](
	logger zerolog.Logger,
	client *http.Client,
	req *http.Request,
) (*T, error) {
	resp, err := client.Do(req)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("failed to call service")
		return nil, fmt.Errorf("failed to call service: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			logger.Error().
				Err(err).
				Msg("failed to close response body")
		}
	}(resp.Body)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("failed to read response")
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusAccepted {
		logger.Error().
			Err(ErrServiceError).
			Msg("failed to call service")
		return nil, fmt.Errorf("%w: %s", ErrServiceError, string(respBody))
	}

	var respData T

	if err := json.Unmarshal(respBody, &respData); err != nil {
		logger.Error().
			Err(err).
			Msg("failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &respData, nil
}
