package models

import (
	"gorm.io/gorm"
)

type Session struct {
	gorm.Model
	User_email    string
	Refresh_token string `gorm:"uniqueIndex"`
	UserID        uint
	User          User
}
