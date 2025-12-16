package server

import (
	"net/http"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/errors"
	"github.com/gin-gonic/gin"
)

// BaseResponse represents a standardized API response
// @Description Standard API response wrapper
type BaseResponse struct {
	// HTTP status code
	Code int `json:"code" example:"200"`
	// Response data
	Data interface{} `json:"data,omitempty"`
	// Response message
	Message string `json:"message,omitempty" example:"Success"`
}

// ErrorResponseBody represents an error response body
// @Description Error response
type ErrorResponseBody struct {
	// Error message
	Message string `json:"message" example:"Invalid request"`
	// Error code
	ErrorCode int `json:"errorCode" example:"400"`
}

// Success sends a success response
func Success(c *gin.Context, statusCode int, data interface{}) {
	c.JSON(statusCode, BaseResponse{
		Code: statusCode,
		Data: data,
	})
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
	if appErr, ok := err.(*errors.AppError); ok {
		c.JSON(statusCode, gin.H{
			"message":   appErr.Message,
			"errorCode": appErr.Code,
		})
		return
	}

	c.JSON(statusCode, gin.H{
		"message":   err.Error(),
		"errorCode": statusCode,
	})
}

// ErrorWithMessage sends an error response with a custom message
func ErrorWithMessage(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{
		"message":   message,
		"errorCode": statusCode,
	})
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

