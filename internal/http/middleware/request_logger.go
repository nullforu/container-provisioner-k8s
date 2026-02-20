package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"smctf/internal/config"
	"smctf/internal/logging"

	"github.com/gin-gonic/gin"
)

var bodyLogMethods = map[string]struct{}{
	http.MethodPost:  {},
	http.MethodPut:   {},
	http.MethodPatch: {},
}

func RequestLogger(cfg config.LoggingConfig, logger *logging.Logger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var log *slog.Logger
		if logger != nil {
			log = logger.Logger
		}

		start := time.Now().UTC()

		_, bodyStr := readRequestBody(ctx, cfg.MaxBodyBytes)

		ctx.Next()

		status := ctx.Writer.Status()
		latency := time.Since(start)
		clientIP := ctx.ClientIP()
		method := ctx.Request.Method
		path := ctx.Request.URL.Path
		rawQuery := ctx.Request.URL.RawQuery
		userAgent := ctx.Request.UserAgent()
		contentType := ctx.GetHeader("Content-Type")
		contentLength := ctx.Request.ContentLength
		errStr := strings.TrimSpace(ctx.Errors.ByType(gin.ErrorTypeAny).String())

		attrs := make([]slog.Attr, 0, 12)
		attrs = append(attrs,
			slog.String("method", method),
			slog.String("path", path),
			slog.Int("status", status),
			slog.Duration("latency", latency),
			slog.String("ip", clientIP),
		)

		if rawQuery != "" {
			attrs = append(attrs, slog.String("query", rawQuery))
		}

		if userAgent != "" {
			attrs = append(attrs, slog.String("user_agent", userAgent))
		}

		if contentType != "" {
			attrs = append(attrs, slog.String("content_type", contentType))
		}

		if contentLength >= 0 {
			attrs = append(attrs, slog.Int64("content_length", contentLength))
		}

		if bodyStr != "" {
			attrs = append(attrs, slog.String("body", bodyStr))
		}

		if errStr != "" {
			attrs = append(attrs, slog.String("error", errStr))
		}

		if log != nil {
			anyAttrs := make([]any, 0, len(attrs))
			for _, attr := range attrs {
				anyAttrs = append(anyAttrs, attr)
			}

			if status >= http.StatusBadRequest || errStr != "" {
				log.Error("http request", slog.Group("http", anyAttrs...))
				return
			}

			log.Info("http request", slog.Group("http", anyAttrs...))
		}
	}
}

func readRequestBody(ctx *gin.Context, maxBodyBytes int) ([]byte, string) {
	if ctx.Request == nil || ctx.Request.Body == nil {
		return nil, ""
	}

	if _, ok := bodyLogMethods[ctx.Request.Method]; !ok {
		return nil, ""
	}

	limited := io.LimitReader(ctx.Request.Body, int64(maxBodyBytes))
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, ""
	}

	ctx.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	bodyStr := string(bodyBytes)
	if maxBodyBytes > 0 && len(bodyStr) == maxBodyBytes {
		bodyStr = bodyStr + "...(truncated)"
	}

	return bodyBytes, bodyStr
}
