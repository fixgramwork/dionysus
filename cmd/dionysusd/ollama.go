package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type OllamaGet struct {
	Reachable bool
	Status    int
	Data      map[string]any
	Error     string
}

type KernelTarget struct {
	ID     string
	Label  string
	Path   string
	Value  string
	Reason string
}

func ollamaStatus(cfg Config) map[string]any {
	meminfo := readMeminfo(cfg.ProcRoot)
	swapTotal := getNum(meminfo, "SwapTotal")
	swapFree := getNum(meminfo, "SwapFree")
	swapUsed := positive(swapTotal - swapFree)
	processes := ollamaProcesses(cfg.ProcRoot)
	api := ollamaAPIStatus(cfg.OllamaAPI)
	serviceState := service("ollama")
	platform := ollamaPlatform(cfg.OllamaAPI, api, processes, serviceState)
	runtime := runtimeSummary(processes)
	loadedModelMemory := loadedModelSummary(asSlice(api["loadedModels"]))
	swapPercent := 0.0
	if swapTotal > 0 {
		swapPercent = float64(swapUsed) / float64(swapTotal) * 100
	}

	status := map[string]any{
		"platform":              platform,
		"apiBase":               cfg.OllamaAPI,
		"apiReachable":          api["reachable"],
		"apiStatus":             api["status"],
		"version":               api["version"],
		"running":               len(processes) > 0 || asBool(api["reachable"]) || serviceState["state"] == "active",
		"models":                api["models"],
		"loadedModels":          api["loadedModels"],
		"loadedModelsReachable": api["loadedModelsReachable"],
		"loadedModelsError":     api["loadedModelsError"],
		"processes":             processes,
		"runtime":               runtime,
		"loadedModelMemory":     loadedModelMemory,
		"service":               serviceState,
		"swap": map[string]any{
			"total":       swapTotal,
			"free":        swapFree,
			"used":        swapUsed,
			"cached":      getNum(meminfo, "SwapCached"),
			"usedPercent": swapPercent,
		},
		"kernel":  kernelTuning(cfg),
		"kvCache": kvCacheProfileStatus(cfg),
	}
	if errorText := asString(api["error"]); errorText != "" {
		status["error"] = errorText
	}
	status["recommendations"] = recommendations(status)
	return status
}

func ollamaAPIStatus(base string) map[string]any {
	tags := ollamaGetJSON(base, "/api/tags")
	version := ollamaGetJSON(base, "/api/version")
	ps := ollamaGetJSON(base, "/api/ps")
	return parseOllamaAPI(tags, version, ps)
}

func parseOllamaAPI(tags OllamaGet, version OllamaGet, ps OllamaGet) map[string]any {
	var models []any
	for _, item := range asSlice(tags.Data["models"]) {
		model := asMap(item)
		models = append(models, map[string]any{
			"name":       asString(model["name"]),
			"size":       asInt64(model["size"]),
			"digest":     asString(model["digest"]),
			"modifiedAt": asString(model["modified_at"]),
		})
	}

	var loadedModels []any
	for _, item := range asSlice(ps.Data["models"]) {
		model := asMap(item)
		details := asMap(model["details"])
		name := firstNonEmpty(asString(model["model"]), asString(model["name"]))
		loadedModels = append(loadedModels, map[string]any{
			"name":          asString(model["name"]),
			"model":         name,
			"size":          asInt64(model["size"]),
			"sizeVram":      asInt64(model["size_vram"]),
			"digest":        asString(model["digest"]),
			"expiresAt":     asString(model["expires_at"]),
			"contextLength": asInt64(model["context_length"]),
			"details": map[string]any{
				"format":            asString(details["format"]),
				"family":            asString(details["family"]),
				"parameterSize":     asString(details["parameter_size"]),
				"quantizationLevel": asString(details["quantization_level"]),
			},
		})
	}

	reachable := tags.Reachable || version.Reachable
	status := version.Status
	if tags.Reachable || tags.Status != 0 {
		status = tags.Status
	}
	var errors []string
	if tags.Error != "" {
		errors = append(errors, tags.Error)
	}
	if version.Error != "" {
		errors = append(errors, version.Error)
	}

	return map[string]any{
		"reachable":             reachable,
		"status":                status,
		"version":               asString(version.Data["version"]),
		"models":                models,
		"loadedModels":          loadedModels,
		"loadedModelsReachable": ps.Reachable,
		"loadedModelsError":     ps.Error,
		"error":                 strings.Join(errors, "; "),
	}
}

