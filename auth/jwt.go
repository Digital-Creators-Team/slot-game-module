package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"git.futuregamestudio.net/be-shared/slot-game-module.git/types"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

// Context keys for user information
const (
	UserIDKey     = "user_id"
	UsernameKey   = "username"
	CurrencyIDKey = "currency_id"
	ClaimsKey     = "claims"
)

// Claims represents the JWT claims structure
type Claims struct {
	UserID     string `json:"user_id"`
	Username   string `json:"username"`
	CurrencyID string `json:"currency_id,omitempty"`
	jwt.RegisteredClaims
}

// JWTConfig holds JWT middleware configuration
type JWTConfig struct {
	Secret          string
	TokenLookup     string // "header:Authorization" or "query:token"
	TokenPrefix     string // "Bearer"
	SkipPaths       []string
	DefaultCurrency string
}

// DefaultJWTConfig returns default JWT configuration
func DefaultJWTConfig(secret string) JWTConfig {
	return JWTConfig{
		Secret:          secret,
		TokenLookup:     "header:Authorization",
		TokenPrefix:     "Bearer",
		SkipPaths:       []string{"/health", "/api/health"},
		DefaultCurrency: "gold",
	}
}

// JWTMiddleware creates a JWT authentication middleware
func JWTMiddleware(secret string, logger zerolog.Logger) gin.HandlerFunc {
	return JWTMiddlewareWithConfig(DefaultJWTConfig(secret), logger)
}

// JWTMiddlewareWithConfig creates a JWT middleware with custom configuration
func JWTMiddlewareWithConfig(config JWTConfig, logger zerolog.Logger) gin.HandlerFunc {
	skipPaths := make(map[string]bool)
	for _, path := range config.SkipPaths {
		skipPaths[path] = true
	}

	return func(c *gin.Context) {
		// Skip authentication for specified paths
		if skipPaths[c.Request.URL.Path] {
			c.Next()
			return
		}

		// Extract token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			logger.Warn().Msg("Missing Authorization header")
			errorResp := types.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				IsSuccess:  false,
				Error: types.ErrorDetail{
					Timestamp:    time.Now().Format(time.RFC3339),
					Path:         c.Request.URL.Path,
					ErrorMessage: "Missing Authorization header",
				},
			}
			c.JSON(http.StatusUnauthorized, errorResp)
			c.Abort()
			return
		}

		// Check if it's a Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != config.TokenPrefix {
			logger.Warn().Str("auth_header", authHeader).Msg("Invalid Authorization header format")
			errorResp := types.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				IsSuccess:  false,
				Error: types.ErrorDetail{
					Timestamp:    time.Now().Format(time.RFC3339),
					Path:         c.Request.URL.Path,
					ErrorMessage: "Invalid Authorization header format. Expected: Bearer <token>",
				},
			}
			c.JSON(http.StatusUnauthorized, errorResp)
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Parse and validate token
		token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
			// Validate signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(config.Secret), nil
		})

		if err != nil {
			logger.Warn().Err(err).Msg("Failed to parse JWT token")
			errorResp := types.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				IsSuccess:  false,
				Error: types.ErrorDetail{
					Timestamp:    time.Now().Format(time.RFC3339),
					Path:         c.Request.URL.Path,
					ErrorMessage: "Invalid or expired token",
				},
			}
			c.JSON(http.StatusUnauthorized, errorResp)
			c.Abort()
			return
		}

		// Extract claims
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			logger.Warn().Msg("Invalid token claims")
			errorResp := types.ErrorResponse{
				StatusCode: http.StatusUnauthorized,
				IsSuccess:  false,
				Error: types.ErrorDetail{
					Timestamp:    time.Now().Format(time.RFC3339),
					Path:         c.Request.URL.Path,
					ErrorMessage: "Invalid token claims",
				},
			}
			c.JSON(http.StatusUnauthorized, errorResp)
			c.Abort()
			return
		}

		// Store user information in context
		c.Set(UserIDKey, claims.UserID)
		c.Set(UsernameKey, claims.Username)
		c.Set(ClaimsKey, claims)

		// Set currency ID (use default if not in claims)
		currencyID := claims.CurrencyID
		if currencyID == "" {
			currencyID = config.DefaultCurrency
		}
		c.Set(CurrencyIDKey, currencyID)

		logger.Debug().
			Str("user_id", claims.UserID).
			Str("username", claims.Username).
			Msg("JWT authentication successful")

		c.Next()
	}
}

// GetUserID extracts user ID from context
func GetUserID(c *gin.Context) (string, bool) {
	userID, exists := c.Get(UserIDKey)
	if !exists {
		return "", false
	}
	userIDStr, ok := userID.(string)
	return userIDStr, ok
}

// GetUsername extracts username from context
func GetUsername(c *gin.Context) (string, bool) {
	username, exists := c.Get(UsernameKey)
	if !exists {
		return "", false
	}
	usernameStr, ok := username.(string)
	return usernameStr, ok
}

// GetCurrencyID extracts currency ID from context
func GetCurrencyID(c *gin.Context) (string, bool) {
	currencyID, exists := c.Get(CurrencyIDKey)
	if !exists {
		return "", false
	}
	currencyIDStr, ok := currencyID.(string)
	return currencyIDStr, ok
}

// GetClaims extracts full claims from context
func GetClaims(c *gin.Context) (*Claims, bool) {
	claims, exists := c.Get(ClaimsKey)
	if !exists {
		return nil, false
	}
	claimsObj, ok := claims.(*Claims)
	return claimsObj, ok
}

// GenerateToken generates a new JWT token
func GenerateToken(secret string, userID, username string, expiration time.Duration) (string, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateTokenWithCurrency generates a new JWT token with currency
func GenerateTokenWithCurrency(secret string, userID, username, currencyID string, expiration time.Duration) (string, error) {
	claims := &Claims{
		UserID:     userID,
		Username:   username,
		CurrencyID: currencyID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(expiration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
