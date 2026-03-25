package initializers

import "github.com/vareja0/go-jwt/models"

func SyncDatabase() {
	DB.AutoMigrate(&models.User{})
	DB.AutoMigrate(&models.Session{})
}
