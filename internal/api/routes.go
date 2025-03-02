package api

import (
	"github.com/gin-gonic/gin"
	"proxy-go/internal/api/handlers"
)

func SetupRoutes(router *gin.Engine) {
	// API路由
	api := router.Group("/api/clients")
	{
		api.GET("", handlers.ListClients)
		api.POST("/create", handlers.CreateClient)
		api.DELETE("/:id", handlers.DeleteClient)
		api.POST("/:id/toggle", handlers.ToggleClient)
	}
	router.GET("/api/heartbeats", handlers.ListHeartbeats)

	// 静态文件服务
	router.Static("/static", "./static")
	router.GET("/admin", func(c *gin.Context) {
		c.File("./static/index.html")
	})
}
