package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/acd19ml/hermes-go/internal/agent"
)

const version = "v0.0.1"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("hermes-go", flag.ContinueOnError)
	flags.SetOutput(stderr)

	showVersion := flags.Bool("version", false, "print version and exit")
	msg := flags.String("msg", "", "send a message and print the response")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "unexpected argument: %s\n", flags.Arg(0))
		flags.Usage()
		return 2
	}

	// --msg takes priority over --version and the no-args version banner.
	if flags.Lookup("msg").Value.String() != "" || isFlagSet(flags, "msg") {
		client, err := agent.NewOpenAIChatClientFromEnv()
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		a := agent.NewAIAgent(client)
		resp, err := a.RunOnce(context.Background(), *msg)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, resp.Content)
		return 0
	}

	if *showVersion || len(args) == 0 {
		fmt.Fprintf(stdout, "hermes-go %s\n", version)
		return 0
	}

	return 0
}

// isFlagSet reports whether the named flag was explicitly provided in args.
// flag.Value.String() returns the default for unset flags, so it cannot
// distinguish between --msg "" (explicitly empty) and a missing --msg.
func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
