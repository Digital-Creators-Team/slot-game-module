package server

import (
	"git.futuregamestudio.net/be-shared/slot-game-module.git/auth"
	"git.futuregamestudio.net/be-shared/slot-game-module.git/game"
	"github.com/gin-gonic/gin"
)

// ModuleContextMiddleware creates a middleware that injects ModuleContext into the request context
// This middleware should be used after JWT middleware (if authentication is required)
// User info will only be available if JWT middleware has set user info in the context
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

		moduleCtx := game.NewModuleContext(
			user, // nil if no auth
			a.logger,
			a.stateProvider,
			a.walletProvider,
			a.rewardProvider,
			a.logProvider,
		)

		// Inject into context
		ctx := game.WithContext(c.Request.Context(), moduleCtx)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}


