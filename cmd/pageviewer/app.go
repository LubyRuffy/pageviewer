package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"time"
)

type cliOptions struct {
	url                string
	mode               string
	jsonOutput         bool
	waitTimeout        time.Duration
	traceID            string
	removeInvisibleDiv bool
	acquireTimeout     time.Duration
	proxy              string
	noHeadless         bool
	devTools           bool
}

func parseFlags(args []string) (cliOptions, error) {
	fs := flag.NewFlagSet("pageviewer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts cliOptions
	fs.StringVar(&opts.url, "url", "", "target url")
	fs.StringVar(&opts.mode, "mode", "", "output mode: html|links|article|raw-text")
	fs.BoolVar(&opts.jsonOutput, "json", false, "render JSON output")
	fs.DurationVar(&opts.waitTimeout, "wait-timeout", 0, "page wait timeout")
	fs.StringVar(&opts.traceID, "trace-id", "", "trace id")
	fs.BoolVar(&opts.removeInvisibleDiv, "remove-invisible-div", false, "remove invisible div")
	fs.DurationVar(&opts.acquireTimeout, "acquire-timeout", 0, "worker acquire timeout")
	fs.StringVar(&opts.proxy, "proxy", "", "browser proxy")
	fs.BoolVar(&opts.noHeadless, "no-headless", false, "show browser window")
	fs.BoolVar(&opts.devTools, "devtools", false, "open devtools")

	if err := fs.Parse(args); err != nil {
		return cliOptions{}, err
	}
	if opts.url == "" {
		return cliOptions{}, errors.New("--url is required")
	}
	if opts.mode == "" {
		return cliOptions{}, errors.New("--mode is required")
	}
	switch opts.mode {
	case "html", "links", "article", "raw-text":
	default:
		return cliOptions{}, fmt.Errorf("invalid --mode: %s", opts.mode)
	}
	return opts, nil
}

func runCLI(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) int {
	_, _ = ctx, args
	_, _ = stdout, stderr
	return 0
}
