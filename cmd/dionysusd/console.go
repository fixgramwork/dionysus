package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	consoleOutputLimit   = 32 * 1024
	consoleCommandMaxLen = 4096
	consoleMaxTimeout    = 10 * time.Minute
)

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (buffer *limitedBuffer) Write(input []byte) (int, error) {
	written := len(input)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return written, nil
	}
	if len(input) > remaining {
		input = input[:remaining]
		buffer.truncated = true
	}
	_, _ = buffer.buffer.Write(input)
	return written, nil
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}

func runConsoleCommand(cfg Config, body map[string]any) (map[string]any, error) {
	command := strings.TrimSpace(jsonString(body, "command", ""))
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if len(command) > consoleCommandMaxLen {
		return nil, fmt.Errorf("command is too long")
	}

	cwd := strings.TrimSpace(jsonString(body, "cwd", "/"))
	if cwd == "" {
		cwd = "/"
	}

	timeout := consoleTimeout(cfg, body)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stdout := &limitedBuffer{limit: consoleOutputLimit}
	stderr := &limitedBuffer{limit: consoleOutputLimit}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "PATH=/bin:/sbin:/usr/bin:/usr/sbin")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second

	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)
	timedOut := ctx.Err() == context.DeadlineExceeded
	exitCode := 0
	if err != nil {
		exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if !timedOut && stderr.buffer.Len() == 0 {
			_, _ = stderr.Write([]byte(err.Error()))
		}
	}

	return map[string]any{
		"command":         command,
		"cwd":             cwd,
		"durationMillis":  duration.Milliseconds(),
		"exitCode":        exitCode,
		"stderr":          stderr.String(),
		"stderrTruncated": stderr.truncated,
		"stdout":          stdout.String(),
		"stdoutTruncated": stdout.truncated,
		"timedOut":        timedOut,
	}, nil
}

func consoleTimeout(cfg Config, body map[string]any) time.Duration {
	seconds := cfg.ConsoleTimeoutSeconds
	if seconds <= 0 {
		seconds = defaultConsoleTimeoutSecs
	}
	if bodySeconds := asInt64(body["timeoutSeconds"]); bodySeconds > 0 {
		seconds = int(bodySeconds)
	}

	timeout := time.Duration(seconds) * time.Second
	if timeout > consoleMaxTimeout {
		return consoleMaxTimeout
	}
	return timeout
}
