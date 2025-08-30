package middleware

import (
	"net/url"
	"strconv"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSMiddleware provides enhanced CORS configuration with port range support
func CORSMiddleware() gin.HandlerFunc {
	config := cors.Config{
		AllowOriginFunc: func(origin string) bool {
			// Parse the origin URL
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}

			// Check if it's a localhost variant
			hostname := u.Hostname()
			if hostname != "localhost" && hostname != "127.0.0.1" && hostname != "[::1]" {
				return false
			}

			// Extract port
			port := u.Port()
			if port == "" {
				return false
			}

			// Parse port number
			portNum, err := strconv.Atoi(port)
			if err != nil {
				return false
			}

			// Allow specific development ports and your range
			// 4320-4399: Your custom site isolation testing range
			return (portNum >= 4320 && portNum <= 4399)
		},
		AllowMethods: []string{
			"GET", "POST", "PUT", "DELETE", "OPTIONS",
		},
		AllowHeaders: []string{
			"Origin", "Content-Type", "Accept", "Authorization",
			"X-Tenant-ID", "X-Requested-With", "X-TractStack-Session-ID", "X-StoryFragment-ID",
			"hx-current-url", "hx-request", "hx-target", "hx-trigger", "hx-boosted",
			"Cache-Control",
			"hx-trigger-name",
			"hx-active-element",
			"hx-active-element-name",
			"hx-active-element-value",
		},
		AllowCredentials: true,
		ExposeHeaders: []string{
			"Content-Type", "Cache-Control", "Connection",
		},
	}

	return cors.New(config)
}
