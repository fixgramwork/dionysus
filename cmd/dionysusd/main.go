package main

import (
	"fmt"
	"os"
)

func main() {
	command := "proxy"
	args := os.Args[1:]
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}

	cfg := configFromArgs(command, args)
	var err error
	switch command {
	case "api", "pvedaemon":
		err = ensureTargetOSRuntime(cfg)
		if err == nil {
			err = serveAPI(cfg)
		}
	case "proxy", "pveproxy":
		err = ensureTargetOSRuntime(cfg)
		if err == nil {
			err = serveProxy(cfg)
		}
	case "metricsd":
		err = ensureTargetOSRuntime(cfg)
		if err == nil {
			err = runMetricsd(cfg, false)
		}
	case "sample-once":
		err = ensureTargetOSRuntime(cfg)
		if err == nil {
			err = runMetricsd(cfg, true)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported command: %s\n", command)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[dionysusd] %s\n", err)
		os.Exit(1)
	}
}