func applyOllamaProfile(cfg Config, dryRun bool) map[string]any {
	targets := kvCacheTargets(cfg)
	before := kvCacheProfileStatus(cfg)
	var writes []any
	for _, target := range targets {
		current := readTrim(target.Path)
		if !fileExists(target.Path) {
			writes = append(writes, map[string]any{
				"id":      target.ID,
				"label":   target.Label,
				"path":    target.Path,
				"current": current,
				"value":   target.Value,
				"status":  "skipped",
				"error":   "path does not exist",
			})
			continue
		}
		if dryRun {
			writes = append(writes, map[string]any{
				"id":      target.ID,
				"label":   target.Label,
				"path":    target.Path,
				"current": current,
				"value":   target.Value,
				"status":  "dry-run",
			})
			continue
		}
		if err := os.WriteFile(target.Path, []byte(target.Value), 0644); err != nil {
			writes = append(writes, map[string]any{
				"id":      target.ID,
				"label":   target.Label,
				"path":    target.Path,
				"current": current,
				"value":   target.Value,
				"status":  "failed",
				"error":   err.Error(),
			})
			continue
		}
		writes = append(writes, map[string]any{
			"id":      target.ID,
			"label":   target.Label,
			"path":    target.Path,
			"current": current,
			"value":   target.Value,
			"status":  "written",
		})
	}
	after := kvCacheProfileStatus(cfg)
	failed := countWriteStatus(writes, "failed")
	skipped := countWriteStatus(writes, "skipped")
	written := countWriteStatus(writes, "written")
	dry := countWriteStatus(writes, "dry-run")
	status := "no-op"
	if failed > 0 {
		status = "failed"
	} else if dryRun {
		status = "dry-run"
	} else if written > 0 {
		status = "applied"
	}

	return map[string]any{
		"profile":     "ollama-kv-cache",
		"description": "Manual kernel profile for bounded Ollama KV-cache overflow into Dionysus-managed swap.",
		"dryRun":      dryRun,
		"status":      status,
		"summary": map[string]any{
			"written":       written,
			"dryRun":        dry,
			"skipped":       skipped,
			"failed":        failed,
			"pendingBefore": asInt64(before["pendingCount"]),
			"pendingAfter":  asInt64(after["pendingCount"]),
			"ready":         asBool(after["ready"]),
		},
		"before":  before,
		"after":   after,
		"writes":  writes,
		"console": kvCacheConsole(status, dryRun, written, skipped, failed, after),
	}
}

func kvCacheTargets(cfg Config) []KernelTarget {
	return []KernelTarget{
		{
			ID:     "swappiness",
			Label:  "vm.swappiness",
			Path:   filepath.Join(cfg.ProcRoot, "sys/vm/swappiness"),
			Value:  "80",
			Reason: "prefer earlier bounded swap use when RAM pressure rises",
		},
		{
			ID:     "page-cluster",
			Label:  "vm.page-cluster",
			Path:   filepath.Join(cfg.ProcRoot, "sys/vm/page-cluster"),
			Value:  "0",
			Reason: "avoid swap readahead for random KV-cache access",
		},
		{
			ID:     "vfs-cache-pressure",
			Label:  "vm.vfs_cache_pressure",
			Path:   filepath.Join(cfg.ProcRoot, "sys/vm/vfs_cache_pressure"),
			Value:  "40",
			Reason: "keep model file cache warmer under memory pressure",
		},
		{
			ID:     "watermark-scale-factor",
			Label:  "vm.watermark_scale_factor",
			Path:   filepath.Join(cfg.ProcRoot, "sys/vm/watermark_scale_factor"),
			Value:  "125",
			Reason: "give reclaim more headroom before hard pressure",
		},
		{
			ID:     "transparent-hugepage",
			Label:  "transparent_hugepage/enabled",
			Path:   filepath.Join(cfg.SysRoot, "kernel/mm/transparent_hugepage/enabled"),
			Value:  "madvise",
			Reason: "avoid unconditional THP while preserving opt-in huge pages",
		},
	}
}

