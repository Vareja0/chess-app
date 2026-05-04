package initializers

import (
	"log"

	"github.com/joho/godotenv"
)

// LoadEnvVariables reads the .env file into the process environment; panics if the file is missing.
func LoadEnvVariables() {
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading envs")
	}

}
