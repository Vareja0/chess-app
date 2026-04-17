package middleware

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/vareja0/go-jwt/controllers"
	"github.com/vareja0/go-jwt/initializers"
	"github.com/vareja0/go-jwt/models"
)

// authenticate validates the JWT cookie and populates c "user".
// Returns true on success, false if the request should be aborted.
func authenticate(c *gin.Context) bool {
	ctx := context.Background()

	tokenString, err := c.Cookie("Authorization")
	if err != nil {
		log.Print("ERRO MID COOKIE")
		return false
	}

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(os.Getenv("SECRET_KEY")), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		log.Print("ERRO MID PARSE")
		return false
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		log.Print("ERRO MID CLAIM ERRADA")
		return false
	}

	if float64(time.Now().Unix()) > claims["exp"].(float64) {
		return false
	}

	var user models.User
	initializers.DB.First(&user, claims["sub"])
	if user.ID == 0 {
		log.Print("ERRO MID NO USER")
		return false
	}

	c.Set("user", user)
	controllers.AddIfNotExists(ctx, user.ID)
	return true
}

// RequireAuth protects API routes — returns 401 on failure.
func RequireAuth(c *gin.Context) {
	if !authenticate(c) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}

// RequireAuthPage protects page routes — redirects to /login on failure.
func RequireAuthPage(c *gin.Context) {
	if !authenticate(c) {
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
		return
	}
	c.Next()
}

func Refresh(c *gin.Context) {
	refreshTokenString, err := c.Cookie("RefreshToken")
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "No refresh token"})
		return
	}

	// Validate the JWT
	token, err := jwt.Parse(refreshTokenString, func(token *jwt.Token) (any, error) {
		return []byte(os.Getenv("REFRESH_SECRET_KEY")), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid claims"})
		return
	}

	// Check it exists in DB (not revoked)
	var storedToken models.Session
	result := initializers.DB.Where("refresh_token = ?", refreshTokenString).First(&storedToken)
	if result.Error != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Refresh token revoked"})
		return
	}

	// Issue new access token
	userID := uint(claims["sub"].(float64))
	newAccessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(15 * time.Minute).Unix(),
	})
	newAccessTokenString, err := newAccessToken.SignedString([]byte(os.Getenv("SECRET_KEY")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create token"})
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("Authorization", newAccessTokenString, 900, "", "", false, true)

	c.JSON(http.StatusOK, gin.H{"success": "Token refreshed"})
}
