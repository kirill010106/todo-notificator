package config

import (
	"cmp"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type Config struct {
	SMTP      SMTP            `yaml:"smtp"`
	Database  Database        `yaml:"database"`
	Intervals []time.Duration `yaml:"notification_intervals"`
	Webhook   Webhook         `yaml:"webhook"`
	AppURL    string          `yaml:"app_url" env:"APP_URL" env-default:"http://localhost:3000"`
}

type Webhook struct {
	Address string `yaml:"address" env:"WEBHOOK_ADDRESS" env-default:"localhost:8084"`
	Secret  string `yaml:"secret" env:"WEBHOOK_SECRET" env-required:"true"`
}

type SMTP struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username" env:"SMTP_USERNAME" env-required:"TRUE"`
	Password string `yaml:"password" env:"SMTP_PASSWORD" env-required:"TRUE"`
}

type Database struct {
	DBUrl string `env:"DATABASE_URL" env-required:"true"`
}

func MustLoad() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment")
	}

	configPath := cmp.Or(os.Getenv("EMAIL_CONFIG_PATH"), "config/local.yaml")

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file not found: %s", configPath)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("failed to read config: %v", err)
	}

	return &cfg

}
