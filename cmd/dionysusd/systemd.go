package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	systemctlStatusTimeout     = 8 * time.Second
	systemctlStatusOutputLimit = 96 * 1024
)

func systemdStatus() map[string]any {
	command := "systemctl status --no-pager --lines=80"
	if _, err := exec.LookPath("systemctl"); err != nil {
		return map[string]any{
			"available":       false,
			"command":         command,
			"durationMillis":  0,
			"exitCode":        -1,
			"status":          "missing",
			"stderr":          "systemctl is not available on this OS",
			"stderrTruncated": false,
			"stdout":          "",
			"stdoutTruncated": false,
			"summary":         map[string]any{},
			"timedOut":        false,
		}
	}

	result := runSystemctlStatus()
	result["available"] = true
	result["command"] = command
	result["summary"] = parseSystemctlStatusSummary(asString(result["stdout"]))
	return result
}

func runSystemctlStatus() map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), systemctlStatusTimeout)
	defer cancel()

	stdout := &limitedBuffer{limit: systemctlStatusOutputLimit}
	stderr := &limitedBuffer{limit: systemctlStatusOutputLimit}
	cmd := exec.CommandContext(ctx, "systemctl", "status", "--no-pager", "--lines=80")
	cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH")+":/bin:/sbin:/usr/bin:/usr/sbin")
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
	status := "ok"
	if err != nil {
		status = "failed"
		exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if !timedOut && stderr.buffer.Len() == 0 {
			_, _ = stderr.Write([]byte(err.Error()))
		}
	}
	if timedOut {
		status = "timeout"
	}

	return map[string]any{
		"durationMillis":  duration.Milliseconds(),
		"exitCode":        exitCode,
		"status":          status,
		"stderr":          stderr.String(),
		"stderrTruncated": stderr.truncated,
		"stdout":          stdout.String(),
		"stdoutTruncated": stdout.truncated,
		"timedOut":        timedOut,
	}
}

func parseSystemctlStatusSummary(output string) map[string]any {
	summary := map[string]any{}
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if _, exists := summary["host"]; !exists {
			summary["host"] = strings.TrimSpace(strings.TrimPrefix(trimmed, "●"))
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "State":
			summary["stateText"] = value
			if fields := strings.Fields(value); len(fields) > 0 {
				summary["state"] = fields[0]
			}
		case "Units":
			summary["units"] = value
		case "Jobs":
			summary["jobs"] = value
		case "Failed":
			summary["failed"] = value
		case "Since":
			summary["since"] = value
		case "CGroup":
			summary["cgroup"] = value
		case "systemd":
			summary["systemd"] = value
		}
	}
	return summary
}
