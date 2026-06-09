package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func networkStatus(cfg Config) map[string]any {
	routes := commandOutputValue("ip", "route")
	return map[string]any{
		"config":     networkConfigPayload(cfg.NetworkConfig),
		"service":    service("dionysus-network"),
		"interfaces": networkInterfaces(cfg),
		"routes":     routes,
		"resolvConf": readTrim("/etc/resolv.conf"),
	}
}

func networkConfigPayload(path string) map[string]any {
	env := readEnvFile(path)
	return networkConfigPayloadFromEnv(env, path)
}

func networkConfigPayloadFromEnv(env map[string]string, path string) map[string]any {
	wifiEnabled := envBoolValue(env, "DIONYSUS_WIFI_ENABLED", false)
	lanEnabled := envBoolValue(env, "DIONYSUS_LAN_ENABLED", false)
	mode := envString(env, "DIONYSUS_NETWORK_MODE", "")
	if mode == "" {
		switch {
		case wifiEnabled:
			mode = "wifi"
		case lanEnabled:
			mode = "lan"
		default:
			mode = "none"
		}
	}

	return map[string]any{
		"path":           path,
		"exists":         fileExists(path),
		"networkEnabled": envBoolValue(env, "DIONYSUS_NETWORK_ENABLED", true),
		"mode":           mode,
		"lan": map[string]any{
			"enabled": lanEnabled,
			"iface":   envString(env, "DIONYSUS_LAN_IFACE", "eth0"),
			"ipv4": map[string]any{
				"method":  envString(env, "DIONYSUS_LAN_IPV4_METHOD", "dhcp"),
				"address": envString(env, "DIONYSUS_LAN_IPV4_ADDRESS", ""),
				"gateway": envString(env, "DIONYSUS_LAN_IPV4_GATEWAY", ""),
				"dns":     envString(env, "DIONYSUS_LAN_DNS", ""),
			},
		},
		"wifi": map[string]any{
			"enabled":       wifiEnabled,
			"iface":         envString(env, "DIONYSUS_WIFI_IFACE", "wlan0"),
			"country":       envString(env, "DIONYSUS_WIFI_COUNTRY", "KR"),
			"ssid":          envString(env, "DIONYSUS_WIFI_SSID", ""),
			"scanSsid":      envBoolValue(env, "DIONYSUS_WIFI_SCAN_SSID", false),
			"pskStored":     env["DIONYSUS_WIFI_PSK"] != "",
			"pskHashStored": env["DIONYSUS_WIFI_PSK_HASH"] != "",
			"ipv4": map[string]any{
				"method":  envString(env, "DIONYSUS_WIFI_IPV4_METHOD", "dhcp"),
				"address": envString(env, "DIONYSUS_WIFI_IPV4_ADDRESS", ""),
				"gateway": envString(env, "DIONYSUS_WIFI_IPV4_GATEWAY", ""),
				"dns":     envString(env, "DIONYSUS_WIFI_DNS", ""),
			},
		},
		"dhcp": map[string]any{
			"client":  envString(env, "DIONYSUS_DHCP_CLIENT", "auto"),
			"tries":   envString(env, "DIONYSUS_DHCP_TRIES", "6"),
			"timeout": envString(env, "DIONYSUS_DHCP_TIMEOUT", "5"),
		},
		"resolvConf": envString(env, "DIONYSUS_RESOLV_CONF", "/etc/resolv.conf"),
	}
}

func updateNetworkConfig(cfg Config, body map[string]any) (map[string]any, error) {
	dryRun := jsonBool(body, "dryRun", false)
	apply := jsonBool(body, "apply", false)
	current := readEnvFile(cfg.NetworkConfig)
	next, err := networkEnvFromPayload(body, current)
	if err != nil {
		return nil, err
	}

	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(cfg.NetworkConfig), 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", filepath.Dir(cfg.NetworkConfig), err)
		}
		if err := os.WriteFile(cfg.NetworkConfig, []byte(renderNetworkEnv(next)), 0600); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", cfg.NetworkConfig, err)
		}
		_ = os.Chmod(cfg.NetworkConfig, 0600)
	}

	var applyResult any
	if apply {
		applyResult = applyNetworkService(cfg, dryRun)
	}

	return map[string]any{
		"dryRun":      dryRun,
		"written":     !dryRun,
		"apply":       apply,
		"config":      networkConfigPayloadFromEnv(next, cfg.NetworkConfig),
		"rendered":    "",
		"applyResult": applyResult,
	}, nil
}

func applyNetworkService(cfg Config, dryRun bool) map[string]any {
	if dryRun {
		return map[string]any{
			"dryRun":  true,
			"status":  "dry-run",
			"command": "systemctl restart dionysus-network.service",
		}
	}
	if cfg.DevAllowHost {
		return map[string]any{
			"dryRun":  false,
			"status":  "dev-skip",
			"command": "systemctl restart dionysus-network.service",
			"reason":  "development host override is enabled; service restart was not executed",
		}
	}
	output, err := newCommand("systemctl", "restart", "dionysus-network.service").CombinedOutput()
	if err != nil {
		return map[string]any{
			"dryRun": false,
			"status": "failed",
			"error":  strings.TrimSpace(string(output)),
		}
	}
	return map[string]any{
		"dryRun":   false,
		"status":   "restarted",
		"exitCode": 0,
		"stdout":   strings.TrimSpace(string(output)),
		"stderr":   "",
	}
}