func kvCacheProfileStatus(cfg Config) map[string]any {
	var targets []any
	for _, target := range kvCacheTargets(cfg) {
		targets = append(targets, kvCacheTargetStatus(target))
	}
	pendingCount := 0
	missingCount := 0
	readyCount := 0
	for _, item := range targets {
		status := asString(asMap(item)["status"])
		switch status {
		case "pending":
			pendingCount++
		case "missing":
			missingCount++
		case "ready":
			readyCount++
		}
	}

	meminfo := readMeminfo(cfg.ProcRoot)
	swapTotal := getNum(meminfo, "SwapTotal")
	swapFree := getNum(meminfo, "SwapFree")
	swapUsed := positive(swapTotal - swapFree)
	swapPercent := 0.0
	if swapTotal > 0 {
		swapPercent = float64(swapUsed) / float64(swapTotal) * 100
	}
	swapService := service("dionysus-llm-swap")
	ready := pendingCount == 0 && missingCount == 0 && swapTotal > 0
	var warnings []any
	if swapTotal == 0 {
		warnings = append(warnings, map[string]any{
			"severity": "warning",
			"text":     "Dionysus-managed swap is not visible in /proc/meminfo.",
			"action":   "enable dionysus-llm-swap.service before heavy Ollama workloads",
		})
	}
	if missingCount > 0 {
		warnings = append(warnings, map[string]any{
			"severity": "warning",
			"text":     "Some kernel tuning paths are missing on this host.",
			"action":   "verify the target is a Linux systemd host with procfs/sysfs mounted",
		})
	}
	if pendingCount > 0 {
		warnings = append(warnings, map[string]any{
			"severity": "info",
			"text":     "KV-cache kernel profile has unapplied values.",
			"action":   "preview, then apply the Ollama KV-cache profile manually",
		})
	}

	return map[string]any{
		"profile":      "ollama-kv-cache",
		"ready":        ready,
		"readyCount":   readyCount,
		"pendingCount": pendingCount,
		"missingCount": missingCount,
		"targets":      targets,
		"swap": map[string]any{
			"service":     swapService,
			"total":       swapTotal,
			"free":        swapFree,
			"used":        swapUsed,
			"enabled":     swapTotal > 0,
			"usedPercent": swapPercent,
		},
		"warnings": warnings,
	}
}

func kvCacheTargetStatus(target KernelTarget) map[string]any {
	current := readTrim(target.Path)
	exists := fileExists(target.Path)
	status := "pending"
	if !exists {
		status = "missing"
	} else if kernelValueMatches(current, target.Value) {
		status = "ready"
	}
	return map[string]any{
		"id":      target.ID,
		"label":   target.Label,
		"path":    target.Path,
		"current": current,
		"target":  target.Value,
		"status":  status,
		"reason":  target.Reason,
	}
}

func kernelValueMatches(current string, target string) bool {
	if target == "madvise" {
		return current == "madvise" || strings.Contains(current, "[madvise]")
	}
	return current == target
}

func kvCacheConsole(status string, dryRun bool, written int, skipped int, failed int, after map[string]any) []any {
	level := "info"
	if failed > 0 {
		level = "error"
	} else if dryRun {
		level = "warn"
	}
	swap := asMap(after["swap"])
	return []any{
		map[string]any{"level": level, "message": fmt.Sprintf("ollama-kv-cache profile %s", status)},
		map[string]any{"level": "info", "message": fmt.Sprintf("writes=%d skipped=%d failed=%d pending=%d", written, skipped, failed, asInt64(after["pendingCount"]))},
		map[string]any{"level": "info", "message": fmt.Sprintf("swap=%d used=%d service=%s", asInt64(swap["total"]), asInt64(swap["used"]), asString(asMap(swap["service"])["state"]))},
	}
}

