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
	r.POST("/logout", controllers.Logout)

	// Protected page routes (redirect to /login on failure)
	r.GET("/profile", middleware.RequireAuthPage, controllers.GetProfile)

	// Protected API/game routes (return 401 on failure)
	protected := r.Group("/", middleware.RequireAuth)
	{
		protected.GET("/create", controllers.CreateGame)
		protected.GET("/ws/:room", controllers.HandleWebSocket)
		protected.POST("/matchmaking", controllers.HandleMatchmaking)
		protected.POST("/matchmaking/cancel", controllers.HandleCancelMatchmaking)
	}

	r.Run("0.0.0.0:3000")
}
