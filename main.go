package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Config struct {
	ProxyTarget  string `json:"proxy_target"`
	HeartbeatURL string `json:"heartbeat_url"`
	ProxyEnabled bool   `json:"proxy_enabled"`
	HeartbeatOK  bool   `json:"heartbeat_ok"`
}

var (
	configLock sync.RWMutex
	appConfig  = Config{
		ProxyTarget:  "http://192.168.50.211:10086",
		HeartbeatURL: "http://example.com",
		ProxyEnabled: true,
		HeartbeatOK:  false,
	}
)

func main() {
	go startHeartbeatChecker()

	router := gin.Default()

	// API路由
	api := router.Group("/api")
	{
		api.GET("/config", getConfig)
		api.PUT("/config", updateConfig)
		api.POST("/toggle", toggleProxy)
		api.GET("/status", getStatus)
	}

	// 静态文件服务
	router.Static("/static", "./static")
	router.NoRoute(func(c *gin.Context) {
		c.File("./static/index.html")
	})

	// 反向代理路由
	router.GET("/test", reverseProxyHandler)

	router.Run(":8092")
}

func reverseProxyHandler(c *gin.Context) {
	configLock.RLock()
	defer configLock.RUnlock()

	if !appConfig.ProxyEnabled {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable,
			gin.H{"error": "Proxy service is disabled"})
		return
	}

	target, _ := url.Parse(appConfig.ProxyTarget)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// 修改请求头
	c.Request.URL.Host = target.Host
	c.Request.URL.Scheme = target.Scheme
	c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))
	c.Request.Host = target.Host

	proxy.ModifyResponse = func(resp *http.Response) error {
		log.Printf("代理响应状态: %d 目标地址: %s", resp.StatusCode, target.String())
		return nil
	}

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("代理错误: %v", err)
		w.WriteHeader(http.StatusBadGateway)
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

func startHeartbeatChecker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		checkHeartbeat()
	}
}

func checkHeartbeat() {
	configLock.RLock()
	checkURL := appConfig.HeartbeatURL
	configLock.RUnlock()

	resp, err := http.Get(checkURL)
	if err != nil {
		updateHeartbeatStatus(false)
		return
	}
	defer resp.Body.Close()

	updateHeartbeatStatus(resp.StatusCode == http.StatusOK)
}

func updateHeartbeatStatus(ok bool) {
	configLock.Lock()
	defer configLock.Unlock()
	appConfig.HeartbeatOK = ok
}

// API处理函数
func getConfig(c *gin.Context) {
	configLock.RLock()
	defer configLock.RUnlock()
	c.JSON(http.StatusOK, appConfig)
}

func updateConfig(c *gin.Context) {
	var newConfig Config
	if err := c.ShouldBindJSON(&newConfig); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	configLock.Lock()
	appConfig.ProxyTarget = newConfig.ProxyTarget
	appConfig.HeartbeatURL = newConfig.HeartbeatURL
	configLock.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "configuration updated"})
}

func toggleProxy(c *gin.Context) {
	configLock.Lock()
	appConfig.ProxyEnabled = !appConfig.ProxyEnabled
	configLock.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"proxy_enabled": appConfig.ProxyEnabled,
	})
}

func getStatus(c *gin.Context) {
	configLock.RLock()
	defer configLock.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"proxy_enabled": appConfig.ProxyEnabled,
		"heartbeat_ok":  appConfig.HeartbeatOK,
	})
}
