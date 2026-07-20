package errors

import (
	"fmt"
	"os"
)

// Standard error codes
const (
	ErrInvalidRequest      = 400
	ErrUnauthorized        = 401
	ErrForbidden           = 403
	ErrNotFound            = 404
	ErrConflict            = 409
	ErrInternalServerError = 500
	ErrServiceUnavailable  = 503

	// Game-specific error codes (1000+)
	ErrInsufficientBalance = 1001
	ErrGameModuleNotFound  = 1002
	ErrPlayerStateError    = 1003
	ErrWalletError         = 1004
	ErrRewardError         = 1005
	ErrKafkaError          = 1006
	ErrRedisError          = 1007
	ErrConfigError         = 1008
	ErrGameLogicError      = 1009
)

// AppError represents a custom application error
type AppError struct {
	Code         int    `json:"code"`
	Message      string `json:"message"`
	DebugMessage string `json:"debug_message,omitempty"`
	Err          error  `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.DebugMessage != "" {
		return fmt.Sprintf("[%d] %s: %s", e.Code, e.Message, e.DebugMessage)
	}
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s [%v]", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error
func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError
func New(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// NewWithDebug creates a new AppError with a debug message
func NewWithDebug(code int, message string, debugMessage string) *AppError {
	return &AppError{
		Code:         code,
		Message:      message,
		DebugMessage: debugMessage,
	}
}

// Wrap wraps an existing error into an AppError
func Wrap(err error, code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// WrapWithDebug wraps an existing error into an AppError with a debug message
func WrapWithDebug(err error, code int, message string, debugMessage string) *AppError {
	return &AppError{
		Code:         code,
		Message:      message,
		DebugMessage: debugMessage,
		Err:          err,
	}
}

// Response returns a map suitable for JSON response
func (e *AppError) Response() map[string]interface{} {
	response := map[string]interface{}{
		"code":    e.Code,
		"message": e.Message,
	}

	// Include debug message in development environment
	env := os.Getenv("APP_ENV")
	if (env == "dev" || env == "development") && e.DebugMessage != "" {
		response["debug_message"] = e.DebugMessage
	}

	return response
}

// IsAppError checks if an error is an AppError
func IsAppError(err error) bool {
	_, ok := err.(*AppError)
	return ok
}

// GetCode extracts error code from an error
func GetCode(err error) int {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Code
	}
	switch err.Error() {
	case "insufficient funds":
		return ErrInsufficientBalance
	default:
		return ErrInternalServerError
	}
}

// HTTPStatusFromCode maps error codes to HTTP status codes
func HTTPStatusFromCode(code int) int {
	switch code {
	case ErrInvalidRequest:
		return 400
	case ErrUnauthorized:
		return 401
	case ErrForbidden:
		return 403
	case ErrNotFound:
		return 404
	case ErrConflict:
		return 409
	case ErrInternalServerError:
		return 500
	case ErrServiceUnavailable:
		return 503
	case ErrInsufficientBalance:
		return 400
	case ErrGameModuleNotFound:
		return 404
	case ErrPlayerStateError:
		return 500
	case ErrWalletError:
		return 502
	case ErrRewardError:
		return 502
	default:
		return 500
	}
}
