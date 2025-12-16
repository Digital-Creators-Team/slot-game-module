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

// RegisterSwagger registers swagger UI endpoint
// Games must import their generated docs package: _ "your-game/docs"
// 
// Example usage in game's main.go:
//   import _ "git.futuregamestudio.net/fgs/backend/game-xxx/docs"
//   
//   app.RegisterSwagger(server.SwaggerInfo{
//       Title:       "Cangaceiro Warrior API",
//       Description: "Game service API documentation",
//       Version:     "1.0",
//   })
func (a *App) RegisterSwagger(info SwaggerInfo) {
	// Register swagger route
	a.engine.GET("/swagger/*any", func(c *gin.Context) {
		// Dynamic host configuration
		handler := ginSwagger.WrapHandler(
			swaggerFiles.Handler,
			ginSwagger.DefaultModelsExpandDepth(-1),
		)
		handler(c)
	})

	a.logger.Info().
		Str("path", "/swagger/index.html").
		Msg("Swagger UI registered")
}

// RegisterSwaggerWithDocs registers swagger with a custom docs handler
// Use this if you need more control over swagger configuration
func (a *App) RegisterSwaggerWithDocs(docsHandler gin.HandlerFunc) {
	a.engine.GET("/swagger/*any", docsHandler)
	a.logger.Info().
		Str("path", "/swagger/index.html").
		Msg("Swagger UI registered with custom handler")
}


