package main

import (
	"os"

	"github.com/self-deprecated/sd/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.Options{Args: os.Args[1:], Stdout: os.Stdout, Stderr: os.Stderr, Stdin: os.Stdin}))
}
