package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vareja0/go-jwt/models"
)

// GetIndex renders the home page.
func GetIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.tmpl", gin.H{})
}

// GetLogin renders the login page.
func GetLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.tmpl", gin.H{})
}

// GetSignUp renders the sign-up page.
func GetSignUp(c *gin.Context) {
	c.HTML(http.StatusOK, "signup.tmpl", gin.H{})
}

// GetProfile renders the profile page, passing the authenticated user set by RequireAuthPage middleware.
func GetProfile(c *gin.Context) {
	user, _ := c.Get("user")
	c.HTML(http.StatusOK, "profile.tmpl", gin.H{"User": user.(models.User)})
}
