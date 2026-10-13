package main

import (
	"context"
	"os"

	"github.com/tobola/unibox/internal/unibox/cli"
)

func main() { os.Exit(cli.Execute(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
