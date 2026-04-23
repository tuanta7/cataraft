package slient

import (
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

func PanicOnErr(err error, msg ...string) {
	if err != nil {
		if len(msg) > 0 {
			fmt.Printf("%s: %s\n", msg[0], err)
		}
		panic(err)
	}
}
