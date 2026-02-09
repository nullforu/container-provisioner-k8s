package middleware

import (
	"net/http"
	"strings"

	"smctf/internal/config"

	"github.com/gin-gonic/gin"
)

const (
	apiKeyHeader = "X-API-KEY"
	apiKeyQuery  = "api_key"
)

func APIKeyAuth(cfg config.APIKeyConfig) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if strings.HasPrefix(ctx.Request.URL.Path, "/dashboard") {
			ctx.Next()
			return
		}

		if !cfg.Enabled {
			ctx.Next()
			return
		}

		expected := strings.TrimSpace(cfg.Value)
		if expected == "" {
			ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "api key is not configured"})
			return
		}

		provided := strings.TrimSpace(ctx.GetHeader(apiKeyHeader))
		if provided == "" {
			provided = strings.TrimSpace(ctx.Query(apiKeyQuery))
		}

		if provided == "" || provided != expected {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid api key"})
			return
		}

		ctx.Next()
	}
}