func networkEnvFromPayload(body map[string]any, current map[string]string) (map[string]string, error) {
	mode := jsonString(body, "mode", envString(current, "DIONYSUS_NETWORK_MODE", "none"))
	if err := validateChoice(mode, []string{"none", "lan", "wifi"}, "network mode"); err != nil {
		return nil, err
	}
	networkEnabled := jsonBool(body, "networkEnabled", true)
	lan := object(body, "lan")
	wifi := object(body, "wifi")
	dhcp := object(body, "dhcp")
	lanIPv4 := object(lan, "ipv4")
	wifiIPv4 := object(wifi, "ipv4")

	lanIface := jsonString(lan, "iface", envString(current, "DIONYSUS_LAN_IFACE", "eth0"))
	wifiIface := jsonString(wifi, "iface", envString(current, "DIONYSUS_WIFI_IFACE", "wlan0"))
	if err := validateIface(lanIface); err != nil {
		return nil, err
	}
	if err := validateIface(wifiIface); err != nil {
		return nil, err
	}

	lanMethod := jsonString(lanIPv4, "method", envString(current, "DIONYSUS_LAN_IPV4_METHOD", "dhcp"))
	wifiMethod := jsonString(wifiIPv4, "method", envString(current, "DIONYSUS_WIFI_IPV4_METHOD", "dhcp"))
	if err := validateChoice(lanMethod, []string{"dhcp", "static", "none"}, "LAN IPv4 method"); err != nil {
		return nil, err
	}
	if err := validateChoice(wifiMethod, []string{"dhcp", "static", "none"}, "Wi-Fi IPv4 method"); err != nil {
		return nil, err
	}

	wifiCountry := strings.ToUpper(jsonString(wifi, "country", envString(current, "DIONYSUS_WIFI_COUNTRY", "KR")))
	if len(wifiCountry) != 2 || !isASCIIAlpha(wifiCountry) {
		return nil, fmt.Errorf("Wi-Fi country must be a two-letter country code")
	}

	next := map[string]string{
		"DIONYSUS_NETWORK_ENABLED":   boolEnv(networkEnabled),
		"DIONYSUS_NETWORK_MODE":      mode,
		"DIONYSUS_LAN_ENABLED":       boolEnv(jsonBool(lan, "enabled", mode == "lan")),
		"DIONYSUS_LAN_IFACE":         lanIface,
		"DIONYSUS_LAN_IPV4_METHOD":   lanMethod,
		"DIONYSUS_LAN_IPV4_ADDRESS":  mustSingleLine(jsonString(lanIPv4, "address", "")),
		"DIONYSUS_LAN_IPV4_GATEWAY":  mustSingleLine(jsonString(lanIPv4, "gateway", "")),
		"DIONYSUS_LAN_DNS":           mustSingleLine(jsonString(lanIPv4, "dns", "")),
		"DIONYSUS_WIFI_ENABLED":      boolEnv(jsonBool(wifi, "enabled", mode == "wifi")),
		"DIONYSUS_WIFI_IFACE":        wifiIface,
		"DIONYSUS_WIFI_COUNTRY":      wifiCountry,
		"DIONYSUS_WIFI_SSID":         mustSingleLine(jsonString(wifi, "ssid", "")),
		"DIONYSUS_WIFI_SCAN_SSID":    boolEnv(jsonBool(wifi, "scanSsid", false)),
		"DIONYSUS_WIFI_IPV4_METHOD":  wifiMethod,
		"DIONYSUS_WIFI_IPV4_ADDRESS": mustSingleLine(jsonString(wifiIPv4, "address", "")),
		"DIONYSUS_WIFI_IPV4_GATEWAY": mustSingleLine(jsonString(wifiIPv4, "gateway", "")),
		"DIONYSUS_WIFI_DNS":          mustSingleLine(jsonString(wifiIPv4, "dns", "")),
		"DIONYSUS_DHCP_CLIENT":       mustSingleLine(jsonString(dhcp, "client", envString(current, "DIONYSUS_DHCP_CLIENT", "auto"))),
		"DIONYSUS_DHCP_TRIES":        mustSingleLine(jsonString(dhcp, "tries", envString(current, "DIONYSUS_DHCP_TRIES", "6"))),
		"DIONYSUS_DHCP_TIMEOUT":      mustSingleLine(jsonString(dhcp, "timeout", envString(current, "DIONYSUS_DHCP_TIMEOUT", "5"))),
		"DIONYSUS_RESOLV_CONF":       mustSingleLine(jsonString(body, "resolvConf", envString(current, "DIONYSUS_RESOLV_CONF", "/etc/resolv.conf"))),
		"DIONYSUS_WIFI_PSK_HASH":     mustSingleLine(jsonString(wifi, "pskHash", envString(current, "DIONYSUS_WIFI_PSK_HASH", ""))),
	}

	pskAction := jsonString(wifi, "pskAction", "preserve")
	if err := validateChoice(pskAction, []string{"preserve", "set", "clear"}, "Wi-Fi PSK action"); err != nil {
		return nil, err
	}
	switch pskAction {
	case "set":
		next["DIONYSUS_WIFI_PSK"] = mustSingleLine(jsonString(wifi, "psk", ""))
	case "clear":
		next["DIONYSUS_WIFI_PSK"] = ""
	default:
		next["DIONYSUS_WIFI_PSK"] = envString(current, "DIONYSUS_WIFI_PSK", "")
	}

	for key, value := range next {
		cleaned, err := cleanSingleLine(value)
		if err != nil {
			return nil, err
		}
		next[key] = cleaned
	}
	return next, nil
}

