package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"smctf/internal/config"

	"github.com/gin-gonic/gin"
)

func TestAPIKeyAuth_EnabledValidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(APIKeyAuth(config.APIKeyConfig{Enabled: true, Value: "secret"}))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	req.Header.Set(apiKeyHeader, "secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAPIKeyAuth_EnabledValidQueryKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(APIKeyAuth(config.APIKeyConfig{Enabled: true, Value: "secret"}))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok?api_key=secret", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAPIKeyAuth_EnabledMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(APIKeyAuth(config.APIKeyConfig{Enabled: true, Value: "secret"}))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAPIKeyAuth_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(APIKeyAuth(config.APIKeyConfig{Enabled: false, Value: ""}))
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
