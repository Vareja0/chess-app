package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/vareja0/go-jwt/models"
)

func GetIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.tmpl", gin.H{})
}

func GetLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.tmpl", gin.H{})
}

func GetSignUp(c *gin.Context) {
	c.HTML(http.StatusOK, "signup.tmpl", gin.H{})
}

func GetProfile(c *gin.Context) {
	user, _ := c.Get("user")
	c.HTML(http.StatusOK, "profile.tmpl", gin.H{"User": user.(models.User)})
}
