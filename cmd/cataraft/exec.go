package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/tuanta7/cataraft/internal/execution"
	"github.com/urfave/cli/v3"
)

func ExecCommand(engine *execution.Engine) *cli.Command {
	return &cli.Command{
		Name:  "exec",
		Usage: "execute a GET or SET query",
		Action: func(ctx context.Context, command *cli.Command) error {
			input := strings.Join(command.Args().Slice(), " ")
			if strings.TrimSpace(input) == "" {
				return fmt.Errorf("query is required")
			}

			result, err := engine.Exec(input)
			if err != nil {
				return err
			}

			if err = engine.Flush(); err != nil {
				return err
			}

			if len(result) > 0 {
				fmt.Println(string(result))
			}

			return nil
		},
	}
}