func kernelTuning(cfg Config) map[string]any {
	return map[string]any{
		"swappiness":           readTrim(filepath.Join(cfg.ProcRoot, "sys/vm/swappiness")),
		"pageCluster":          readTrim(filepath.Join(cfg.ProcRoot, "sys/vm/page-cluster")),
		"vfsCachePressure":     readTrim(filepath.Join(cfg.ProcRoot, "sys/vm/vfs_cache_pressure")),
		"watermarkScaleFactor": readTrim(filepath.Join(cfg.ProcRoot, "sys/vm/watermark_scale_factor")),
		"transparentHugepage":  readTrim(filepath.Join(cfg.SysRoot, "kernel/mm/transparent_hugepage/enabled")),
	}
}

func ollamaProcesses(procRoot string) []any {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil
	}
	var processes []any
	for _, entry := range entries {
		name := entry.Name()
		if !allDigits(name) {
			continue
		}
		dir := filepath.Join(procRoot, name)
		comm := readTrim(filepath.Join(dir, "comm"))
		cmdline := strings.ReplaceAll(readTrim(filepath.Join(dir, "cmdline")), "\x00", " ")
		search := strings.ToLower(comm + " " + cmdline)
		if !strings.Contains(search, "ollama") && !strings.Contains(search, "llama") {
			continue
		}
		status := processStatus(filepath.Join(dir, "status"))
		processes = append(processes, map[string]any{
			"pid":     asInt64FromString(name),
			"name":    comm,
			"command": cmdline,
			"rss":     getNum(status, "VmRSS"),
			"peakRss": getNum(status, "VmHWM"),
			"swap":    getNum(status, "VmSwap"),
		})
	}
	sort.Slice(processes, func(left, right int) bool {
		return asInt64(asMap(processes[left])["pid"]) < asInt64(asMap(processes[right])["pid"])
	})
	return processes
}

func processStatus(path string) map[string]int64 {
	values := map[string]int64{}
	content, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key == "VmRSS" || key == "VmHWM" || key == "VmSwap" {
			values[key] = asInt64FromString(fields[1]) * 1024
		}
	}
	return values
}

func ollamaPlatform(apiBase string, api map[string]any, processes []any, serviceState map[string]any) map[string]any {
	var sources []string
	if asBool(api["reachable"]) {
		sources = append(sources, "api")
	}
	if len(processes) > 0 {
		sources = append(sources, "process")
	}
	if serviceState["state"] == "active" {
		sources = append(sources, "systemd")
	}

	state := "not-detected"
	switch {
	case asBool(api["reachable"]):
		state = "api-online"
	case len(processes) > 0:
		state = "process-only"
	case serviceState["state"] == "active":
		state = "service-active"
	case serviceState["state"] != "unknown":
		state = "service-" + asString(serviceState["state"])
	}

	return map[string]any{
		"name":             "Ollama",
		"type":             "local-llm",
		"detected":         len(sources) > 0,
		"state":            state,
		"apiBase":          apiBase,
		"apiReachable":     api["reachable"],
		"apiStatus":        api["status"],
		"version":          api["version"],
		"service":          serviceState,
		"detectionSources": sources,
		"modelCount":       len(asSlice(api["models"])),
		"loadedModelCount": len(asSlice(api["loadedModels"])),
		"processCount":     len(processes),
	}
}

func runtimeSummary(processes []any) map[string]any {
	var processRSS int64
	var processPeak int64
	var processSwap int64
	for _, item := range processes {
		process := asMap(item)
		processRSS += asInt64(process["rss"])
		processPeak += asInt64(process["peakRss"])
		processSwap += asInt64(process["swap"])
	}
	return map[string]any{
		"processCount":   len(processes),
		"processRss":     processRSS,
		"processPeakRss": processPeak,
		"processSwap":    processSwap,
	}
}

