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
	StorageAccountId string
	StorageAccessKey string
	StorageAccessSecret string
	StorageBucket string
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
	storageAccountId:= os.Getenv("STORAGE_ACCOUNT_ID")
	if storageAccountId == "" {
		panic("JWT_STORAGE_ACCOUNT_ID environment variable is not set")
	}
	storageAccessKey := os.Getenv("STORAGE_ACCESS_KEY")
	if storageAccessKey == "" {
		panic("STORAGE_ACCESS_KEY environment variable is not set")
	}
	storageAccessSecret := os.Getenv("STORAGE_ACCESS_SECRET")
	if storageAccessSecret == "" {
		panic("STORAGE_ACCESS_SECRET environment variable is not set")
	}
	storageBucket := os.Getenv("STORAGE_BUCKET")
	if storageBucket == "" {
		panic("STORAGE_BUCKET environment variable is not set")
	}
	
	return Config{
		Port: port,
		Env: env,
		DbUri:dbUri,
		JwtKey: jwtKey,
		StorageAccountId: storageAccountId,
		StorageAccessKey: storageAccessKey,
		StorageAccessSecret: storageAccessSecret,
		StorageBucket: storageBucket,
	}
	

}