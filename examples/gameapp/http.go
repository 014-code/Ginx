package gameapp

import (
	"Ginx/limit"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// NewHTTPHandler 创建可嵌入标准 http.Server 的 Gin 路由。
func NewHTTPHandler(service *Service) *gin.Engine {
	router := gin.New()
	// 不自动信任代理头，也不记录登录请求体或 Authorization。
	_ = router.SetTrustedProxies(nil)
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		c.Next()
	})
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	// 原型入口的总量限制，避免密码哈希计算占满服务。
	loginLimit, _ := limit.NewTokenBucket(10, 20)
	router.POST("/api/v1/login", func(c *gin.Context) {
		if !loginLimit.Allow() {
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, gin.H{"code": "rate_limited"})
			return
		}
		var request struct {
			AccountID string `json:"account_id"`
			Password  string `json:"password"`
		}
		// 完整读取受限请求体，拒绝首个 JSON 后的额外内容或超大尾部。
		body, err := io.ReadAll(c.Request.Body)
		if err != nil || json.Unmarshal(body, &request) != nil || request.AccountID == "" || len(request.AccountID) > 128 || request.Password == "" || len(request.Password) > 72 {
			c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request"})
			return
		}
		result, err := service.Login(c.Request.Context(), request.AccountID, request.Password)
		if err != nil {
			writeHTTPError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	router.GET("/api/v1/me", func(c *gin.Context) {
		view, err := service.Me(c.Request.Context(), bearer(c))
		if err != nil {
			writeHTTPError(c, err)
			return
		}
		c.JSON(http.StatusOK, view)
	})
	router.GET("/api/v1/rooms", func(c *gin.Context) {
		rooms, err := service.Rooms(bearer(c))
		if err != nil {
			writeHTTPError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"rooms": rooms})
	})
	router.GET("/api/v1/progress", func(c *gin.Context) {
		progress, err := service.Progress(c.Request.Context(), bearer(c))
		if err != nil {
			writeHTTPError(c, err)
			return
		}
		c.JSON(http.StatusOK, progress)
	})
	return router
}

func bearer(c *gin.Context) string {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func writeHTTPError(c *gin.Context, err error) {
	if errors.Is(err, ErrUnauthorized) {
		c.Header("WWW-Authenticate", "Bearer")
		c.JSON(http.StatusUnauthorized, gin.H{"code": "unauthorized"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": "internal_error"})
}
