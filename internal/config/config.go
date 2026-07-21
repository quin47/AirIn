package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type HotkeyConfig struct {
	Modifiers []string `json:"modifiers"` // "ctrl", "alt", "shift", "super"
	Key       string   `json:"key"`       // 按键名称,如 "v", "space", "f1"
}

type Config struct {
	APIKey    string       `json:"api_key"`
	SecretKey string       `json:"secret_key,omitempty"`
	AppID     string       `json:"app_id,omitempty"`
	Cluster   string       `json:"cluster,omitempty"`
	Hotkey    HotkeyConfig `json:"hotkey"`
	mu        sync.RWMutex
}

func Default() *Config {
	return &Config{
		APIKey: "",
		Hotkey: HotkeyConfig{
			Modifiers: []string{"ctrl", "shift"},
			Key:       "v",
		},
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".config", "ime")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create config dir: %w", err)
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	cfg := Default()

	p, err := configPath()
	if err != nil {
		return cfg, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return Default(), fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	p, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(p, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func (c *Config) IsConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.APIKey != ""
}

func (c *Config) GetAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.APIKey
}

func (c *Config) SetAPIKey(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.APIKey = key
}

func (c *Config) GetHotkey() HotkeyConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Hotkey
}

func (c *Config) SetHotkey(hk HotkeyConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Hotkey = hk
}

func (c *Config) HotkeyString() string {
	hk := c.GetHotkey()
	s := ""
	for _, m := range hk.Modifiers {
		s += m + "+"
	}
	s += hk.Key
	return s
}

func (c *Config) GetSecretKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SecretKey
}

func (c *Config) GetAppID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.AppID
}

func (c *Config) GetCluster() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.Cluster == "" {
		return "volcengine_input_edu"
	}
	return c.Cluster
}
