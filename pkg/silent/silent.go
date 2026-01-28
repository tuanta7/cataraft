package slient

import (
	"context"
	"fmt"
	"io"
	"log"
)

// Close implements a wrapper for services that require a silent close within a defer statement.
// It should only be used within the main function.
func Close(srv io.Closer) {
	if err := srv.Close(); err != nil {
		log.Printf("Error while closing: %s", err)
	}
}

type CloserWithContext interface {
	Close(ctx context.Context) error
}

func CloseWithContext(srv CloserWithContext, ctx context.Context) {
	if err := srv.Close(ctx); err != nil {
		log.Printf("Error while closing: %v", err)
	}
}

func PanicOnErr(err error, msg ...string) {
	if err != nil {
		if len(msg) > 0 {
			fmt.Printf("%s: %s\n", msg, err)
		}
		panic(err)
	}
}
