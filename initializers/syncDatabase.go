package initializers

import "github.com/vareja0/go-jwt/models"

// SyncDatabase runs GORM AutoMigrate to create or update the users and sessions tables.
func SyncDatabase() {
	DB.AutoMigrate(&models.User{})
	DB.AutoMigrate(&models.Session{})
}
