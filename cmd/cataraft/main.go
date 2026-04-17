package main

import (
	"context"
	"os"

	"github.com/tuanta7/cataraft/internal/config"
	"github.com/tuanta7/cataraft/internal/execution"
	"github.com/tuanta7/cataraft/internal/query"
	"github.com/tuanta7/cataraft/internal/storage/access/bptree"
	"github.com/tuanta7/cataraft/internal/storage/buffer"
	"github.com/tuanta7/cataraft/internal/storage/disk"
	"github.com/tuanta7/cataraft/internal/storage/disk/copyonwrite"
	"github.com/tuanta7/cataraft/pkg/monitor"
	"github.com/tuanta7/cataraft/pkg/silent"
	"github.com/urfave/cli/v3"
)

func main() {
	logger := monitor.NewLogger("debug")
	cfg, err := config.LoadConfig()
	if err != nil {
		slient.PanicOnErr(err)
	}

	diskAdapter, err := disk.NewAdapter(cfg.DataDir)
	if err != nil {
		slient.PanicOnErr(err)
	}
	defer slient.Close(diskAdapter)

	copyOnWrite, err := copyonwrite.NewAdapter(diskAdapter)
	if err != nil {
		slient.PanicOnErr(err)
	}

	lruBuffer := buffer.NewLRUBuffer(1000, copyOnWrite)
	tree, err := bptree.New(bptree.DefaultOrder, lruBuffer)
	if err != nil {
		slient.PanicOnErr(err)
	}

	engine, err := execution.NewEngine(query.NewParser(), tree, lruBuffer)
	if err != nil {
		slient.PanicOnErr(err)
	}

	cmd := &cli.Command{
		Commands: []*cli.Command{
			ExecCommand(engine),
		},
		Action: func(ctx context.Context, command *cli.Command) error {
			logger.Info().Str("CATARAFT_DATA_DIR", cfg.DataDir).Msg("")
			return engine.Flush()
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slient.PanicOnErr(err)
	}
}
