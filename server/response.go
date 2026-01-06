package server

import (
	"net/http"
	"time"

	"github.com/Digital-Creators-Team/slot-game-module/errors"
	"github.com/Digital-Creators-Team/slot-game-module/types"
	"github.com/gin-gonic/gin"
)

const ErrUndefinedErrorCode = -99

// ErrorDetail is an alias for types.ErrorDetail
// @Description Error payload details
type ErrorDetail = types.ErrorDetail

// ErrorResponse is an alias for types.ErrorResponse
// @Description Standardized error response
type ErrorResponse = types.ErrorResponse

// SuccessResponse is a type alias for types.SuccessResponse[T]
// @Description Standardized success response
type SuccessResponse[T any] = types.SuccessResponse[T]

// BaseResponse is a type alias for SuccessResponse[interface{}] for backward compatibility with swagger
// @Description Standard API response wrapper
type BaseResponse = SuccessResponse[interface{}]

// Success sends a success response
func Success(c *gin.Context, statusCode int, data interface{}) {
	successResp := types.SuccessResponse[interface{}]{
		StatusCode: statusCode,
		IsSuccess:  true,
		Data:       data,
	}
	c.JSON(statusCode, successResp)
}

// OK sends a 200 OK response
func OK(c *gin.Context, data interface{}) {
	Success(c, http.StatusOK, data)
}

// Created sends a 201 Created response
func Created(c *gin.Context, data interface{}) {
	Success(c, http.StatusCreated, data)
}

// NoContent sends a 204 No Content response
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Error sends an error response
func Error(c *gin.Context, statusCode int, err error) {
	var errorMsg string
	var errCode int
	if appErr, ok := err.(*errors.AppError); ok {
		errorMsg = appErr.Message
		errCode = appErr.Code
	} else {
		errorMsg = err.Error()
		errCode = ErrUndefinedErrorCode
	}

	errorResp := types.ErrorResponse{
		StatusCode: statusCode,
		IsSuccess:  false,
		Error: types.ErrorDetail{
			Timestamp:    time.Now().Format(time.RFC3339),
			Path:         c.Request.URL.Path,
			ErrorMessage: errorMsg,
			ErrorCode:    errCode,
		},
	}
	c.JSON(statusCode, errorResp)
}

// ErrorWithMessage sends an error response with a custom message
func ErrorWithMessage(c *gin.Context, statusCode int, message string) {
	errorResp := types.ErrorResponse{
		StatusCode: statusCode,
		IsSuccess:  false,
		Error: types.ErrorDetail{
			Timestamp:    time.Now().Format(time.RFC3339),
			Path:         c.Request.URL.Path,
			ErrorMessage: message,
			ErrorCode:    ErrUndefinedErrorCode,
		},
	}
	c.JSON(statusCode, errorResp)
}

// BadRequest sends a 400 Bad Request response
func BadRequest(c *gin.Context, err error) {
	Error(c, http.StatusBadRequest, err)
}

// Unauthorized sends a 401 Unauthorized response
func Unauthorized(c *gin.Context, err error) {
	Error(c, http.StatusUnauthorized, err)
}

// Forbidden sends a 403 Forbidden response
func Forbidden(c *gin.Context, err error) {
	Error(c, http.StatusForbidden, err)
}

// NotFound sends a 404 Not Found response
func NotFound(c *gin.Context, err error) {
	Error(c, http.StatusNotFound, err)
}

// InternalError sends a 500 Internal Server Error response
func InternalError(c *gin.Context, err error) {
	Error(c, http.StatusInternalServerError, err)
}

// ServiceUnavailable sends a 503 Service Unavailable response
func ServiceUnavailable(c *gin.Context, err error) {
	Error(c, http.StatusServiceUnavailable, err)
}

// HandleAppError handles AppError and sends appropriate response
func HandleAppError(c *gin.Context, err error) {
	if appErr, ok := err.(*errors.AppError); ok {
		statusCode := errors.HTTPStatusFromCode(appErr.Code)
		Error(c, statusCode, appErr)
	} else {
		InternalError(c, err)
	}
}
