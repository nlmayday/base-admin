// Package middleware contains the cross-cutting Gin middleware applied to
// every request. Order matters; see app.newEngine.
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"base-admin/server/internal/platform/httpx"
	"base-admin/server/internal/platform/requestid"
)

// RequestID reuses a well-formed incoming X-Request-ID or generates one,
// stores it in the request context and echoes it in the response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestid.Header)
		if !requestid.Valid(id) {
			id = requestid.New()
		}
		c.Request = c.Request.WithContext(requestid.With(c.Request.Context(), id))
		c.Header(requestid.Header, id)
		c.Next()
	}
}

// AccessLog writes one structured log line per request. The query string is
// not logged because it may carry sensitive values.
func AccessLog(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		fields := []zap.Field{
			zap.String("request_id", requestid.From(c.Request.Context())),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("route", c.FullPath()),
			zap.Int("status", status),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("bytes", c.Writer.Size()),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.Strings("errors", c.Errors.Errors()))
		}

		// Probes repeat every few seconds; an expected 503 (dependency down,
		// draining) must not flood the error log.
		isProbe := strings.HasPrefix(c.FullPath(), "/health/")
		switch {
		case status >= http.StatusInternalServerError && !isProbe:
			logger.Error("http request", fields...)
		case status >= http.StatusBadRequest:
			logger.Warn("http request", fields...)
		case isProbe:
			logger.Debug("http request", fields...)
		default:
			logger.Info("http request", fields...)
		}
	}
}

// Recover turns a handler panic into a 500 response. http.ErrAbortHandler is
// re-panicked so net/http can abort the connection as intended.
func Recover(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if r == http.ErrAbortHandler { //nolint:errorlint // sentinel compared by identity, as net/http does
				panic(r)
			}
			logger.Error("panic recovered",
				zap.String("request_id", requestid.From(c.Request.Context())),
				zap.Any("panic", r),
				zap.Stack("stack"),
			)
			httpx.Fail(c, httpx.Internal(fmt.Errorf("panic: %v", r)))
		}()
		c.Next()
	}
}

// SecurityHeaders sets conservative headers for JSON API responses. HSTS is
// set by the TLS-terminating proxy.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		// API responses carry personal data; never store them in caches.
		h.Set("Cache-Control", "no-store")
		c.Next()
	}
}

// BodyLimit caps the request body size. Requests that declare a larger
// Content-Length are rejected immediately; others fail when reading past
// the limit.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return BodyLimitFunc(func(*gin.Context) int64 { return maxBytes })
}

// BodyLimitFunc is BodyLimit with a per-request limit. A limit of 0 or less
// leaves the body alone: such routes (uploads) apply their own, larger limit
// after the access guard.
func BodyLimitFunc(limit func(*gin.Context) int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		maxBytes := limit(c)
		if maxBytes <= 0 {
			c.Next()
			return
		}
		if c.Request.ContentLength > maxBytes {
			httpx.Fail(c, httpx.PayloadTooLarge())
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

// Timeout gives each request context a deadline. Handlers and the data
// layer must pass this context down so that work stops when it expires.
func Timeout(d time.Duration) gin.HandlerFunc {
	return TimeoutFunc(d, nil)
}

// TimeoutFunc is Timeout with a per-request override of the context
// deadline (a long upload or download). It does not touch the connection
// deadlines: the module router extends those after the access guard.
// A nil override uses def for every request.
func TimeoutFunc(def time.Duration, override func(*gin.Context) time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		d := def
		if override != nil {
			if o := override(c); o > 0 {
				d = o
			}
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
