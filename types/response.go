package types

// ErrorDetail represents the error payload details
type ErrorDetail struct {
	Timestamp    string `json:"timestamp"`
	Path         string `json:"path"`
	ErrorMessage string `json:"error_message"`
}

// ErrorResponse represents the standardized error response structure
type ErrorResponse struct {
	StatusCode int         `json:"status_code"`
	IsSuccess  bool        `json:"is_success"`
	Error      ErrorDetail `json:"error,omitempty"`
}

// SuccessResponse represents the standardized success response structure
type SuccessResponse[T any] struct {
	StatusCode int `json:"status_code"`
	IsSuccess  bool `json:"is_success"`
	Data       T    `json:"data,omitempty"`
}

