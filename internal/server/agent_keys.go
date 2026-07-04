package server

// agent_keys.go — 拆分自 agent_api.go

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleAgentEncryptKey(c *gin.Context) {
	var body struct {
		Key      string `json:"key"`
		DeviceId string `json:"deviceId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	encrypted := EncryptApiKey(body.Key, body.DeviceId)
	c.JSON(http.StatusOK, gin.H{"encrypted": encrypted})
}

func (s *Server) handleAgentDecryptKey(c *gin.Context) {
	var body struct {
		Encrypted string `json:"encrypted"`
		DeviceId  string `json:"deviceId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	decrypted := DecryptApiKey(body.Encrypted, body.DeviceId)
	c.JSON(http.StatusOK, gin.H{"decrypted": decrypted})
}

func (s *Server) handleAgentTest(c *gin.Context) {
	// deviceId 从 query/header 取（GET 无 body，POST 允许 header 走）
	deviceId := c.Query("deviceId")
	if deviceId == "" {
		deviceId = c.GetHeader("X-Device-Id")
	}
	cfg := s.readAgentConfig(deviceId)

	result := gin.H{
		"openai": "ok",
		"model":  "connected",
		"note":   "agent integrated into encv-go",
	}

	if cfg.APIKey != "" {
		result["model"] = cfg.BaseURL
	} else {
		result["openai"] = "no_key"
		result["note"] = "未配置 API Key"
	}

	c.JSON(http.StatusOK, result)
}
