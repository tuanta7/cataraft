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

func LoadConfig(filenames ...string) (Config, error) {
	var cfg Config

	if len(filenames) > 0 {
		if err := godotenv.Load(filenames...); err != nil {
			return cfg, err
		}
	}

	if err := envconfig.Process("CATARAFT", &cfg); err != nil {
		return cfg, err
	}

	if cfg.DataDir == "" {
		if runtime.GOOS == "linux" {
			cfg.DataDir = "~/data/cataraft"
		} else {
			return cfg, errors.New("CATARAFT_DATA_DIR not set")
		}
	}

	return cfg, nil
}
