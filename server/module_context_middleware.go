package server

import (
	"github.com/Digital-Creators-Team/slot-game-module/auth"
	"github.com/Digital-Creators-Team/slot-game-module/game"
	"github.com/gin-gonic/gin"
)

// ModuleContextMiddleware creates a middleware that injects ModuleContext into the request context
// This middleware should be used after JWT middleware (if authentication is required)
// User info will only be available if JWT middleware has set user info in the context
//
// If ModuleContext already exists in context, it will update the user info if available from JWT
//
// Usage:
//   games.Use(auth.JWTMiddleware(...))
//   games.Use(app.ModuleContextMiddleware())
func (a *App) ModuleContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to extract user info from JWT (may not be available if no auth middleware)
		var user *game.User
		if userID, ok := auth.GetUserID(c); ok {
			username, _ := auth.GetUsername(c)
			currencyID, _ := auth.GetCurrencyID(c)
			user = game.NewUser(userID, username, currencyID)
		}
		// user will be nil if no auth middleware or no user in token

		// Check if ModuleContext already exists (from previous middleware call)
		existingCtx := game.FromContext(c.Request.Context())
		if existingCtx != nil && user != nil {
			// Update existing ModuleContext with user info
			// Create new ModuleContext with user info (since ModuleContext is immutable)
			moduleCtx := game.NewModuleContext(
				user,
				existingCtx.Logger,
				existingCtx.GetStateProvider(),
				existingCtx.GetWalletProvider(),
				existingCtx.GetRewardProvider(),
				existingCtx.GetLogProvider(),
			)
			ctx := game.WithContext(c.Request.Context(), moduleCtx)
			c.Request = c.Request.WithContext(ctx)
		} else if existingCtx == nil {
			// Create new ModuleContext
			moduleCtx := game.NewModuleContext(
				user, // nil if no auth
				a.logger,
				a.stateProvider,
				a.walletProvider,
				a.rewardProvider,
				a.logProvider,
			)
			ctx := game.WithContext(c.Request.Context(), moduleCtx)
			c.Request = c.Request.WithContext(ctx)
		}
		// If existingCtx != nil && user == nil, keep existing context (no update needed)

		c.Next()
	}
}


