package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GoogleAPIKey        string
	JQuantsRefreshToken string
}

func Load() *Config {
	// .envファイルがあれば読み込む（本番環境などではない場合も考慮してエラーは無視しないが、Fatalにはしない）
	if err := godotenv.Load(); err != nil {
		log.Println("Note: .env file not found, reading from system environment variables.")
	}

	cfg := &Config{
		GoogleAPIKey:        os.Getenv("GOOGLE_API_KEY"),
		JQuantsRefreshToken: os.Getenv("JQUANTS_REFRESH_TOKEN"),
	}

	if cfg.GoogleAPIKey == "" || cfg.JQuantsRefreshToken == "" {
		log.Fatal("Error: GOOGLE_API_KEY and JQUANTS_REFRESH_TOKEN must be set.")
	}

	return cfg
}