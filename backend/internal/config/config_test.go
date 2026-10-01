package config

import (
	"strings"
	"testing"
	"time"
)

func validProdConfig() Config {
	var c Config
	c.App.Name = "app"
	c.App.Env = "production"
	c.HTTP.Addr = ":8080"
	c.DB.Host, c.DB.User, c.DB.Name = "db", "u", "n"
	c.Redis.Addr = "redis:6379"
	c.Queue.Concurrency = 1
	c.JWT.AccessTTL, c.JWT.RefreshTTL = time.Minute, time.Hour
	c.JWT.AccessSecret = strings.Repeat("a", 40)
	c.JWT.RefreshSecret = strings.Repeat("b", 40)
	c.Encryption.Key = strings.Repeat("k", 32)
	c.Encryption.CustomerPIIKey = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
	c.Auth.AdapterSecret = strings.Repeat("s", 40)
	c.Centrifugo.Enabled = true
	c.Centrifugo.APIKey = "real-api-key"
	c.Centrifugo.TokenHMAC = "real-hmac"
	return c
}

func TestValidateRejectsDefaultSecretsInProduction(t *testing.T) {
	if err := validProdConfig().validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	mutations := map[string]func(*Config){
		"default encryption key": func(c *Config) { c.Encryption.Key = defaultEncryptionKey },
		"empty encryption key":   func(c *Config) { c.Encryption.Key = "" },
		"default adapter secret": func(c *Config) { c.Auth.AdapterSecret = defaultAdapterSecret },
		"default centrifugo":     func(c *Config) { c.Centrifugo.TokenHMAC = defaultCentrifugoTokenHMAC },
		"short jwt secret":       func(c *Config) { c.JWT.AccessSecret = "short" },
		"missing pii key":        func(c *Config) { c.Encryption.CustomerPIIKey = "" },
		"raw pii key":            func(c *Config) { c.Encryption.CustomerPIIKey = strings.Repeat("p", 32) },
		"pii key equals app key": func(c *Config) { c.Encryption.Key = c.Encryption.CustomerPIIKey },
	}
	for name, mut := range mutations {
		c := validProdConfig()
		mut(&c)
		if err := c.validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
	dev := validProdConfig()
	dev.App.Env = "development"
	dev.Encryption.Key = defaultEncryptionKey
	if err := dev.validate(); err != nil {
		t.Fatalf("development must accept defaults: %v", err)
	}
}
