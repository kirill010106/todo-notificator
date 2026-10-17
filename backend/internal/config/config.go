package config

import (
	"fmt"
	"log"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env             string        `yaml:"env" env:"ENV" env-default:"local"`
	StoragePath     string        `yaml:"storage_path" env:"STORAGE_PATH"`
	ClientURL       string        `yaml:"client_url" env:"CLIENT_URL" env-default:"http://localhost:3030"`
	MetricsServer   MetricsServer `yaml:"metrics_server"`
	HTTPServer      `yaml:"http_server"`
	AccessTokenTTL  time.Duration `yaml:"access_token_ttl" env-default:"15m"`
	RefreshTokenTTL time.Duration `yaml:"refresh_token_ttl" env-default:"168h"`
	AppSecret       string        `yaml:"app_secret" env-required:"true" env:"APP_SECRET"`
	Webhook         Webhook       `yaml:"webhook"`
	Clients         Clients       `yaml:"clients"`
	YooKassa        YooKassa      `yaml:"yookassa"`
	AutoMigrate     bool          `yaml:"auto_migrate" env:"AUTO_MIGRATE" env-default:"false"`
}

type YooKassa struct {
	ShopID    string `yaml:"shop_id" env:"SHOP_ID" env-default:""`
	SecretKey string `yaml:"secret_key" env:"SECRET_KEY" env-default:""`
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
	TrustedProxies []string      `yaml:"trusted_proxies" env:"TRUSTED_PROXIES" env-separator:","`
	Address        string        `yaml:"address" env-default:"localhost:8080"`
	Timeout        time.Duration `yaml:"timeout" env-default:"4s"`
	IdleTimeout    time.Duration `yaml:"idle_timeout" env-default:"60s"`
}

// ProxyPrefixes rejects accidental trust of the entire Internet. Empty means
// direct connections only; private networks are not implicitly trusted.
func (c *Config) ProxyPrefixes() ([]netip.Prefix, error) {
	var result []netip.Prefix
	for _, raw := range c.TrustedProxies {
		raw = strings.TrimSpace(raw)
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			if addr, addrErr := netip.ParseAddr(raw); addrErr == nil {
				p, err = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen()), nil
			}
		}
		if err != nil || p.Bits() == 0 {
			return nil, fmt.Errorf("invalid trusted proxy %q: specify an IP or a restricted CIDR", raw)
		}
		result = append(result, p.Masked())
	}
	return result, nil
}

type MetricsServer struct {
	Address string `yaml:"address" env:"METRICS_ADDRESS" env-default:"0.0.0.0:9091"`
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
	if _, err := cfg.ProxyPrefixes(); err != nil {
		log.Fatal(err)
	}
	return &cfg
}
