package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParsesOllamaPSPayload(t *testing.T) {
	tags := OllamaGet{
		Reachable: true,
		Status:    200,
		Data: map[string]any{
			"models": []any{map[string]any{"name": "gemma3", "size": float64(10), "digest": "abc", "modified_at": "now"}},
		},
	}
	version := OllamaGet{Reachable: true, Status: 200, Data: map[string]any{"version": "0.23.3"}}
	ps := OllamaGet{
		Reachable: true,
		Status:    200,
		Data: map[string]any{
			"models": []any{map[string]any{"name": "gemma3", "model": "gemma3", "size": float64(100), "size_vram": float64(80), "context_length": float64(4096)}},
		},
	}
	parsed := parseOllamaAPI(tags, version, ps)
	if parsed["version"] != "0.23.3" {
		t.Fatalf("version mismatch: %v", parsed["version"])
	}
	if asString(asMap(asSlice(parsed["models"])[0])["name"]) != "gemma3" {
		t.Fatalf("model name mismatch: %#v", parsed["models"])
	}
	loaded := asMap(asSlice(parsed["loadedModels"])[0])
	if asInt64(loaded["sizeVram"]) != 80 || asInt64(loaded["contextLength"]) != 4096 {
		t.Fatalf("loaded model mismatch: %#v", loaded)
	}
}

