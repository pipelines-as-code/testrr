package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"testrr/internal/app"
)

func main() {
	if err := app.Run(context.Background(), os.Args[1:]); err != nil {
		if errors.Is(err, app.ErrUsage) {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		log.Printf("testrr: %v", err)
		os.Exit(1)
	}
}
