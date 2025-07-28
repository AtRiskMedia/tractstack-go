// Package api provides HTTP handlers and middleware.
package api

import (
	"errors"
	"log"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

// isClientDisconnectError checks if the error is a common network error
// that occurs when a client closes the connection prematurely. These errors
// are safe to ignore in logs as they are not application-level bugs.
func isClientDisconnectError(err error) bool {
	if err == nil {
		return false
	}

	// This is the most common error on POSIX systems when writing to a closed socket.
	if errors.Is(err, syscall.EPIPE) {
		return true
	}

	// This error can happen on Windows or when a connection is forcibly closed.
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	// Go's net/http package can wrap these errors. We check for the specific text.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if opErr.Err.Error() == "write: broken pipe" {
			return true
		}
		// Also check for the underlying syscall errors within the OpError.
		if errors.Is(opErr.Err, syscall.EPIPE) || errors.Is(opErr.Err, syscall.ECONNRESET) {
			return true
		}
	}

	// Fallback for other potential error wrappers, like from within Gin itself.
	if strings.Contains(strings.ToLower(err.Error()), "broken pipe") {
		return true
	}

	return false
}

// FilteredLogger creates a Gin logger middleware that mimics gin.Default()
// but intelligently filters out benign "broken pipe" errors to reduce log noise.
func FilteredLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if c.Request.URL.RawQuery != "" {
			path = path + "?" + c.Request.URL.RawQuery
		}

		c.Next()

		lastError := c.Errors.Last()
		if lastError != nil && isClientDisconnectError(lastError.Err) {
			return
		}

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		var errorMsg string
		if lastError != nil {
			errorMsg = lastError.Error()
		}

		log.Printf("[GIN] %v | %3d | %13v | %15s | %-7s %#v %s",
			time.Now().Format("2006/01/02 - 15:04:05"),
			statusCode,
			latency,
			clientIP,
			method,
			path,
			errorMsg,
		)
	}
}
