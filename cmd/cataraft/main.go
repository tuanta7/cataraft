package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/execution"
	"github.com/tuanta7/cataraft/internal/query"
	"github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/internal/storage/index/bptree"
	"github.com/tuanta7/cataraft/internal/storage/recovery"
	"github.com/tuanta7/cataraft/internal/storage/writer/dwb"
	"github.com/tuanta7/cataraft/pkg/monitor"
	"github.com/tuanta7/cataraft/pkg/silent"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Commands: []*cli.Command{
			ExecCommand(),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			log := monitor.NewLogger("debug")
			cfg, err := config.LoadConfig()
			if err != nil {
				return err
			}

			log.Info().Str("CATARAFT_DATA_DIR", cfg.DataDir).Msg("")

			diskAdapter, err := disk.NewAdapter(disk.AdapterConfig{
				BaseDir: cfg.DataDir,
				Direct:  false,
			})
			if err != nil {
				return err
			}
			defer slient.Close(diskAdapter)

			doubleWrite, err := dwb.NewDoubleWriteBuffer(diskAdapter, "")
			if err != nil {
				return err
			}

			lruBuffer := buffer.NewLRUBuffer(1000, diskAdapter, doubleWrite)

			bPlusTreeIndex, err := bptree.New(bptree.DefaultOrder, lruBuffer)
			if err != nil {
				return err
			}

			writeAheadLog, err := recovery.NewWriteAheadLog(diskAdapter, "writeAheadLog")
			if err != nil {
				return err
			}

			manager := execution.NewManager(
				query.NewParser(),
				bPlusTreeIndex,
				lruBuffer,
				writeAheadLog,
			)
			fmt.Println(manager)

			return lruBuffer.FlushAll()
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slient.PanicOnErr(err)
	}
}
