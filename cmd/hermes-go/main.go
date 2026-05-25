package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const version = "v0.0.1"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("hermes-go", flag.ContinueOnError)
	flags.SetOutput(stderr)

	showVersion := flags.Bool("version", false, "print version and exit")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", flags.Arg(0))
		flags.Usage()
		return 2
	}

	if *showVersion || len(args) == 0 {
		fmt.Fprintf(stdout, "hermes-go %s\n", version)
		return 0
	}

	return 0
}
