package utils

import (
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/vareja0/go-jwt/initializers"
	"github.com/vareja0/go-jwt/models"
)

func GetUserId(c *gin.Context) models.User {
	tokenString, err := c.Cookie("Authorization")

	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		log.Print("ERRO ACHAR COOKIE")
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {

		return []byte(os.Getenv("SECRET_KEY")), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

	if err != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		log.Print("ERRO PARSE")
	}
	var user models.User

	if claims, ok := token.Claims.(jwt.MapClaims); ok {

		initializers.DB.First(&user, claims["sub"])

		if user.ID == 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			log.Print("ERRO NO USER")
		}

	} else {
		c.AbortWithStatus(http.StatusUnauthorized)
		log.Print("ERRO CLAIM ERRADA")

	}
	return user

}
