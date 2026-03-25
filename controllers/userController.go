package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/vareja0/go-jwt/initializers"
	"github.com/vareja0/go-jwt/models"
	"golang.org/x/crypto/bcrypt"
)

type PlayerState struct {
	Status string `json:"status"`
	RoomID string `json:"room_id"`
}

func GetSignUp(c *gin.Context) {
	c.HTML(http.StatusOK, "signup.tmpl", gin.H{})
}

func userKey(userID uint) string {
	return fmt.Sprintf("player:%d", userID)
}

func SetPlayerState(ctx context.Context, userID uint, state PlayerState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return initializers.RDB.Set(ctx, userKey(userID), data, 0).Err()
}

func getPlayerStateRaw(ctx context.Context, userID uint) (*PlayerState, error) {
	val, err := initializers.RDB.Get(ctx, userKey(userID)).Bytes()
	if err != nil {
		return nil, err
	}
	var state PlayerState
	if err := json.Unmarshal(val, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func GetPlayerState(ctx context.Context, userID uint) (*PlayerState, error) {
	return getPlayerStateRaw(ctx, userID)
}

func GetPlayerStatus(ctx context.Context, userID uint) (string, error) {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		return "", err
	}
	return state.Status, nil
}

func GetPlayerRoom(ctx context.Context, userID uint) (string, error) {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		return "", err
	}
	return state.RoomID, nil
}

func UpdatePlayerStatus(ctx context.Context, userID uint, status string) error {
	state, err := getPlayerStateRaw(ctx, userID)
	if err != nil {
		state = &PlayerState{RoomID: ""}
	}
	state.Status = status
	return SetPlayerState(ctx, userID, *state)
}

func UpdatePlayerRoom(ctx context.Context, userID uint, status string, roomID string) error {
	return SetPlayerState(ctx, userID, PlayerState{Status: status, RoomID: roomID})
}

func AddIfNotExists(ctx context.Context, userID uint) error {
	key := userKey(userID)
	exists, _ := initializers.RDB.Exists(ctx, key).Result()
	log.Printf("AddIfNotExists: key=%s exists=%d", key, exists)
	if exists == 0 {
		err := SetPlayerState(ctx, userID, PlayerState{Status: "idle", RoomID: ""})
		log.Printf("AddIfNotExists: set result err=%v", err)
		return err
	}
	return nil
}

func DeletePlayerState(ctx context.Context, userID uint) error {
	return initializers.RDB.Del(ctx, userKey(userID)).Err()
}

func Signup(c *gin.Context) {
	ctx := context.Background()
	var body struct {
		Email    string
		Password string
	}

	if c.Bind(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to read body",
		})

		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to hash password",
		})

		return
	}

	user := models.User{Email: body.Email, Password: string(hash)}

	result := initializers.DB.Create(&user)

	UpdatePlayerStatus(ctx, user.ID, "idle")

	if result.Error != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to create user",
		})

		return
	}

	c.JSON(http.StatusOK, gin.H{
		"sucess": "User created",
	})
}

func GetLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.tmpl", gin.H{})
}

func GetIndex(c *gin.Context) {
	c.HTML(http.StatusOK, "index.tmpl", gin.H{})
}

func Login(c *gin.Context) {

	var body struct {
		Email    string
		Password string
	}

	if c.ShouldBind(&body) != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to read body",
		})

		return
	}

	var user models.User
	initializers.DB.First(&user, "email = ?", body.Email)

	if user.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid email or password" + body.Email + body.Password,
		})

		return
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(body.Password))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid email or password",
		})

		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID,
		"exp": time.Now().Add(15 * time.Minute).Unix(),
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("SECRET_KEY")))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to create token",
		})

		return
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": user.ID,
		"exp": time.Now().Add(30 * 24 * time.Hour).Unix(),
	})

	refreshTokenString, err := refreshToken.SignedString([]byte(os.Getenv("REFRESH_SECRET_KEY")))

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to create refresh token",
		})

		return
	}

	initializers.DB.Where("user_id = ?", user.ID).Delete(&models.Session{})
	initializers.DB.Create(&models.Session{Refresh_token: refreshTokenString, UserID: user.ID})

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("Authorization", tokenString, 900, "", "", false, true)
	c.SetCookie("RefreshToken", refreshTokenString, 3600*24*30, "", "", false, true)

	c.HTML(http.StatusOK, "login.tmpl", gin.H{
		"Sucess": "login efetuado",
	})

}
