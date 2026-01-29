package main

import (
	"context"
	"errors"
	"os"
	"runtime"

	_ "github.com/joho/godotenv/autoload"
	"github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/internal/storage/buffer/strategy"
	"github.com/tuanta7/cataraft/pkg/logger"
	"github.com/tuanta7/cataraft/pkg/silent"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Commands: []*cli.Command{
			ExecCommand(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			log := logger.NewLogger("debug")

			configDir := os.Getenv("CATA_DATA")
			if configDir == "" {
				if runtime.GOOS == "linux" {
					configDir = "/var/lib/cataraft"
				} else {
					return errors.New("CRDATA not set")
				}
			}
			log.Info().Str("CATA_DATA", configDir).Msg("")

			diskAdapter, err := buffer.NewDiskAdapter(configDir, false)
			if err != nil {
				return err
			}
			defer slient.Close(diskAdapter)

			lru := strategy.NewLRUList[buffer.PageID]()
			bf := buffer.NewBuffer(1000, lru, diskAdapter)

			return bf.FlushAll()
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slient.PanicOnErr(err)
	}
}
