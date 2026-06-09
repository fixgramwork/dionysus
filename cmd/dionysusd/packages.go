package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	aptListOutputLimit     = 768 * 1024
	aptSearchOutputLimit   = 256 * 1024
	aptActionOutputLimit   = 128 * 1024
	aptReadTimeout         = 25 * time.Second
	aptSearchTimeout       = 45 * time.Second
	aptActionTimeout       = 10 * time.Minute
	aptSearchResultLimit   = 80
	aptPackageNameMaxLen   = 128
	aptPackageSearchMaxLen = 120
)

type aptPackage struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	Architecture     string `json:"architecture"`
	Status           string `json:"status"`
	Description      string `json:"description,omitempty"`
	Upgradeable      bool   `json:"upgradeable"`
	CandidateVersion string `json:"candidateVersion,omitempty"`
}

type aptSearchResult struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	Upgradeable      bool   `json:"upgradeable"`
	CandidateVersion string `json:"candidateVersion,omitempty"`
}

func aptPackagesStatus() map[string]any {
	tools := aptToolsStatus()
	installed, installedErr := listInstalledAptPackages()
	upgradeable, upgradeableErr := listUpgradeableAptPackages()

	for index := range installed {
		if candidate, ok := upgradeable[installed[index].Name]; ok {
			installed[index].Upgradeable = true
			installed[index].CandidateVersion = candidate
		}
	}

	payload := map[string]any{
		"available":        asBool(tools["dpkgQuery"]) && asBool(tools["aptGet"]) && asBool(tools["aptCache"]),
		"installedCount":   len(installed),
		"upgradeableCount": len(upgradeable),
		"packages":         installed,
		"tools":            tools,
	}
	if installedErr != nil {
		payload["error"] = installedErr.Error()
	}
	if upgradeableErr != nil {
		payload["upgradeableError"] = upgradeableErr.Error()
	}
	return payload
}

func searchAptPackages(query string) (map[string]any, error) {
	cleaned, err := cleanAptSearchQuery(query)
	if err != nil {
		return nil, err
	}
	if cleaned == "" {
		return map[string]any{
			"query":   "",
			"results": []aptSearchResult{},
			"tools":   aptToolsStatus(),
		}, nil
	}

	installed, _ := listInstalledAptPackages()
	installedByName := map[string]aptPackage{}
	for _, item := range installed {
		installedByName[item.Name] = item
	}
	upgradeable, _ := listUpgradeableAptPackages()

	if !commandAvailable("apt-cache") {
		return map[string]any{
			"query":     cleaned,
			"available": false,
			"results":   []aptSearchResult{},
			"tools":     aptToolsStatus(),
			"error":     "apt-cache is not available on this OS",
		}, nil
	}

	output, err := runPackageReadCommand(aptSearchTimeout, aptSearchOutputLimit, "apt-cache", "search", "--names-only", cleaned)
	if err != nil {
		return map[string]any{
			"query":   cleaned,
			"results": []aptSearchResult{},
			"tools":   aptToolsStatus(),
			"error":   err.Error(),
		}, nil
	}

	results := parseAptSearchResults(output, installedByName, upgradeable)
	if len(results) > aptSearchResultLimit {
		results = results[:aptSearchResultLimit]
	}
	return map[string]any{
		"query":     cleaned,
		"available": true,
		"results":   results,
		"tools":     aptToolsStatus(),
	}, nil
}

func updateAptPackageIndex(cfg Config) map[string]any {
	return runAptPackageManager(cfg, "update-index", "", []string{"update"})
}

func installAptPackage(cfg Config, body map[string]any) (map[string]any, error) {
	name, err := cleanAptPackageName(jsonString(body, "name", ""))
	if err != nil {
		return nil, err
	}
	return runAptPackageManager(cfg, "install", name, []string{"install", "-y", name}), nil
}

func removeAptPackage(cfg Config, body map[string]any) (map[string]any, error) {
	name, err := cleanAptPackageName(jsonString(body, "name", ""))
	if err != nil {
		return nil, err
	}
	return runAptPackageManager(cfg, "remove", name, []string{"remove", "-y", name}), nil
}

func upgradeAptPackage(cfg Config, body map[string]any) (map[string]any, error) {
	name, err := cleanAptPackageName(jsonString(body, "name", ""))
	if err != nil {
		return nil, err
	}
	return runAptPackageManager(cfg, "upgrade", name, []string{"install", "--only-upgrade", "-y", name}), nil
}

func listInstalledAptPackages() ([]aptPackage, error) {
	if !commandAvailable("dpkg-query") {
		return []aptPackage{}, fmt.Errorf("dpkg-query is not available on this OS")
	}
	output, err := runPackageReadCommand(aptReadTimeout, aptListOutputLimit, "dpkg-query", "-W", "-f=${Package}\t${Version}\t${Architecture}\t${Status}\n")
	if err != nil {
		return []aptPackage{}, err
	}
	return parseAptInstalledPackages(output), nil
}

func listUpgradeableAptPackages() (map[string]string, error) {
	if !commandAvailable("apt") {
		return map[string]string{}, fmt.Errorf("apt is not available on this OS")
	}
	output, err := runPackageReadCommand(aptReadTimeout, aptListOutputLimit, "apt", "list", "--upgradeable")
	if err != nil {
		return map[string]string{}, err
	}
	return parseAptUpgradeablePackages(output), nil
}

