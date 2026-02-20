package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"smctf/internal/config"
	"smctf/internal/logging"

	"github.com/gin-gonic/gin"
)

func TestRequestLoggerSkipsBodyForGET(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()

	logger, err := logging.New(config.LoggingConfig{
		Dir:          dir,
		FilePrefix:   "req",
		MaxBodyBytes: 1024,
	}, logging.Options{Service: "container-provisioner", Env: "test"})
	if err != nil {
		t.Fatalf("logger init: %v", err)
	}

	defer func() {
		_ = logger.Close()
	}()

	r := gin.New()
	r.Use(RequestLogger(config.LoggingConfig{MaxBodyBytes: 1024}, logger))
	r.GET("/test", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", strings.NewReader(`{"foo":"bar"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}

	payload := readLogLine(t, dir, "req")
	httpFields := extractGroup(t, payload, "http")
	if _, ok := httpFields["body"]; ok {
		t.Fatalf("expected no body in log: %v", httpFields)
	}
}

func readLogLine(t *testing.T, dir, prefix string) map[string]any {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, prefix+"-*.log"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("log file not found: %v", err)
	}

	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("no log lines found")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &payload); err != nil {
		t.Fatalf("invalid json log: %v", err)
	}

	return payload
}

func extractGroup(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := payload[key]
	if !ok {
		t.Fatalf("missing group %s in log: %v", key, payload)
	}

	group, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("invalid group %s in log: %T", key, value)
	}

	return group
}
