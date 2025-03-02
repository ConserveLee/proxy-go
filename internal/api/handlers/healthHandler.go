package handlers

import (
	"github.com/gin-gonic/gin"
	"proxy-go/pkg/server"
)

type HeartbeatStatus struct {
	Name  string `json:"name"`
	IP    string `json:"ip"`
	State uint8  `json:"state"`
}

func ListHeartbeats(c *gin.Context) {
	var ret []HeartbeatStatus
	heartbeats := server.Instance.Heartbeats
	if len(heartbeats) == 0 {
		c.JSON(200, []HeartbeatStatus{})
		return
	}
	for name, heartbeat := range heartbeats {
		ret = append(ret, HeartbeatStatus{
			Name:  name,
			IP:    heartbeat.IP,
			State: heartbeat.State,
		})
	}
	c.JSON(200, ret)
}