func parseAptInstalledPackages(output string) []aptPackage {
	packages := []aptPackage{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 4)
		if len(fields) < 4 {
			continue
		}
		status := strings.TrimSpace(fields[3])
		if !strings.Contains(status, "install ok installed") && !strings.HasPrefix(status, "ii") {
			continue
		}
		name := strings.TrimSpace(fields[0])
		if _, err := cleanAptPackageName(name); err != nil {
			continue
		}
		packages = append(packages, aptPackage{
			Name:         name,
			Version:      strings.TrimSpace(fields[1]),
			Architecture: strings.TrimSpace(fields[2]),
			Status:       "installed",
		})
	}
	sort.Slice(packages, func(left, right int) bool {
		return packages[left].Name < packages[right].Name
	})
	return packages
}

func parseAptUpgradeablePackages(output string) map[string]string {
	upgradeable := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Listing") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name, _, _ := strings.Cut(fields[0], "/")
		name = strings.TrimSpace(name)
		if _, err := cleanAptPackageName(name); err != nil {
			continue
		}
		upgradeable[name] = strings.TrimSpace(fields[1])
	}
	return upgradeable
}

func parseAptSearchResults(output string, installed map[string]aptPackage, upgradeable map[string]string) []aptSearchResult {
	results := []aptSearchResult{}
	seen := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, description, ok := strings.Cut(line, " - ")
		if !ok {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			name = fields[0]
			description = strings.TrimSpace(strings.TrimPrefix(line, name))
		}
		name = strings.TrimSpace(name)
		if _, err := cleanAptPackageName(name); err != nil {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true

		installedPackage, isInstalled := installed[name]
		candidate := upgradeable[name]
		results = append(results, aptSearchResult{
			Name:             name,
			Description:      strings.TrimSpace(description),
			Installed:        isInstalled,
			InstalledVersion: installedPackage.Version,
			Upgradeable:      candidate != "",
			CandidateVersion: candidate,
		})
	}
	sort.Slice(results, func(left, right int) bool {
		if results[left].Installed != results[right].Installed {
			return !results[left].Installed
		}
		return results[left].Name < results[right].Name
	})
	return results
}

func cleanAptSearchQuery(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > aptPackageSearchMaxLen {
		return "", fmt.Errorf("package search is too long")
	}
	for _, ch := range value {
		if ch == 0 || ch == '\n' || ch == '\r' || ch == '\t' {
			return "", fmt.Errorf("package search contains unsupported whitespace")
		}
	}
	return value, nil
}

func cleanAptPackageName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("package name is required")
	}
	if len(value) > aptPackageNameMaxLen {
		return "", fmt.Errorf("package name is too long")
	}
	if strings.HasPrefix(value, "-") {
		return "", fmt.Errorf("package name must not start with '-'")
	}
	for _, ch := range value {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '+' || ch == '.' || ch == '_' || ch == ':' || ch == '-' {
			continue
		}
		return "", fmt.Errorf("invalid package name: %s", value)
	}
	return value, nil
}

func aptToolsStatus() map[string]any {
	return map[string]any{
		"apt":       commandAvailable("apt"),
		"aptCache":  commandAvailable("apt-cache"),
		"aptGet":    commandAvailable("apt-get"),
		"dpkgQuery": commandAvailable("dpkg-query"),
	}
}

func commandAvailable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func runPackageReadCommand(timeout time.Duration, outputLimit int, command string, args ...string) (string, error) {
	result := runPackageCommand(timeout, outputLimit, command, args...)
	if asString(result["status"]) != "ok" {
		return "", fmt.Errorf("%s failed: %s", command, firstNonEmpty(asString(result["stderr"]), asString(result["stdout"]), "no output"))
	}
	return asString(result["stdout"]), nil
}

func runAptPackageManager(cfg Config, action string, packageName string, args []string) map[string]any {
	commandLine := append([]string{"apt-get"}, args...)
	if cfg.DevAllowHost {
		return map[string]any{
			"action":   action,
			"package":  packageName,
			"command":  strings.Join(commandLine, " "),
			"status":   "dev-skip",
			"exitCode": 0,
			"stdout":   "",
			"stderr":   "dev-allow-host is set; apt-get mutation skipped",
			"timedOut": false,
		}
	}
	if !commandAvailable("apt-get") {
		return map[string]any{
			"action":   action,
			"package":  packageName,
			"command":  strings.Join(commandLine, " "),
			"status":   "missing",
			"exitCode": -1,
			"stdout":   "",
			"stderr":   "apt-get is not available on this OS",
			"timedOut": false,
		}
	}
	result := runPackageCommand(aptActionTimeout, aptActionOutputLimit, "apt-get", args...)
	result["action"] = action
	result["package"] = packageName
	result["command"] = strings.Join(commandLine, " ")
	return result
}

func runPackageCommand(timeout time.Duration, outputLimit int, command string, args ...string) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stdout := &limitedBuffer{limit: outputLimit}
	stderr := &limitedBuffer{limit: outputLimit}
	cmd := exec.CommandContext(ctx, command, args...)
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
