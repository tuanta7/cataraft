package config

import (
	"encoding/binary"
	"errors"
	"runtime"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

const (
	PageSize = 8192 // 8KB
)

var (
	ByteOrder = binary.LittleEndian
)

type Config struct {
	BindAddr string `envconfig:"BIND_ADDR" default:"0.0.0.0:13792"`
	DataDir  string `envconfig:"DATA_DIR"`
}

func LoadConfig() (Config, error) {
	var cfg Config

	err := godotenv.Load()
	if err != nil {
		return cfg, err
	}

	err = envconfig.Process("CATARAFT", &cfg)
	if err != nil {
		return cfg, err
	}

	if cfg.DataDir == "" {
		if runtime.GOOS == "linux" {
			cfg.DataDir = "/var/lib/cataraft"
		} else {
			return cfg, errors.New("DATA_DIR not set")
		}
	}

	return cfg, nil
}
