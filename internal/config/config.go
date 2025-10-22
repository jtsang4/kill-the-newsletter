package config

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

type TLS struct {
	Key         string `json:"key"`
	Certificate string `json:"certificate"`
}

type Config struct {
	Hostname                 string  `json:"hostname"`
	SystemAdministratorEmail *string `json:"systemAdministratorEmail,omitempty"`
	TLS                      TLS     `json:"tls"`
	DataDirectory            string  `json:"dataDirectory"`
	Environment              string  `json:"environment"`
	SMTPPort                 int     `json:"smtpPort"`
	HSTSPreload              *bool   `json:"hstsPreload,omitempty"`
	ExtraCaddyfile           *string `json:"extraCaddyfile,omitempty"`
	HTTPAddr                 string  `json:"httpAddr"`
	RunType                  string  `json:"runType"`
}

type AppEnv string

const (
	EnvProduction  AppEnv = "production"
	EnvDevelopment AppEnv = "development"
)

type AppOptions struct {
	Now func() time.Time
}

func DefaultOptions() AppOptions { return AppOptions{Now: time.Now} }

func Load(path string) (Config, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.Hostname == "" {
		return Config{}, errors.New("hostname is required")
	}
	if cfg.DataDirectory == "" {
		cfg.DataDirectory, _ = filepath.Abs("./data/")
	}
	if cfg.Environment == "" {
		cfg.Environment = string(EnvProduction)
	}
	if cfg.SMTPPort == 0 {
		if cfg.Environment == string(EnvDevelopment) {
			cfg.SMTPPort = 2525
		} else {
			cfg.SMTPPort = 25
		}
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.RunType == "" {
		cfg.RunType = "all"
	}
	return cfg, nil
}
