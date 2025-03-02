package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net"
	"net/http"
	t "proxy-go/pkg/client"
	"sync"
	"time"
)

var (
	app, _      = t.AppConfig()
	clientsLock sync.RWMutex
)

// @Summary 获取所有客户端配置
// @Produce json
// @Success 200 {array} Config
// @Router /api/clients [get]

func ListClients(c *gin.Context) {
	clientsLock.RLock()
	defer clientsLock.RUnlock()

	clientList := app.ClientsList()

	c.JSON(http.StatusOK, clientList)
}

func CreateClient(c *gin.Context) {
	var newClient struct {
		Name        string `json:"name" binding:"required"`
		ProxyTarget string `json:"proxy_target" binding:"required,url"`
	}

	if err := c.ShouldBindJSON(&newClient); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取客户端真实IP（支持代理）
	clientIP := c.ClientIP()
	if clientIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取客户端IP"})
		return
	}

	// 验证IP格式
	if parsedIP := net.ParseIP(clientIP); parsedIP == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的客户端IP格式"})
		return
	}

	//验证IP是否已存在
	if _, exists := app.IpMap[clientIP]; exists {
		c.JSON(http.StatusConflict, gin.H{"error": "IP已存在"})
		return
	}

	app.Mu.Lock()
	defer app.Mu.Unlock()

	// 创建客户端对象
	client := &t.Client{
		ID:          app.NextId,
		Name:        newClient.Name,
		IP:          clientIP, // 使用获取的IP
		ProxyTarget: newClient.ProxyTarget,
		Enabled:     true,
	}

	clientID := fmt.Sprintf("client%d", client.ID)

	// 检查冲突
	if _, exists := app.Clients[clientID]; exists {
		c.JSON(http.StatusConflict, gin.H{"error": "ID冲突"})
		return
	}

	// 保存配置
	app.Clients[clientID] = client
	if err := app.Save(); err != nil {
		app.NextId++
		app.IpMap[clientIP] = newClient.ProxyTarget
		c.JSON(http.StatusInternalServerError, gin.H{"error": "配置保存失败"})
		return
	}

	c.JSON(http.StatusCreated, client)
}

// @Summary 删除客户端配置
// @Param id path string true "客户端ID"
// @Success 204
// @Failure 404 {object} map[string]string
// @Router /api/clients/{id} [delete]

func DeleteClient(c *gin.Context) {
	clientID := c.Param("id")

	clientsLock.Lock()
	defer clientsLock.Unlock()

	// 获取客户端真实IP（支持代理）
	clientIP := c.ClientIP()
	if clientIP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法获取客户端IP"})
		return
	}

	// 验证IP格式
	if parsedIP := net.ParseIP(clientIP); parsedIP == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的客户端IP格式"})
		return
	}

	if app.IsExist(clientID) {
		delete(app.Clients, "client"+clientID)
		delete(app.IpMap, clientIP)
		go func() {
			end := time.After(time.Second * 15)
			done := make(chan struct{}, 1)
			go func() {
				_ = app.Save()
				close(done)
			}()
			select {
			case <-done:
			case <-end:
				log.Println("保存操作超时终止")
			}
		}()
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
	return

}

// @Summary 切换代理启用状态
// @Param id path string true "客户端ID"
// @Success 200 {object} map[string]bool
// @Failure 404 {object} map[string]string
// @Router /api/clients/{id}/toggle [post]

func ToggleClient(c *gin.Context) {
	clientID := c.Param("id")

	clientsLock.Lock()
	defer clientsLock.Unlock()

	if !app.IsExist(clientID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
		return
	}

	clientKey := "client" + clientID
	state := !app.Clients[clientKey].Enabled
	app.Clients[clientKey].Enabled = state
	go func() { //todo 控制这个goroutine的退出
		_ = app.Save()
	}()
	c.JSON(http.StatusOK, gin.H{"enabled": state})
}
