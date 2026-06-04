package main

import (
	"os"
	"path/filepath"
	"runtime"
)

func nodeStatus(cfg Config) map[string]any {
	meminfo := readMeminfo(cfg.ProcRoot)
	total := getNum(meminfo, "MemTotal")
	available := getNum(meminfo, "MemAvailable")
	free := getNum(meminfo, "MemFree")
	cached := getNum(meminfo, "Cached")
	swapTotal := getNum(meminfo, "SwapTotal")
	swapFree := getNum(meminfo, "SwapFree")
	swapUsed := positive(swapTotal - swapFree)
	swapPercent := 0.0
	if swapTotal > 0 {
		swapPercent = float64(swapUsed) / float64(swapTotal) * 100
	}

	return map[string]any{
		"node":         "localhost",
		"os":           osStatus(cfg),
		"controlPlane": controlPlaneStatus(cfg),
		"cpu":          cpuStatus(cfg),
		"uptime":       readUptime(cfg.ProcRoot),
		"loadavg":      readLoadavg(cfg.ProcRoot),
		"memory": map[string]any{
			"total":     total,
			"free":      free,
			"available": available,
			"cached":    cached,
			"used":      positive(total - available),
		},
		"swap": map[string]any{
			"total":       swapTotal,
			"free":        swapFree,
			"used":        swapUsed,
			"cached":      getNum(meminfo, "SwapCached"),
			"usedPercent": swapPercent,
		},
		"localtime": now(),
	}
}

func osStatus(cfg Config) map[string]any {
	osRelease := readEnvFile(filepath.Join(cfg.EtcRoot, "os-release"))
	hostname := firstNonEmpty(
		readTrim(filepath.Join(cfg.ProcRoot, "sys/kernel/hostname")),
		readTrim(filepath.Join(cfg.EtcRoot, "hostname")),
		commandOutputValue("hostname"),
	)
	kernelName := firstNonEmpty(
		readTrim(filepath.Join(cfg.ProcRoot, "sys/kernel/ostype")),
		commandOutputValue("uname", "-s"),
	)
	kernelRelease := firstNonEmpty(
		readTrim(filepath.Join(cfg.ProcRoot, "sys/kernel/osrelease")),
		commandOutputValue("uname", "-r"),
	)
	kernelVersion := firstNonEmpty(
		readTrim(filepath.Join(cfg.ProcRoot, "sys/kernel/version")),
		commandOutputValue("uname", "-v"),
	)
	architecture := firstNonEmpty(commandOutputValue("uname", "-m"), runtime.GOARCH)
	osName := envString(osRelease, "NAME", "")
	prettyName := firstNonEmpty(envString(osRelease, "PRETTY_NAME", ""), osName, kernelName)

	return map[string]any{
		"hostname":   hostname,
		"name":       osName,
		"prettyName": prettyName,
		"id":         envString(osRelease, "ID", ""),
		"version":    envString(osRelease, "VERSION", ""),
		"versionId":  envString(osRelease, "VERSION_ID", ""),
		"kernel": map[string]any{
			"name":         kernelName,
			"release":      kernelRelease,
			"version":      kernelVersion,
			"architecture": architecture,
		},
	}
}

func controlPlaneStatus(cfg Config) map[string]any {
	mode := "target-os"
	if cfg.DevAllowHost {
		mode = "development"
	}
	return map[string]any{
		"stack":         "go-svelte-systemd",
		"api":           "api2-json",
		"runtimeMode":   mode,
		"listen":        cfg.Listen,
		"wwwRoot":       cfg.WWWRoot,
		"metricsDb":     cfg.MetricsDB,
		"networkConfig": cfg.NetworkConfig,
		"tokenAuth":     fileExists(cfg.TokenFile),
	}
}

func serviceStatus() []any {
	names := []string{
		"dionysus-pvedaemon",
		"dionysus-pveproxy",
		"dionysus-metricsd",
		"dionysus-network",
		"dionysus-llm-swap",
		"ollama",
		"docker",
		"pve-qemu-kvm",
		"lxc",
	}
	services := make([]any, 0, len(names))
	for _, name := range names {
		services = append(services, service(name))
	}
	return services
}

func service(name string) map[string]any {
	state := commandOutputValue("systemctl", "is-active", name)
	if state == "" {
		state = "unknown"
	}
	return map[string]any{"name": name, "state": state}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
