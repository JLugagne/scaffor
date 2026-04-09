package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JLugagne/joist/internal/joist"
)

func main() {
	runner := joist.Setup()
	ctx := context.Background()
	if err := runner(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
