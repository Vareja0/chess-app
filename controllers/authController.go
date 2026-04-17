package controllers

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/vareja0/go-jwt/initializers"
	"github.com/vareja0/go-jwt/models"
	"golang.org/x/crypto/bcrypt"
)

func issueTokens(c *gin.Context, user models.User) error {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID,
		"exp": time.Now().Add(15 * time.Minute).Unix(),
	})
	tokenString, err := token.SignedString([]byte(os.Getenv("SECRET_KEY")))
	if err != nil {
		return err
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID,
		"exp": time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
	refreshTokenString, err := refreshToken.SignedString([]byte(os.Getenv("REFRESH_SECRET_KEY")))
	if err != nil {
		return err
	}

	initializers.DB.Where("user_id = ?", user.ID).Delete(&models.Session{})
	initializers.DB.Create(&models.Session{Refresh_token: refreshTokenString, UserID: user.ID})

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("Authorization", tokenString, 900, "", "", false, true)
	c.SetCookie("RefreshToken", refreshTokenString, 3600*24*30, "", "", false, true)
	return nil
}

func Signup(c *gin.Context) {
	ctx := context.Background()
	var body struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	if c.Bind(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}

	if body.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username is required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to hash password"})
		return
	}

	user := models.User{Name: body.Username, Email: body.Email, Password: string(hash)}
	if result := initializers.DB.Create(&user); result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to create user"})
		return
	}

	if err := UpdatePlayerStatus(ctx, user.ID, "idle"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set player state"})
		return
	}

	if err := issueTokens(c, user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to issue tokens"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": "User created"})
}

func Logout(c *gin.Context) {
	refreshToken, err := c.Cookie("RefreshToken")
	if err == nil && refreshToken != "" {
		initializers.DB.Where("refresh_token = ?", refreshToken).Delete(&models.Session{})
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("Authorization", "", -1, "", "", false, true)
	c.SetCookie("RefreshToken", "", -1, "", "", false, true)

	c.JSON(http.StatusOK, gin.H{"success": "logged out"})
}

func Login(c *gin.Context) {
	var body struct {
		Email    string
		Password string
	}

	if c.ShouldBind(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}

	var user models.User
	initializers.DB.First(&user, "email = ?", body.Email)

	if user.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email or password"})
		return
	}

	if err := issueTokens(c, user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to issue tokens"})
		return
	}

	c.HTML(http.StatusOK, "login.tmpl", gin.H{"Success": "login efetuado"})
}
