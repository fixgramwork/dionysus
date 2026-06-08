package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	loadedModels := []any{}
	var loadedModelNames []string
	for _, item := range asSlice(ps.Data["models"]) {
		model := asMap(item)
		details := asMap(model["details"])
		name := firstNonEmpty(asString(model["model"]), asString(model["name"]))
		loadedModelNames = append(loadedModelNames, name, asString(model["name"]))
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

	models := []any{}
	for _, item := range asSlice(tags.Data["models"]) {
		model := asMap(item)
		details := asMap(model["details"])
		name := asString(model["name"])
		models = append(models, map[string]any{
			"name":       name,
			"size":       asInt64(model["size"]),
			"digest":     asString(model["digest"]),
			"modifiedAt": asString(model["modified_at"]),
			"running":    ollamaModelNameMatches(name, loadedModelNames),
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

func applyOllamaServiceAction(cfg Config, body map[string]any) (map[string]any, error) {
	action := jsonString(body, "action", "")
	if err := validateChoice(action, []string{"start", "stop", "restart"}, "Ollama service action"); err != nil {
		return nil, err
	}

	command := fmt.Sprintf("systemctl %s ollama.service", action)
	if cfg.DevAllowHost {
		return map[string]any{
			"action":  action,
			"command": command,
			"status":  "dev-skip",
			"reason":  "development host override is enabled; service action was not executed",
			"service": service("ollama"),
		}, nil
	}

	output, err := exec.Command("systemctl", action, "ollama.service").CombinedOutput()
	result := map[string]any{
		"action":  action,
		"command": command,
		"service": service("ollama"),
	}
	if err != nil {
		result["status"] = "failed"
		result["error"] = strings.TrimSpace(string(output))
		return result, nil
	}
	result["status"] = map[string]string{
		"start":   "started",
		"stop":    "stopped",
		"restart": "restarted",
	}[action]
	result["exitCode"] = 0
	result["stdout"] = strings.TrimSpace(string(output))
	result["stderr"] = ""
	return result, nil
}

func runOllamaModel(cfg Config, body map[string]any) (map[string]any, error) {
	model, err := cleanOllamaModelName(jsonString(body, "model", ""))
	if err != nil {
		return nil, err
	}
	keepAlive, err := cleanOllamaKeepAlive(jsonString(body, "keepAlive", "30m"))
	if err != nil {
		return nil, err
	}
	data, err := ollamaPostJSON(cfg.OllamaAPI, "/api/generate", map[string]any{
		"model":      model,
		"prompt":     "",
		"stream":     false,
		"keep_alive": keepAlive,
	}, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"action":    "run",
		"model":     model,
		"keepAlive": keepAlive,
		"status":    "requested",
		"api":       data,
		"ollama":    ollamaStatus(cfg),
	}, nil
}

func stopOllamaModel(cfg Config, body map[string]any) (map[string]any, error) {
	model, err := cleanOllamaModelName(jsonString(body, "model", ""))
	if err != nil {
		return nil, err
	}
	data, err := ollamaPostJSON(cfg.OllamaAPI, "/api/generate", map[string]any{
		"model":      model,
		"prompt":     "",
		"stream":     false,
		"keep_alive": 0,
	}, time.Minute)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"action": "stop",
		"model":  model,
		"status": "requested",
		"api":    data,
		"ollama": ollamaStatus(cfg),
	}, nil
}

func pullOllamaModel(cfg Config, body map[string]any) (map[string]any, error) {
	model, err := cleanOllamaModelName(jsonString(body, "model", ""))
	if err != nil {
		return nil, err
	}
	data, err := ollamaPostJSON(cfg.OllamaAPI, "/api/pull", map[string]any{
		"name":   model,
		"stream": false,
	}, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"action": "pull",
		"model":  model,
		"status": firstNonEmpty(asString(data["status"]), "pulled"),
		"api":    data,
		"ollama": ollamaStatus(cfg),
	}, nil
}

func searchOllamaLibrary(query string) map[string]any {
	cleaned := strings.TrimSpace(query)
	source := "fallback"
	results := fallbackOllamaLibraryResults(cleaned)
	liveResults, err := fetchOllamaLibrarySearch(cleaned)
	if err == nil && len(liveResults) > 0 {
		source = "ollama.com"
		results = mergeOllamaLibraryResults(liveResults, results)
	}
	payload := map[string]any{
		"query":   cleaned,
		"source":  source,
		"results": results,
	}
	if err != nil {
		payload["warning"] = err.Error()
	}
	return payload
}

func fetchOllamaLibrarySearch(query string) ([]any, error) {
	cleaned, err := cleanOllamaSearchQuery(query)
	if err != nil {
		return nil, err
	}
	if cleaned == "" {
		cleaned = "llama"
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("https://ollama.com/search?q=" + url.QueryEscape(cleaned))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama library search returned http status %d", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		return nil, err
	}
	results := parseOllamaSearchHTML(string(content))
	if len(results) == 0 {
		return nil, fmt.Errorf("ollama library search returned no models")
	}
	return results, nil
}

func parseOllamaSearchHTML(content string) []any {
	pattern := regexp.MustCompile(`href=["']/library/([A-Za-z0-9][A-Za-z0-9._-]*)["']`)
	matches := pattern.FindAllStringSubmatch(content, -1)
	seen := map[string]bool{}
	var results []any
	for _, match := range matches {
		name := match[1]
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		results = append(results, map[string]any{
			"name":        name,
			"title":       name,
			"pullName":    name,
			"description": "Ollama library model",
			"source":      "ollama.com",
		})
		if len(results) >= 24 {
			break
		}
	}
	return results
}

func fallbackOllamaLibraryResults(query string) []any {
	catalog := []map[string]string{
		{"name": "llama3.2", "title": "llama3.2", "description": "Meta Llama 3.2 general-purpose text model"},
		{"name": "llama3.1", "title": "llama3.1", "description": "Meta Llama 3.1 instruction model"},
		{"name": "gemma3", "title": "gemma3", "description": "Google Gemma 3 family"},
		{"name": "qwen3", "title": "qwen3", "description": "Alibaba Qwen 3 instruction model"},
		{"name": "deepseek-r1", "title": "deepseek-r1", "description": "DeepSeek R1 reasoning model"},
		{"name": "mistral", "title": "mistral", "description": "Mistral general-purpose model"},
		{"name": "phi4", "title": "phi4", "description": "Microsoft Phi 4 compact reasoning model"},
		{"name": "nomic-embed-text", "title": "nomic-embed-text", "description": "Text embedding model"},
		{"name": "codellama", "title": "codellama", "description": "Code-focused Llama model"},
		{"name": "tinyllama", "title": "tinyllama", "description": "Small local model for low-memory targets"},
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var results []any
	for _, item := range catalog {
		if needle != "" && !strings.Contains(strings.ToLower(item["name"]+" "+item["description"]), needle) {
			continue
		}
		results = append(results, map[string]any{
			"name":        item["name"],
			"title":       item["title"],
			"pullName":    item["name"],
			"description": item["description"],
			"source":      "fallback",
		})
	}
	if len(results) == 0 && needle != "" {
		results = append(results, map[string]any{
			"name":        query,
			"title":       query,
			"pullName":    query,
			"description": "Direct model name",
			"source":      "direct",
		})
	}
	return results
}

func mergeOllamaLibraryResults(primary []any, fallback []any) []any {
	seen := map[string]bool{}
	var merged []any
	for _, items := range [][]any{primary, fallback} {
		for _, item := range items {
			name := strings.ToLower(asString(asMap(item)["pullName"]))
			if name == "" {
				name = strings.ToLower(asString(asMap(item)["name"]))
			}
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			merged = append(merged, item)
		}
	}
	return merged
}

func ollamaPostJSON(base string, endpoint string, payload map[string]any, timeout time.Duration) (map[string]any, error) {
	var requestBody bytes.Buffer
	if err := json.NewEncoder(&requestBody).Encode(payload); err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, base+endpoint, &requestBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: timeout}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(content))
		if message == "" {
			message = fmt.Sprintf("http status %d", response.StatusCode)
		}
		return nil, fmt.Errorf("ollama %s failed: %s", endpoint, message)
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return map[string]any{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(content, &data); err != nil {
		return map[string]any{"raw": strings.TrimSpace(string(content))}, nil
	}
	return data, nil
}

func cleanOllamaModelName(value string) (string, error) {
	model := strings.TrimSpace(value)
	if model == "" {
		return "", fmt.Errorf("Ollama model name is required")
	}
	if len(model) > 200 {
		return "", fmt.Errorf("Ollama model name is too long")
	}
	for _, ch := range model {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-' || ch == ':' || ch == '/') {
			return "", fmt.Errorf("invalid Ollama model name: %s", model)
		}
	}
	if strings.HasPrefix(model, "/") || strings.HasSuffix(model, "/") || strings.Contains(model, "//") {
		return "", fmt.Errorf("invalid Ollama model name: %s", model)
	}
	for _, segment := range strings.Split(model, "/") {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("invalid Ollama model name: %s", model)
		}
	}
	return model, nil
}

func cleanOllamaKeepAlive(value string) (string, error) {
	keepAlive := strings.TrimSpace(value)
	if keepAlive == "" {
		return "30m", nil
	}
	if len(keepAlive) > 32 {
		return "", fmt.Errorf("Ollama keepAlive is too long")
	}
	for _, ch := range keepAlive {
		if !(ch >= '0' && ch <= '9' || ch == 'h' || ch == 'm' || ch == 's') {
			return "", fmt.Errorf("invalid Ollama keepAlive value: %s", keepAlive)
		}
	}
	return keepAlive, nil
}

func cleanOllamaSearchQuery(value string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if len(cleaned) > 120 {
		return "", fmt.Errorf("Ollama search query is too long")
	}
	if strings.ContainsAny(cleaned, "\n\r\x00") {
		return "", fmt.Errorf("Ollama search query must be a single line")
	}
	return cleaned, nil
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
		if !isOllamaProcess(comm, cmdline) {
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

func isOllamaProcess(comm string, cmdline string) bool {
	name := strings.ToLower(strings.TrimSpace(comm))
	if name == "ollama" || name == "llama-server" {
		return true
	}
	fields := strings.Fields(cmdline)
	if len(fields) == 0 {
		return false
	}
	executable := strings.ToLower(filepath.Base(fields[0]))
	return executable == "ollama" || executable == "llama-server" || strings.HasPrefix(executable, "ollama-") || strings.HasPrefix(executable, "llama-")
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

func ollamaModelNameMatches(name string, candidates []string) bool {
	targets := ollamaModelAliases(name)
	for _, candidate := range candidates {
		aliases := ollamaModelAliases(candidate)
		for _, target := range targets {
			for _, alias := range aliases {
				if target != "" && target == alias {
					return true
				}
			}
		}
	}
	return false
}

func ollamaModelAliases(value string) []string {
	cleaned := strings.ToLower(strings.TrimSpace(value))
	if cleaned == "" {
		return nil
	}
	aliases := []string{cleaned}
	if strings.HasSuffix(cleaned, ":latest") {
		aliases = append(aliases, strings.TrimSuffix(cleaned, ":latest"))
	} else if !strings.Contains(cleaned, ":") {
		aliases = append(aliases, cleaned+":latest")
	}
	return aliases
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