func networkInterfaces(cfg Config) []any {
	stats := map[string][2]int64{}
	content, _ := os.ReadFile(filepath.Join(cfg.ProcRoot, "net/dev"))
	lines := strings.Split(string(content), "\n")
	if len(lines) > 2 {
		for _, line := range lines[2:] {
			name, data, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			fields := strings.Fields(data)
			if len(fields) >= 16 {
				rx, _ := strconvParseInt(fields[0])
				tx, _ := strconvParseInt(fields[8])
				stats[strings.TrimSpace(name)] = [2]int64{rx, tx}
			}
		}
	}

	addresses := map[string][]string{}
	if output := commandOutputValue("ip", "-o", "addr", "show"); output != "" {
		for _, line := range strings.Split(output, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 4 && (fields[2] == "inet" || fields[2] == "inet6") {
				iface := strings.TrimSuffix(strings.Split(fields[1], "@")[0], ":")
				addresses[iface] = append(addresses[iface], fields[2]+" "+fields[3])
			}
		}
	}

	seen := map[string]bool{}
	var interfaces []any
	entries, err := os.ReadDir(filepath.Join(cfg.SysRoot, "class/net"))
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			seen[name] = true
			path := filepath.Join(cfg.SysRoot, "class/net", name)
			stat := stats[name]
			interfaces = append(interfaces, map[string]any{
				"name":      name,
				"address":   readTrim(filepath.Join(path, "address")),
				"addresses": addresses[name],
				"operstate": readTrim(filepath.Join(path, "operstate")),
				"mtu":       readTrim(filepath.Join(path, "mtu")),
				"rxBytes":   stat[0],
				"txBytes":   stat[1],
			})
		}
	}
	for name, stat := range stats {
		if seen[name] {
			continue
		}
		interfaces = append(interfaces, map[string]any{
			"name":      name,
			"address":   "",
			"addresses": addresses[name],
			"operstate": "",
			"mtu":       "",
			"rxBytes":   stat[0],
			"txBytes":   stat[1],
		})
	}
	sort.Slice(interfaces, func(left, right int) bool {
		return asString(asMap(interfaces[left])["name"]) < asString(asMap(interfaces[right])["name"])
	})
	return interfaces
}

func renderNetworkEnv(values map[string]string) string {
	keys := []string{
		"DIONYSUS_NETWORK_ENABLED",
		"DIONYSUS_NETWORK_MODE",
		"DIONYSUS_LAN_ENABLED",
		"DIONYSUS_LAN_IFACE",
		"DIONYSUS_LAN_IPV4_METHOD",
		"DIONYSUS_LAN_IPV4_ADDRESS",
		"DIONYSUS_LAN_IPV4_GATEWAY",
		"DIONYSUS_LAN_DNS",
		"DIONYSUS_WIFI_ENABLED",
		"DIONYSUS_WIFI_IFACE",
		"DIONYSUS_WIFI_COUNTRY",
		"DIONYSUS_WIFI_SSID",
		"DIONYSUS_WIFI_PSK",
		"DIONYSUS_WIFI_PSK_HASH",
		"DIONYSUS_WIFI_SCAN_SSID",
		"DIONYSUS_WIFI_IPV4_METHOD",
		"DIONYSUS_WIFI_IPV4_ADDRESS",
		"DIONYSUS_WIFI_IPV4_GATEWAY",
		"DIONYSUS_WIFI_DNS",
		"DIONYSUS_DHCP_CLIENT",
		"DIONYSUS_DHCP_TRIES",
		"DIONYSUS_DHCP_TIMEOUT",
		"DIONYSUS_RESOLV_CONF",
	}
	var builder strings.Builder
	builder.WriteString("# Persistent Dionysus network settings.\n")
	builder.WriteString("# Generated by dionysusd; keep mode 0600 when a Wi-Fi PSK is stored here.\n\n")
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(shellQuote(values[key]))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func isASCIIAlpha(value string) bool {
	for _, ch := range value {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z') {
			return false
		}
	}
	return true
}

func strconvParseInt(value string) (int64, error) {
	var parsed int64
	_, err := fmt.Sscan(value, &parsed)
	return parsed, err
}
