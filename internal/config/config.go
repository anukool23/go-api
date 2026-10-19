package config

import (
	"os"

	"github.com/joho/godotenv"
)


type Config struct {
	Port string
	Env string
	JwtKey string
	DbUri string
}

func MustLoad() Config {
	godotenv.Load()

	port := os.Getenv("PORT")
	if port == "" {
		panic("PORT environment variable is not set")
	}
	env := os.Getenv("ENV")
	if env == "" {
		panic("ENV environment variable is not set")
	}
	dbUri := os.Getenv("DB_URI")
	if dbUri == "" {
		panic("DB_URI environment variable is not set")
	}
	jwtKey := os.Getenv("JWT_KEY")
	if jwtKey == "" {
		panic("JWT_KEY environment variable is not set")
	}
	return Config{
		Port: port,
		Env: env,
		DbUri:dbUri,
		JwtKey: jwtKey,
	}
	

}