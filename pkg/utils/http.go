package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/rs/zerolog"

	"github.com/Digital-Creators-Team/slot-game-module/pkg/trace"
)

var (
	ErrServiceError = fmt.Errorf("service returned error")
)

type ErrorDetail struct {
	Timestamp    string `json:"timestamp"`
	Path         string `json:"path"`
	ErrorMessage string `json:"error_message"`
	ErrorCode    int    `json:"error_code"`
}

type ErrorResponse struct {
	StatusCode int         `json:"status_code"`
	IsSuccess  bool        `json:"is_success"`
	Error      ErrorDetail `json:"error,omitempty"`
}

type InternalResponse[T any] struct {
	ErrorResponse

	// TODO: fix annotation
	Data T `json:"data,omitempty"`
}

func MakeRequest[T any](
	ctx context.Context,
	logger zerolog.Logger,
	url string,
	apiReq *T,
) (*http.Request, error) {
	var (
		req *http.Request
		err error
	)

	if apiReq != nil {
		reqBody, err := json.Marshal(apiReq)
		if err != nil {
			logger.Error().Err(err).Msg("failed to marshal request")
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		req, err = http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			logger.Error().Err(err).Msg("failed to create request")
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			logger.Error().Err(err).Msg("failed to create request")
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
	}

	traceID := trace.GetTraceID(ctx)
	if traceID != "" {
		req.Header.Add(trace.TraceIDHeader, traceID)
	}

	return req, nil
}

func DoInternalRequest[T any](
	logger zerolog.Logger,
	client *http.Client,
	req *http.Request,
) (*InternalResponse[T], error) {
	var errorResponse ErrorResponse

	rawBytes, respData, err := DoRequest[InternalResponse[T]](logger, client, req)
	if respData != nil {
		errorResponse = respData.ErrorResponse
	}
	if errors.Is(err, ErrServiceError) {
		errorData, _ := Unmarshal[InternalResponse[any]](rawBytes)
		if errorData != nil {
			if !errorData.IsSuccess || errorData.Error.ErrorMessage != "" {
				// skip the next error check to return the service error message
				err = nil
				errorResponse = errorData.ErrorResponse
			}
		}
	}
	if err != nil {
		return nil, err
	}

	if !errorResponse.IsSuccess || errorResponse.Error.ErrorMessage != "" {
		logger.Error().
			Err(ErrServiceError).
			Str("url", req.URL.String()).
			Str("error_message", errorResponse.Error.ErrorMessage).
			Int("error_code", errorResponse.Error.ErrorCode).
			Bytes("raw_response", rawBytes).
			Msg("failed to call service")

		errMsg := "unknown error"
		if errorResponse.Error.ErrorMessage != "" {
			errMsg = errorResponse.Error.ErrorMessage
		}

		return nil, fmt.Errorf("%w: %s", ErrServiceError, errMsg)
	}

	return respData, nil
}

func DoRequest[T any](
	logger zerolog.Logger,
	client *http.Client,
	req *http.Request,
) ([]byte, *T, error) {
	resp, err := client.Do(req)
	if err != nil {
		logger.Error().
			Err(err).
			Str("url", req.URL.String()).
			Msg("failed to send request")
		return nil, nil, fmt.Errorf("failed to send request: %w", err)
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
			Str("url", req.URL.String()).
			Msg("failed to read response")
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK &&
		resp.StatusCode != http.StatusCreated &&
		resp.StatusCode != http.StatusAccepted {
		logger.Error().
			Err(ErrServiceError).
			Str("url", req.URL.String()).
			Str("status", resp.Status).
			Int("status_code", resp.StatusCode).
			Bytes("raw_response", respBody).
			Msg("response status error")
		return respBody, nil, fmt.Errorf("%w: status %d", ErrServiceError, resp.StatusCode)
	}

	respData, err := Unmarshal[T](respBody)
	if err != nil {
		logger.Error().
			Err(err).
			Str("url", req.URL.String()).
			Bytes("raw_response", respBody).
			Msg("failed to unmarshal response")
		return respBody, nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return respBody, respData, nil
}

func Unmarshal[T any](rawBytes []byte) (*T, error) {
	var data T

	err := json.Unmarshal(rawBytes, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}
