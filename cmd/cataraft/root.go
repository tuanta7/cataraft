package main

import (
	"os"
	"runtime"

	_ "github.com/joho/godotenv/autoload"
	"github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/pkg/logger"
	"github.com/tuanta7/cataraft/pkg/silent"
)

func main() {
	log := logger.NewLogger("debug")

	configDir := os.Getenv("CRDATA")
	if configDir == "" {
		if runtime.GOOS == "linux" {
			configDir = "/var/lib/cataraft"
		} else {
			panic("CRDATA not set")
		}
	}
	log.Info().Str("CRDATA", configDir)

	diskAdapter, err := buffer.NewDiskAdapter(configDir, false)
	slient.PanicOnErr(err)
	defer slient.Close(diskAdapter)
}
