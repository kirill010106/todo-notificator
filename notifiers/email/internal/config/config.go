package config

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/joho/godotenv"
)

type Config struct {
	SMTP      SMTP       `yaml:"smtp"`
	Database  Database   `yaml:"database"`
	Intervals []Duration `yaml:"notification_intervals"`
	Webhook   Webhook    `yaml:"webhook"`
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
	DBUrl string `env:"DATABASE_URL" env-requied:"true"`
}

type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	return nil
}

func (c *Config) NotificationIntervals() []time.Duration {
	result := make([]time.Duration, len(c.Intervals))
	for i, d := range c.Intervals {
		result[i] = d.Duration
	}
	return result
}

func MustLoad() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file found, reading from environment")
	}

	configPath := os.Getenv("EMAIL_CONFIG_PATH")
	if configPath == "" {
		configPath = "config/config.yaml"
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file not found: %s", configPath)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("failed to read config: %v", err)
	}

	return &cfg

}
