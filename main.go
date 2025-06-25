package main

import (
	"log"

	"github.com/AtRiskMedia/tractstack-go/api"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}
	r := gin.Default()
	r.SetTrustedProxies([]string{"127.0.0.1"})
	r.Use(cors.Default())
	r.POST("/api/v1/auth/visit", api.VisitHandler)
	r.GET("/api/v1/auth/sse", api.SseHandler)
	r.POST("/api/v1/auth/state", api.StateHandler)
	r.GET("/api/v1/auth/profile/decode", api.DecodeProfileHandler)
	r.POST("/api/v1/auth/login", api.LoginHandler)
	r.Run(":8080")
}
