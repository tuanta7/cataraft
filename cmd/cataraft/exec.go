package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func ExecCommand() *cli.Command {
	return &cli.Command{
		Name:  "exec",
		Usage: "execute query",
		Flags: []cli.Flag{},
		Action: func(ctx context.Context, command *cli.Command) error {
			queryTokens := command.Args()
			fmt.Println("exec query")
			fmt.Println(queryTokens)
			return nil
		},
	}
}
