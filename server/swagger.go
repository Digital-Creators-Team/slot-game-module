package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// SwaggerInfo holds swagger metadata that games can configure
type SwaggerInfo struct {
	Title       string
	Description string
	Version     string
	Host        string
	BasePath    string
}

// SwaggerHostUpdater is a function type to update SwaggerInfo.Host at runtime
// Games should pass a function that updates docs.SwaggerInfo.Host
// Example: func(host string) { docs.SwaggerInfo.Host = host }
type SwaggerHostUpdater func(host string)

// RegisterSwagger registers swagger UI endpoint with dynamic host from request
// Games must import their generated docs package: _ "your-game/docs"
//
// Example usage in game's main.go:
//
//	import (
//	    _ "git.futuregamestudio.net/fgs/backend/game-xxx/docs"
//	    "git.futuregamestudio.net/fgs/backend/game-xxx/docs"
//	)
//
//	app.RegisterSwagger(server.SwaggerInfo{
//	    Title:       "Cangaceiro Warrior API",
//	    Description: "Game service API documentation",
//	    Version:     "1.0",
//	}, func(host string) {
//	    docs.SwaggerInfo.Host = host
//	})
func (a *App) RegisterSwagger(info SwaggerInfo, hostUpdater SwaggerHostUpdater) {
	// Register swagger route with dynamic host
	a.engine.GET("/swagger/*any", func(c *gin.Context) {
		// Get host from request (supports X-Forwarded-Host for reverse proxy)
		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}

		// Update SwaggerInfo.Host at runtime
		if hostUpdater != nil {
			hostUpdater(host)
		}

		// Create handler
		handler := ginSwagger.WrapHandler(
			swaggerFiles.Handler,
			ginSwagger.DefaultModelsExpandDepth(-1),
		)
		handler(c)
	})

	a.logger.Info().
		Str("path", "/swagger/index.html").
		Msg("Swagger UI registered with dynamic host")
}

// RegisterSwaggerWithDocs registers swagger with a custom docs handler
// Use this if you need more control over swagger configuration
func (a *App) RegisterSwaggerWithDocs(docsHandler gin.HandlerFunc) {
	a.engine.GET("/swagger/*any", docsHandler)
	a.logger.Info().
		Str("path", "/swagger/index.html").
		Msg("Swagger UI registered with custom handler")
}
