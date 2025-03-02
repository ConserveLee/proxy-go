package main

import (
	"github.com/gin-gonic/gin"
	"proxy-go/internal/api"
	"proxy-go/pkg/server"
)

func main() {
	go server.StartHeartbeatChecker()
	//设置路由
	router := gin.Default()
	api.SetupRoutes(router)

	router.Run(":8092")
}
