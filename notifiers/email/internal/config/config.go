package config

import (
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
	Host       string `yaml:"host" env:"SMTP_HOST" env-default:"smtp-relay.brevo.com"`
	Port       int    `yaml:"port" env:"SMTP_PORT" env-default:"587"`
	Username   string `yaml:"username" env:"SMTP_USERNAME" env-required:"TRUE"`
	Password   string `yaml:"password" env:"SMTP_PASSWORD" env-required:"TRUE"`
	From       string `yaml:"from" env:"SMTP_FROM"`
	SenderName string `yaml:"sender_name" env:"SMTP_SENDER_NAME" env-default:"ToDoNotificator"`
}

type Database struct {
	DBUrl string `env:"DATABASE_URL" env-required:"true"`
}

func MustLoad() *Config {
	// Load environment variables without overwriting already set vars (like DATABASE_URL from dev.ps1)
	_ = godotenv.Load()
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../backend/.env")
	_ = godotenv.Load("../../backend/.env")

	configPath := os.Getenv("EMAIL_CONFIG_PATH")
	if configPath == "" || !fileExists(configPath) {
		configPath = "config/local.yaml"
	}
	if !fileExists(configPath) {
		configPath = "notifiers/email/config/local.yaml"
	}
	if !fileExists(configPath) {
		log.Fatalf("config file not found: %s", configPath)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("failed to read config: %v", err)
	}

	return &cfg
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
