package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	consoleOutputLimit   = 32 * 1024
	consoleCommandMaxLen = 4096
	consoleRunIDMaxLen   = 80
	consoleMaxTimeout    = 10 * time.Minute
)

type consoleRunState struct {
	cancel  context.CancelFunc
	stopped bool
}

type consoleRunRegistry struct {
	mu   sync.Mutex
	runs map[string]*consoleRunState
}

var activeConsoleRuns = consoleRunRegistry{runs: map[string]*consoleRunState{}}

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

func runConsoleCommand(parent context.Context, cfg Config, body map[string]any) (map[string]any, error) {
	command := strings.TrimSpace(jsonString(body, "command", ""))
	if command == "" {
		return nil, fmt.Errorf("command is required")
	}
	if len(command) > consoleCommandMaxLen {
		return nil, fmt.Errorf("command is too long")
	}
	runID, err := cleanConsoleRunID(jsonString(body, "runID", ""))
	if err != nil {
		return nil, err
	}

	cwd := strings.TrimSpace(jsonString(body, "cwd", "/"))
	if cwd == "" {
		cwd = "/"
	}

	timeout := consoleTimeout(cfg, body)
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	unregister := func() bool { return false }
	if runID != "" {
		var err error
		unregister, err = activeConsoleRuns.register(runID, cancel)
		if err != nil {
			return nil, err
		}
	}

	stdout := &limitedBuffer{limit: consoleOutputLimit}
	stderr := &limitedBuffer{limit: consoleOutputLimit}
	cmd := newCommandContext(ctx, "/bin/sh", "-lc", command)
	cmd.Dir = cwd
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
	err = cmd.Run()
	duration := time.Since(started)
	stopped := unregister()
	timedOut := ctx.Err() == context.DeadlineExceeded
	cancelled := stopped || ctx.Err() == context.Canceled
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
		"cancelled":       cancelled,
		"command":         command,
		"cwd":             cwd,
		"durationMillis":  duration.Milliseconds(),
		"exitCode":        exitCode,
		"runID":           runID,
		"stderr":          stderr.String(),
		"stderrTruncated": stderr.truncated,
		"stdout":          stdout.String(),
		"stdoutTruncated": stdout.truncated,
		"stopped":         stopped,
		"timedOut":        timedOut,
	}, nil
}

func stopConsoleCommand(body map[string]any) (map[string]any, error) {
	runID, err := cleanConsoleRunID(jsonString(body, "runID", ""))
	if err != nil {
		return nil, err
	}
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}
	return map[string]any{
		"runID":   runID,
		"stopped": activeConsoleRuns.stop(runID),
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

func cleanConsoleRunID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len(value) > consoleRunIDMaxLen {
		return "", fmt.Errorf("runID is too long")
	}
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == ':' || ch == '.' {
			continue
		}
		return "", fmt.Errorf("runID contains invalid characters")
	}
	return value, nil
}

func (registry *consoleRunRegistry) register(runID string, cancel context.CancelFunc) (func() bool, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.runs[runID]; exists {
		return nil, fmt.Errorf("console run is already active")
	}
	registry.runs[runID] = &consoleRunState{cancel: cancel}
	return func() bool {
		registry.mu.Lock()
		defer registry.mu.Unlock()
		state := registry.runs[runID]
		if state == nil {
			return false
		}
		delete(registry.runs, runID)
		return state.stopped
	}, nil
}

func (registry *consoleRunRegistry) stop(runID string) bool {
	registry.mu.Lock()
	state := registry.runs[runID]
	if state != nil {
		state.stopped = true
		state.cancel()
	}
	registry.mu.Unlock()
	return state != nil
}
