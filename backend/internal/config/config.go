package config

import (
	"log"
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env             string `yaml:"env" env:"ENV" env-default:"local"`
	StoragePath     string `yaml:"storage_path" env:"STORAGE_PATH"`
	HTTPServer      `yaml:"http_server"`
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl" env-default:"15m"`
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl" env-default:"168h"`
	AppSecret       string        `yaml:"app_secret" env-required:"true" env:"APP_SECRET"`
	Webhook         Webhook       `yaml:"webhook"`
	Clients         Clients       `yaml:"clients"`
	YooKassa        YooKassa      `yaml:"yookassa"`
}

type YooKassa struct {
	ShopID    string `yaml:"shop_id" env:"SHOP_ID" env-required:"true"`
	SecretKey string `yaml:"secret_key" env:"SECRET_KEY" env-required:"true"`
}

type Clients struct {
	ActivityLogger ActivityLoggerClientConf `yaml:"activity_logger"`
}

type ActivityLoggerClientConf struct {
	Address      string        `yaml:"address" env:"ACTIVITY_LOGGER_ADDR" env-default:"localhost:50051"`
	Timeout      time.Duration `yaml:"timeout" env-default:"2s"`
	RetriesCount int           `yaml:"retries_count" env-default:"3"`
}

type Webhook struct {
	URL    string `yaml:"url" env:"WEBHOOK_URL" env-default:""`
	Secret string `yaml:"secret" env:"WEBHOOK_SECRET" env-default:""`
}

type HTTPServer struct {
	ClientURL   string        `yaml:"client_url" env:"CLIENT_URL" env-default:"http://localhost:3000"`
	Address     string        `yaml:"address" env-default:"localhost:8080"`
	Timeout     time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout time.Duration `yaml:"idle_timeout" env-default:"60s"`
}

func MustLoad() *Config {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		log.Fatal("CONFIG_PATH is not set")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("CONFIG_PATH does not exist: %s", configPath)
	}

	var cfg Config
	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config %s", err)
	}
	return &cfg
}
