package main

import (
	"github.com/gin-gonic/gin"
	"github.com/vareja0/go-jwt/controllers"
	"github.com/vareja0/go-jwt/initializers"
	"github.com/vareja0/go-jwt/middleware"
)

func init() {
	initializers.LoadEnvVariables()
	initializers.ConnectDb()
	initializers.SyncDatabase()
	initializers.ConnectRedis()
}

func main() {
	r := gin.Default()
	r.SetTrustedProxies(nil)

	r.Static("/public", "./public")
	r.LoadHTMLGlob("views/*")

	// Pages
	r.GET("/", controllers.GetIndex)
	r.GET("/login", controllers.GetLogin)
	r.GET("/signup", controllers.GetSignUp)

	// Auth
	r.POST("/login", controllers.Login)
	r.POST("/signup", controllers.Signup)
	r.POST("/refresh", middleware.Refresh)

	// Game (requires auth)
	game := r.Group("/", middleware.RequireAuth)
	{
		game.GET("/create", controllers.CreateGame)
		game.GET("/ws/:room", controllers.HandleWebSocket)
		game.POST("/matchmaking", controllers.HandleMatchmaking)
		game.POST("/matchmaking/cancel", controllers.HandleCancelMatchmaking)
	}

	r.Run()
}