func TestAuthRequiresExistingTokenFile(t *testing.T) {
	root := uniqueTempDir(t, "auth")
	cfg := testConfig(root)
	request, _ := http.NewRequest(http.MethodGet, "/api2/json/version", nil)
	if err := authorize(cfg, request); err != nil {
		t.Fatalf("auth should be open without token file: %v", err)
	}
	if err := os.WriteFile(cfg.TokenFile, []byte("secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, cfg.AuthUserFile, "root\n")
	writeFile(t, cfg.AuthPasswordFile, "dionysus\n")
	if err := authorize(cfg, request); err == nil {
		t.Fatal("auth should reject missing bearer token")
	}
	request.Header.Set("Authorization", "Bearer wrong")
	if err := authorize(cfg, request); err == nil {
		t.Fatal("auth should reject malformed jwt")
	}

	token, _, err := issueJWT(cfg, defaultAuthUsername, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if err := authorize(cfg, request); err != nil {
		t.Fatalf("auth should accept valid jwt: %v", err)
	}
}

func TestLoginTicketIssuesJWT(t *testing.T) {
	root := uniqueTempDir(t, "login-ticket")
	cfg := testConfig(root)
	writeFile(t, cfg.TokenFile, "secret\n")
	writeFile(t, cfg.AuthUserFile, "admin\n")
	writeFile(t, cfg.AuthPasswordFile, "dionysus\n")

	if _, err := loginTicket(cfg, map[string]any{"username": "admin", "password": "wrong"}); err == nil {
		t.Fatal("login should reject wrong password")
	}

	ticket, err := loginTicket(cfg, map[string]any{"username": "admin", "password": "dionysus"})
	if err != nil {
		t.Fatal(err)
	}
	if asString(ticket["tokenType"]) != "Bearer" || asString(ticket["username"]) != "admin" {
		t.Fatalf("login ticket mismatch: %#v", ticket)
	}

	request, _ := http.NewRequest(http.MethodGet, "/api2/json/version", nil)
	request.Header.Set("Authorization", "Bearer "+asString(ticket["token"]))
	if err := authorize(cfg, request); err != nil {
		t.Fatalf("auth should accept login jwt: %v", err)
	}
}

func TestManagesAuthUsers(t *testing.T) {
	root := uniqueTempDir(t, "auth-users")
	cfg := testConfig(root)
	writeFile(t, cfg.TokenFile, "secret\n")
	writeFile(t, cfg.AuthUserFile, "root\n")
	writeFile(t, cfg.AuthPasswordFile, "dionysus\n")

	user, err := createAuthUser(cfg, map[string]any{"username": "ops", "password": "secret123"})
	if err != nil {
		t.Fatal(err)
	}
	if asString(user["username"]) != "ops" {
		t.Fatalf("created user mismatch: %#v", user)
	}
	if !fileExists(cfg.AuthUsersFile) {
		t.Fatal("users file should be created")
	}

	if _, err := loginTicket(cfg, map[string]any{"username": "ops", "password": "secret123"}); err != nil {
		t.Fatalf("new user should login: %v", err)
	}
	if _, err := updateAuthUserPassword(cfg, map[string]any{"username": "ops", "password": "secret456"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loginTicket(cfg, map[string]any{"username": "ops", "password": "secret123"}); err == nil {
		t.Fatal("old password should be rejected")
	}
	if _, err := loginTicket(cfg, map[string]any{"username": "ops", "password": "secret456"}); err != nil {
		t.Fatalf("updated password should login: %v", err)
	}

	rootToken, _, err := issueJWT(cfg, "root", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodPost, "/api2/json/access/users/delete", nil)
	request.Header.Set("Authorization", "Bearer "+rootToken)
	session, err := currentSession(cfg, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deleteAuthUser(cfg, asString(session["username"]), map[string]any{"username": "ops"}); err != nil {
		t.Fatal(err)
	}
	if _, err := loginTicket(cfg, map[string]any{"username": "ops", "password": "secret456"}); err == nil {
		t.Fatal("deleted user should not login")
	}
}

func TestParsesProcMemoryAndOllamaProcesses(t *testing.T) {
	root := uniqueTempDir(t, "proc")
	mkdirAll(t, filepath.Join(root, "proc/123"))
	mkdirAll(t, filepath.Join(root, "proc/sys/kernel"))
	mkdirAll(t, filepath.Join(root, "etc"))
	writeFile(t, filepath.Join(root, "proc/meminfo"), "MemTotal: 1024 kB\nMemAvailable: 256 kB\nMemFree: 128 kB\nCached: 64 kB\nSwapTotal: 512 kB\nSwapFree: 128 kB\nSwapCached: 16 kB\n")
	writeFile(t, filepath.Join(root, "proc/uptime"), "10.50 20.00\n")
	writeFile(t, filepath.Join(root, "proc/loadavg"), "0.01 0.02 0.03 1/2 3\n")
	writeFile(t, filepath.Join(root, "proc/sys/kernel/hostname"), "dionysus-test\n")
	writeFile(t, filepath.Join(root, "proc/sys/kernel/ostype"), "Linux\n")
	writeFile(t, filepath.Join(root, "proc/sys/kernel/osrelease"), "6.12.0-test\n")
	writeFile(t, filepath.Join(root, "proc/sys/kernel/version"), "#1 SMP PREEMPT\n")
	writeFile(t, filepath.Join(root, "etc/os-release"), "NAME=\"Dionysus Test OS\"\nPRETTY_NAME=\"Dionysus Test OS 0.1\"\nID=dionysus\nVERSION_ID=0.1\n")
	writeFile(t, filepath.Join(root, "proc/123/comm"), "ollama\n")
	writeFileBytes(t, filepath.Join(root, "proc/123/cmdline"), []byte("ollama\x00serve\x00"))
	writeFile(t, filepath.Join(root, "proc/123/status"), "Name:\tollama\nVmRSS:\t100 kB\nVmHWM:\t120 kB\nVmSwap:\t30 kB\n")

	cfg := testConfig(root)
	status := nodeStatus(cfg)
	if asInt64(asMap(status["memory"])["total"]) != 1024*1024 {
		t.Fatalf("memory total mismatch: %#v", status["memory"])
	}
	if asInt64(asMap(status["memory"])["used"]) != 768*1024 {
		t.Fatalf("memory used mismatch: %#v", status["memory"])
	}
	if asInt64(asMap(status["swap"])["used"]) != 384*1024 {
		t.Fatalf("swap used mismatch: %#v", status["swap"])
	}
	if loadavg := status["loadavg"].([]string); len(loadavg) == 0 || loadavg[0] != "0.01" {
		t.Fatalf("loadavg mismatch: %#v", status["loadavg"])
	}
	if asString(asMap(status["os"])["hostname"]) != "dionysus-test" {
		t.Fatalf("hostname mismatch: %#v", status["os"])
	}
	processes := ollamaProcesses(cfg.ProcRoot)
	if len(processes) != 1 {
		t.Fatalf("process count mismatch: %#v", processes)
	}
	process := asMap(processes[0])
	if asInt64(process["pid"]) != 123 || asInt64(process["rss"]) != 100*1024 || asInt64(process["swap"]) != 30*1024 {
		t.Fatalf("process mismatch: %#v", process)
	}
}

func TestFreeCommandStatusRunsFreeHumanReadable(t *testing.T) {
	root := uniqueTempDir(t, "free-command")
	binDir := filepath.Join(root, "bin")
	freePath := filepath.Join(binDir, "free")
	writeFile(t, freePath, "#!/bin/sh\nprintf '               total        used        free\\nMem:           744Mi        48Mi       698Mi\\n'\n")
	if err := os.Chmod(freePath, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	status := freeCommandStatus()
	if !asBool(status["ok"]) {
		t.Fatalf("free command should succeed: %#v", status)
	}
	if asString(status["command"]) != "free -h" {
		t.Fatalf("free command label mismatch: %#v", status)
	}
	if !strings.Contains(asString(status["stdout"]), "Mem:") {
		t.Fatalf("free output missing memory row: %#v", status)
	}
}

func TestCPUUsageDeltaFromProcStat(t *testing.T) {
	first, ok := parseCPUTimes("cpu  100 0 50 850 0 0 0 0 0 0\ncpu0 50 0 25 425 0 0 0 0 0 0\ncpu1 50 0 25 425 0 0 0 0 0 0\n")
	if !ok || first.cores != 2 {
		t.Fatalf("first cpu sample mismatch: %#v", first)
	}
	second, ok := parseCPUTimes("cpu  130 0 70 900 0 0 0 0 0 0\ncpu0 65 0 35 450 0 0 0 0 0 0\ncpu1 65 0 35 450 0 0 0 0 0 0\n")
	if !ok || second.cores != 2 {
		t.Fatalf("second cpu sample mismatch: %#v", second)
	}

	usage := cpuUsage(first, second)
	if got := usage["usedPercent"].(float64); got != 50 {
		t.Fatalf("cpu used percent mismatch: %#v", usage)
	}
	if got := usage["userPercent"].(float64); got != 30 {
		t.Fatalf("cpu user percent mismatch: %#v", usage)
	}
	if got := usage["systemPercent"].(float64); got != 20 {
		t.Fatalf("cpu system percent mismatch: %#v", usage)
	}
	if asInt64(asMap(usage["ticks"])["total"]) != 1100 {
		t.Fatalf("cpu ticks mismatch: %#v", usage["ticks"])
	}
}

func TestReadsARMCPUModelFromCPUInfo(t *testing.T) {
	root := uniqueTempDir(t, "arm-cpuinfo")
	procRoot := filepath.Join(root, "proc")
	writeFile(t, filepath.Join(procRoot, "cpuinfo"), "processor\t: 0\nBogoMIPS\t: 125.00\nFeatures\t: fp asimd aes pmull sha1 sha2 crc32 cpuid\nCPU implementer\t: 0x41\nCPU architecture: 8\nCPU variant\t: 0x1\nCPU part\t: 0xd07\nCPU revision\t: 0\n\nprocessor\t: 1\nBogoMIPS\t: 125.00\nCPU implementer\t: 0x41\nCPU architecture: 8\nCPU part\t: 0xd07\n")

	if got := readCPUModel(procRoot); got != "ARM Cortex-A57 (ARMv8)" {
		t.Fatalf("arm cpu model mismatch: %q", got)
	}
}

func TestConsoleCommandReturnsOutputAndExitCode(t *testing.T) {
	root := uniqueTempDir(t, "console")
	result, err := runConsoleCommand(testConfig(root), map[string]any{
		"command": "printf 'hello'; printf 'warn' >&2; exit 7",
		"cwd":     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if asString(result["stdout"]) != "hello" || asString(result["stderr"]) != "warn" {
		t.Fatalf("console output mismatch: %#v", result)
	}
	if asInt64(result["exitCode"]) != 7 {
		t.Fatalf("console exit code mismatch: %#v", result)
	}
}

func TestConsoleCommandTimeoutKillsNestedChild(t *testing.T) {
	root := uniqueTempDir(t, "console-timeout")
	cfg := testConfig(root)
	cfg.ConsoleTimeoutSeconds = 1

	started := time.Now()
	result, err := runConsoleCommand(cfg, map[string]any{
		"command": "sh -c 'sleep 10'",
		"cwd":     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !asBool(result["timedOut"]) {
		t.Fatalf("console command should time out: %#v", result)
	}
	if time.Since(started) > 4*time.Second {
		t.Fatalf("console timeout did not stop nested child quickly: %#v", result)
	}
}

func TestTargetOSGuardRequiresRuntimeUnlessDevOverrideIsExplicit(t *testing.T) {
	root := uniqueTempDir(t, "runtime-guard")
	cfg := testConfig(root)
	if err := ensureTargetOSRuntime(cfg); err == nil {
		t.Fatal("runtime guard should reject a fake target OS")
	}
	cfg.DevAllowHost = true
	if err := ensureTargetOSRuntime(cfg); err != nil {
		t.Fatalf("dev override should pass: %v", err)
	}
}

func TestNetworkConfigMasksWifiSecretAndParsesLAN(t *testing.T) {
	root := uniqueTempDir(t, "network-read")
	cfg := testConfig(root)
	writeFile(t, cfg.NetworkConfig, "DIONYSUS_NETWORK_ENABLED='1'\nDIONYSUS_NETWORK_MODE='wifi'\nDIONYSUS_LAN_ENABLED='1'\nDIONYSUS_LAN_IFACE='enp1s0'\nDIONYSUS_WIFI_ENABLED='1'\nDIONYSUS_WIFI_IFACE='wlan0'\nDIONYSUS_WIFI_SSID='office'\nDIONYSUS_WIFI_PSK='secret value'\n")
	payload := networkConfigPayload(cfg.NetworkConfig)
	if payload["mode"] != "wifi" {
		t.Fatalf("mode mismatch: %#v", payload)
	}
	if asString(asMap(payload["lan"])["iface"]) != "enp1s0" {
		t.Fatalf("lan iface mismatch: %#v", payload["lan"])
	}
	wifi := asMap(payload["wifi"])
	if asString(wifi["ssid"]) != "office" || !asBool(wifi["pskStored"]) {
		t.Fatalf("wifi mismatch: %#v", wifi)
	}
	if _, exists := wifi["psk"]; exists {
		t.Fatal("wifi psk must not be returned")
	}
	if bytes.Contains([]byte(encodeJSON(payload)), []byte("secret value")) {
		t.Fatal("payload leaked wifi secret")
	}
}

func TestNetworkConfigDryRunPreservesSecretWithoutReturningIt(t *testing.T) {
	root := uniqueTempDir(t, "network-write")
	cfg := testConfig(root)
	original := "DIONYSUS_WIFI_PSK='secret value'\nDIONYSUS_WIFI_PSK_HASH=''\n"
	writeFile(t, cfg.NetworkConfig, original)
	body := map[string]any{
		"dryRun":         true,
		"apply":          true,
		"networkEnabled": true,
		"mode":           "lan",
		"lan": map[string]any{
			"enabled": true,
			"iface":   "eth0",
			"ipv4": map[string]any{
				"method":  "static",
				"address": "192.168.10.20/24",
				"gateway": "192.168.10.1",
				"dns":     "1.1.1.1 8.8.8.8",
			},
		},
		"wifi": map[string]any{
			"enabled":   false,
			"iface":     "wlan0",
			"country":   "kr",
			"ssid":      "office",
			"scanSsid":  false,
			"pskAction": "preserve",
			"ipv4":      map[string]any{"method": "dhcp"},
		},
		"dhcp":       map[string]any{"client": "auto", "tries": "6", "timeout": "5"},
		"resolvConf": "/etc/resolv.conf",
	}
	result, err := updateNetworkConfig(cfg, body)
	if err != nil {
		t.Fatal(err)
	}
	if !asBool(result["dryRun"]) || asBool(result["written"]) {
		t.Fatalf("dry run flags mismatch: %#v", result)
	}
	config := asMap(result["config"])
	if config["mode"] != "lan" || asString(asMap(asMap(config["lan"])["ipv4"])["address"]) != "192.168.10.20/24" {
		t.Fatalf("network config mismatch: %#v", config)
	}
	if !asBool(asMap(config["wifi"])["pskStored"]) {
		t.Fatalf("psk should be preserved: %#v", config["wifi"])
	}
	if bytes.Contains([]byte(encodeJSON(result)), []byte("secret value")) {
		t.Fatal("result leaked wifi secret")
	}
	if got := readTrim(cfg.NetworkConfig); got != strings.TrimSpace(original) {
		t.Fatalf("dry run should not write config: %q", got)
	}
}

func TestDryRunDoesNotWriteSysctl(t *testing.T) {
	root := uniqueTempDir(t, "dry-run")
	mkdirAll(t, filepath.Join(root, "proc/sys/vm"))
	mkdirAll(t, filepath.Join(root, "sys/kernel/mm/transparent_hugepage"))
	writeFile(t, filepath.Join(root, "proc/sys/vm/swappiness"), "30\n")
	writeFile(t, filepath.Join(root, "proc/sys/vm/page-cluster"), "1\n")
	writeFile(t, filepath.Join(root, "proc/sys/vm/vfs_cache_pressure"), "100\n")
	writeFile(t, filepath.Join(root, "proc/sys/vm/watermark_scale_factor"), "10\n")
	writeFile(t, filepath.Join(root, "sys/kernel/mm/transparent_hugepage/enabled"), "always [madvise] never\n")
	cfg := testConfig(root)
	profile := kvCacheProfileStatus(cfg)
	if len(asSlice(profile["targets"])) != 5 || asInt64(profile["missingCount"]) != 0 || asInt64(profile["pendingCount"]) == 0 {
		t.Fatalf("profile mismatch: %#v", profile)
	}
	result := applyOllamaProfile(cfg, true)
	if asString(asMap(asSlice(result["writes"])[0])["status"]) != "dry-run" || result["status"] != "dry-run" {
		t.Fatalf("dry run mismatch: %#v", result)
	}
	if readTrim(filepath.Join(root, "proc/sys/vm/swappiness")) != "30" {
		t.Fatal("dry run wrote sysctl")
	}
	result = applyOllamaProfile(cfg, false)
	if asString(asMap(asSlice(result["writes"])[0])["status"]) != "written" {
		t.Fatalf("write mismatch: %#v", result)
	}
	if readTrim(filepath.Join(root, "proc/sys/vm/swappiness")) != "80" {
		t.Fatal("apply did not write sysctl")
	}
}

func TestMetricsRetentionKeepsRecentRows(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	root := uniqueTempDir(t, "metrics")
	cfg := testConfig(root)
	old := map[string]any{
		"collectedAt": now() - 8*24*60*60,
		"node":        map[string]any{"memory": map[string]any{"total": int64(1), "used": int64(1), "available": int64(0)}, "swap": map[string]any{"total": int64(1), "used": int64(1)}},
		"ollama":      map[string]any{"apiReachable": true, "runtime": map[string]any{"processCount": int64(1), "processRss": int64(1), "processSwap": int64(1)}, "loadedModelMemory": map[string]any{"count": int64(1), "size": int64(1), "sizeVram": int64(0)}},
	}
	recent := map[string]any{
		"collectedAt": now(),
		"node":        map[string]any{"memory": map[string]any{"total": int64(2), "used": int64(1), "available": int64(1)}, "swap": map[string]any{"total": int64(2), "used": int64(1)}},
		"ollama":      map[string]any{"apiReachable": true, "runtime": map[string]any{"processCount": int64(2), "processRss": int64(20), "processSwap": int64(3)}, "loadedModelMemory": map[string]any{"count": int64(2), "size": int64(100), "sizeVram": int64(80)}},
	}
	if err := initMetricsSchema(cfg.MetricsDB); err != nil {
		t.Fatal(err)
	}
	insertSampleForTest(t, cfg.MetricsDB, old, 7)
	insertSampleForTest(t, cfg.MetricsDB, recent, 7)
	history := metricsHistory(cfg.MetricsDB, 10)
	if len(history) != 1 {
		t.Fatalf("history length mismatch: %#v", history)
	}
	ollama := asMap(asMap(history[0])["ollama"])
	if asInt64(ollama["loadedModelCount"]) != 2 || asInt64(ollama["processRss"]) != 20 {
		t.Fatalf("history row mismatch: %#v", history[0])
	}
}

func insertSampleForTest(t *testing.T, db string, sample map[string]any, retentionDays int64) {
	t.Helper()
	summary := sampleSummary(sample)
	cutoff := asInt64(summary["collectedAt"]) - retentionDays*24*60*60
	sql := fmt.Sprintf(
		"INSERT INTO ollama_samples (collected_at, memory_total, memory_used, memory_available, swap_total, swap_used, ollama_reachable, process_count, process_rss, process_swap, loaded_model_count, loaded_model_size, loaded_model_vram, sample_json) VALUES (%d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %s); DELETE FROM ollama_samples WHERE collected_at < %d;",
		asInt64(summary["collectedAt"]),
		asInt64(summary["memoryTotal"]),
		asInt64(summary["memoryUsed"]),
		asInt64(summary["memoryAvailable"]),
		asInt64(summary["swapTotal"]),
		asInt64(summary["swapUsed"]),
		asInt64(summary["ollamaReachable"]),
		asInt64(summary["processCount"]),
		asInt64(summary["processRss"]),
		asInt64(summary["processSwap"]),
		asInt64(summary["loadedModelCount"]),
		asInt64(summary["loadedModelSize"]),
		asInt64(summary["loadedModelVram"]),
		sqlQuote(encodeJSON(sample)),
		cutoff,
	)
	if err := runSQLite(db, sql); err != nil {
		t.Fatal(err)
	}
}

func testConfig(root string) Config {
	return Config{
		Listen:           defaultProxyListen,
		ProcRoot:         filepath.Join(root, "proc"),
		SysRoot:          filepath.Join(root, "sys"),
		EtcRoot:          filepath.Join(root, "etc"),
		OllamaAPI:        defaultOllamaAPI,
		MetricsDB:        filepath.Join(root, "metrics.sqlite3"),
		TokenFile:        filepath.Join(root, "pve.token"),
		AuthUsersFile:    filepath.Join(root, "pve.users.json"),
		AuthUserFile:     filepath.Join(root, "pve.user"),
		AuthPasswordFile: filepath.Join(root, "pve.password"),
		AuthUsername:     defaultAuthUsername,
		WWWRoot:          root,
		NetworkConfig:    filepath.Join(root, "network.env"),
		IntervalSeconds:  30,
		RetentionDays:    7,
		JWTTTLSeconds:    defaultJWTTTLSeconds,
	}
}

func uniqueTempDir(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(os.TempDir(), fmt.Sprintf("dionysus-%s-%d-%d", name, os.Getpid(), now()))
	_ = os.RemoveAll(path)
	mkdirAll(t, path)
	t.Cleanup(func() {
		_ = os.RemoveAll(path)
	})
	return path
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	writeFileBytes(t, path, []byte(content))
}

func writeFileBytes(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
}