func loadedModelSummary(models []any) map[string]any {
	var size int64
	var sizeVRAM int64
	for _, item := range models {
		model := asMap(item)
		size += asInt64(model["size"])
		sizeVRAM += asInt64(model["sizeVram"])
	}
	return map[string]any{"count": len(models), "size": size, "sizeVram": sizeVRAM}
}

func recommendations(status map[string]any) []any {
	var items []any
	platform := asMap(status["platform"])
	if !asBool(platform["detected"]) {
		items = append(items, map[string]any{
			"severity": "warning",
			"text":     "Ollama local LLM platform is not detected on this node.",
			"action":   "start ollama.service or set DIONYSUS_OLLAMA_API to the local Ollama endpoint",
		})
	} else if !asBool(status["apiReachable"]) {
		items = append(items, map[string]any{
			"severity": "warning",
			"text":     "Ollama was detected, but its HTTP API is not reachable.",
			"action":   "check ollama.service, OLLAMA_HOST, and DIONYSUS_OLLAMA_API",
		})
	}
	if asBool(status["apiReachable"]) && len(asSlice(status["models"])) == 0 {
		items = append(items, map[string]any{
			"severity": "info",
			"text":     "Ollama API is reachable, but no local models are listed.",
			"action":   "pull a model with ollama pull before serving workloads",
		})
	}
	swap := asMap(status["swap"])
	if asInt64(swap["total"]) == 0 {
		items = append(items, map[string]any{
			"severity": "warning",
			"text":     "LLM swap backing store is disabled.",
			"action":   "enable dionysus-llm-swap.service before starting ollama.service",
		})
	}
	kernel := asMap(status["kernel"])
	if asInt64FromString(asString(kernel["swappiness"])) < 60 {
		items = append(items, map[string]any{
			"severity": "info",
			"text":     "swappiness is conservative for KV-cache overflow.",
			"action":   "apply /api2/json/nodes/localhost/ollama/optimize",
		})
	}
	if asInt64FromString(asString(kernel["pageCluster"])) > 0 {
		items = append(items, map[string]any{
			"severity": "info",
			"text":     "swap readahead is enabled; KV-cache access is often random.",
			"action":   "set vm.page-cluster=0",
		})
	}
	if percent, ok := swap["usedPercent"].(float64); ok && percent >= 80 {
		items = append(items, map[string]any{
			"severity": "warning",
			"text":     "LLM swap backing store is close to full.",
			"action":   "increase DIONYSUS_LLM_SWAP_SIZE_MB or reduce concurrent model load",
		})
	}
	runtime := asMap(status["runtime"])
	if asInt64(runtime["processSwap"]) > 0 {
		items = append(items, map[string]any{
			"severity": "info",
			"text":     "Ollama processes are using swap.",
			"action":   "watch latency and keep swap as bounded overflow, not a speed path",
		})
	}
	return items
}

func ollamaGetJSON(base string, endpoint string) OllamaGet {
	result, err := httpGetJSON(base, endpoint)
	if err != nil {
		return OllamaGet{Data: map[string]any{}, Error: err.Error()}
	}
	return result
}

func httpGetJSON(base string, endpoint string) (OllamaGet, error) {
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(base + endpoint)
	if err != nil {
		return OllamaGet{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return OllamaGet{
			Reachable: false,
			Status:    response.StatusCode,
			Data:      map[string]any{},
			Error:     fmt.Sprintf("http status %d", response.StatusCode),
		}, nil
	}
	var data map[string]any
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return OllamaGet{}, fmt.Errorf("invalid json")
	}
	return OllamaGet{Reachable: true, Status: response.StatusCode, Data: data}, nil
}

func countWriteStatus(writes []any, status string) int {
	count := 0
	for _, item := range writes {
		if asString(asMap(item)["status"]) == status {
			count++
		}
	}
	return count
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func asInt64FromString(value string) int64 {
	var parsed int64
	_, _ = fmt.Sscan(value, &parsed)
	return parsed
}
