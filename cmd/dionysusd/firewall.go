package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	firewallRulesLimit       = 64
	firewallRulesetTextLimit = 32 * 1024
)

type firewallConfig struct {
	Path             string
	Exists           bool
	Enabled          bool
	DefaultIncoming  string
	DefaultOutgoing  string
	AllowEstablished bool
	AllowLoopback    bool
	AllowPing        bool
	Rules            []firewallRule
	Error            string
}

type firewallRule struct {
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Direction   string `json:"direction"`
	Action      string `json:"action"`
	Protocol    string `json:"protocol"`
	Port        string `json:"port,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

func firewallStatus(cfg Config) map[string]any {
	config := readFirewallConfig(cfg.FirewallConfig)
	return map[string]any{
		"config":        firewallConfigView(config),
		"service":       service("dionysus-firewall"),
		"tools":         firewallTools(),
		"activeRuleset": firewallActiveRuleset(),
		"rendered":      renderNftablesConfig(config),
		"ruleCount":     len(config.Rules),
	}
}

func firewallConfigPayload(path string) map[string]any {
	return firewallConfigView(readFirewallConfig(path))
}

func readFirewallConfig(path string) firewallConfig {
	return firewallConfigFromEnv(readEnvFile(path), path, fileExists(path))
}

func firewallConfigFromEnv(env map[string]string, path string, exists bool) firewallConfig {
	config := firewallConfig{
		Path:             path,
		Exists:           exists,
		Enabled:          envBoolValue(env, "DIONYSUS_FIREWALL_ENABLED", false),
		DefaultIncoming:  envChoice(env, "DIONYSUS_FIREWALL_DEFAULT_INCOMING", "drop"),
		DefaultOutgoing:  envChoice(env, "DIONYSUS_FIREWALL_DEFAULT_OUTGOING", "accept"),
		AllowEstablished: envBoolValue(env, "DIONYSUS_FIREWALL_ALLOW_ESTABLISHED", true),
		AllowLoopback:    envBoolValue(env, "DIONYSUS_FIREWALL_ALLOW_LOOPBACK", true),
		AllowPing:        envBoolValue(env, "DIONYSUS_FIREWALL_ALLOW_PING", true),
		Rules:            defaultFirewallRules(),
	}

	if err := validateChoice(config.DefaultIncoming, []string{"accept", "drop", "reject"}, "default incoming firewall policy"); err != nil {
		config.DefaultIncoming = "drop"
		config.Error = err.Error()
	}
	if err := validateChoice(config.DefaultOutgoing, []string{"accept", "drop", "reject"}, "default outgoing firewall policy"); err != nil {
		config.DefaultOutgoing = "accept"
		config.Error = firstNonEmpty(config.Error, err.Error())
	}

	rawRules, ok := env["DIONYSUS_FIREWALL_RULES"]
	if ok {
		rules, err := parseStoredFirewallRules(rawRules)
		if err != nil {
			config.Error = firstNonEmpty(config.Error, err.Error())
			config.Rules = []firewallRule{}
		} else {
			config.Rules = rules
		}
	}

	return config
}

func defaultFirewallRules() []firewallRule {
	return []firewallRule{
		{
			Name:      "ssh",
			Enabled:   true,
			Direction: "in",
			Action:    "accept",
			Protocol:  "tcp",
			Port:      "22",
			Comment:   "SSH access",
		},
		{
			Name:      "pve-web",
			Enabled:   true,
			Direction: "in",
			Action:    "accept",
			Protocol:  "tcp",
			Port:      "8006",
			Comment:   "Dionysus web console",
		},
	}
}

func envChoice(env map[string]string, key string, fallback string) string {
	value := strings.TrimSpace(env[key])
	if value == "" {
		return fallback
	}
	return value
}

func firewallConfigView(config firewallConfig) map[string]any {
	return map[string]any{
		"path":             config.Path,
		"exists":           config.Exists,
		"enabled":          config.Enabled,
		"defaultIncoming":  config.DefaultIncoming,
		"defaultOutgoing":  config.DefaultOutgoing,
		"allowEstablished": config.AllowEstablished,
		"allowLoopback":    config.AllowLoopback,
		"allowPing":        config.AllowPing,
		"rules":            firewallRuleViews(config.Rules),
		"error":            config.Error,
	}
}

func firewallRuleViews(rules []firewallRule) []any {
	views := make([]any, 0, len(rules))
	for _, rule := range rules {
		views = append(views, map[string]any{
			"name":        rule.Name,
			"enabled":     rule.Enabled,
			"direction":   rule.Direction,
			"action":      rule.Action,
			"protocol":    rule.Protocol,
			"port":        rule.Port,
			"source":      rule.Source,
			"destination": rule.Destination,
			"comment":     rule.Comment,
		})
	}
	return views
}

func parseStoredFirewallRules(raw string) ([]firewallRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []firewallRule{}, nil
	}

	var rules []firewallRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return nil, fmt.Errorf("firewall rules are invalid JSON")
	}
	if len(rules) > firewallRulesLimit {
		return nil, fmt.Errorf("firewall rules exceed limit %d", firewallRulesLimit)
	}

	for index := range rules {
		cleaned, err := normalizeFirewallRule(rules[index], index)
		if err != nil {
			return nil, err
		}
		rules[index] = cleaned
	}
	return rules, nil
}

func updateFirewallConfig(cfg Config, body map[string]any) (map[string]any, error) {
	dryRun := jsonBool(body, "dryRun", false)
	apply := jsonBool(body, "apply", false)
	current := readFirewallConfig(cfg.FirewallConfig)
	next, err := firewallConfigFromPayload(cfg.FirewallConfig, body, current)
	if err != nil {
		return nil, err
	}

	if !dryRun {
		if err := os.MkdirAll(filepath.Dir(cfg.FirewallConfig), 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s: %w", filepath.Dir(cfg.FirewallConfig), err)
		}
		if err := os.WriteFile(cfg.FirewallConfig, []byte(renderFirewallEnv(next)), 0600); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", cfg.FirewallConfig, err)
		}
		_ = os.Chmod(cfg.FirewallConfig, 0600)
	}

	var applyResult any
	if apply {
		applyResult = applyFirewallConfig(next, cfg.DevAllowHost, dryRun)
	}

	return map[string]any{
		"dryRun":      dryRun,
		"written":     !dryRun,
		"apply":       apply,
		"config":      firewallConfigView(next),
		"rendered":    renderNftablesConfig(next),
		"applyResult": applyResult,
	}, nil
}

func firewallConfigFromPayload(path string, body map[string]any, current firewallConfig) (firewallConfig, error) {
	config := firewallConfig{
		Path:             path,
		Exists:           current.Exists,
		Enabled:          jsonBool(body, "enabled", current.Enabled),
		DefaultIncoming:  jsonString(body, "defaultIncoming", current.DefaultIncoming),
		DefaultOutgoing:  jsonString(body, "defaultOutgoing", current.DefaultOutgoing),
		AllowEstablished: jsonBool(body, "allowEstablished", current.AllowEstablished),
		AllowLoopback:    jsonBool(body, "allowLoopback", current.AllowLoopback),
		AllowPing:        jsonBool(body, "allowPing", current.AllowPing),
	}

	if err := validateChoice(config.DefaultIncoming, []string{"accept", "drop", "reject"}, "default incoming firewall policy"); err != nil {
		return firewallConfig{}, err
	}
	if err := validateChoice(config.DefaultOutgoing, []string{"accept", "drop", "reject"}, "default outgoing firewall policy"); err != nil {
		return firewallConfig{}, err
	}

	rules, err := firewallRulesFromPayload(body["rules"], current.Rules)
	if err != nil {
		return firewallConfig{}, err
	}
	config.Rules = rules
	return config, nil
}

func firewallRulesFromPayload(value any, fallback []firewallRule) ([]firewallRule, error) {
	if value == nil {
		return fallback, nil
	}

	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("firewall rules must be an array")
	}
	if len(items) > firewallRulesLimit {
		return nil, fmt.Errorf("firewall rules exceed limit %d", firewallRulesLimit)
	}

	rules := make([]firewallRule, 0, len(items))
	for index, item := range items {
		ruleMap := asMap(item)
		rule := firewallRule{
			Name:        jsonString(ruleMap, "name", ""),
			Enabled:     jsonBool(ruleMap, "enabled", true),
			Direction:   jsonString(ruleMap, "direction", "in"),
			Action:      jsonString(ruleMap, "action", "accept"),
			Protocol:    jsonString(ruleMap, "protocol", "tcp"),
			Port:        jsonString(ruleMap, "port", ""),
			Source:      jsonString(ruleMap, "source", ""),
			Destination: jsonString(ruleMap, "destination", ""),
			Comment:     jsonString(ruleMap, "comment", ""),
		}
		cleaned, err := normalizeFirewallRule(rule, index)
		if err != nil {
			return nil, err
		}
		rules = append(rules, cleaned)
	}
	return rules, nil
}

func normalizeFirewallRule(rule firewallRule, index int) (firewallRule, error) {
	var err error
	rule.Name, err = cleanFirewallLabel(rule.Name, fmt.Sprintf("rule-%02d", index+1), "firewall rule name")
	if err != nil {
		return firewallRule{}, err
	}
	rule.Direction = strings.ToLower(strings.TrimSpace(rule.Direction))
	if err := validateChoice(rule.Direction, []string{"in", "out"}, "firewall rule direction"); err != nil {
		return firewallRule{}, err
	}
	rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
	if err := validateChoice(rule.Action, []string{"accept", "drop", "reject"}, "firewall rule action"); err != nil {
		return firewallRule{}, err
	}
	rule.Protocol = strings.ToLower(strings.TrimSpace(rule.Protocol))
	if err := validateChoice(rule.Protocol, []string{"tcp", "udp", "icmp", "any"}, "firewall rule protocol"); err != nil {
		return firewallRule{}, err
	}
	rule.Port, err = cleanFirewallPortSpec(rule.Port)
	if err != nil {
		return firewallRule{}, err
	}
	if rule.Port != "" && (rule.Protocol == "icmp" || rule.Protocol == "any") {
		return firewallRule{}, fmt.Errorf("firewall rule %s cannot use a port with protocol %s", rule.Name, rule.Protocol)
	}
	rule.Source, err = cleanFirewallAddress(rule.Source, "source")
	if err != nil {
		return firewallRule{}, err
	}
	rule.Destination, err = cleanFirewallAddress(rule.Destination, "destination")
	if err != nil {
		return firewallRule{}, err
	}
	if rule.Source != "" && rule.Destination != "" && firewallAddressFamily(rule.Source) != firewallAddressFamily(rule.Destination) {
		return firewallRule{}, fmt.Errorf("firewall rule %s cannot mix IPv4 and IPv6 addresses", rule.Name)
	}
	rule.Comment, err = cleanFirewallLabel(rule.Comment, "", "firewall rule comment")
	if err != nil {
		return firewallRule{}, err
	}
	return rule, nil
}

func cleanFirewallLabel(value string, fallback string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	if len(value) > 80 {
		return "", fmt.Errorf("%s is too long", label)
	}
	if strings.ContainsAny(value, "\n\r\x00") {
		return "", fmt.Errorf("%s must be a single-line string", label)
	}
	return value, nil
}

func cleanFirewallAddress(value string, label string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if _, err := netip.ParsePrefix(value); err == nil {
		return value, nil
	}
	if address, err := netip.ParseAddr(value); err == nil {
		return address.String(), nil
	}
	return "", fmt.Errorf("invalid firewall %s address: %s", label, value)
}

func cleanFirewallPortSpec(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parts := strings.Split(value, ",")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return "", fmt.Errorf("firewall port list contains an empty value")
		}
		if strings.Contains(part, "-") {
			left, right, ok := strings.Cut(part, "-")
			if !ok {
				return "", fmt.Errorf("invalid firewall port range: %s", part)
			}
			start, err := parseFirewallPort(left)
			if err != nil {
				return "", err
			}
			end, err := parseFirewallPort(right)
			if err != nil {
				return "", err
			}
			if start > end {
				return "", fmt.Errorf("invalid firewall port range: %s", part)
			}
			cleaned = append(cleaned, fmt.Sprintf("%d-%d", start, end))
			continue
		}
		port, err := parseFirewallPort(part)
		if err != nil {
			return "", err
		}
		cleaned = append(cleaned, strconv.Itoa(port))
	}
	return strings.Join(cleaned, ","), nil
}

func parseFirewallPort(value string) (int, error) {
	value = strings.TrimSpace(value)
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid firewall port: %s", value)
	}
	return port, nil
}

func renderFirewallEnv(config firewallConfig) string {
	rulesJSON, _ := json.Marshal(config.Rules)
	var builder strings.Builder
	builder.WriteString("# Persistent Dionysus firewall settings.\n")
	builder.WriteString("# Generated by dionysusd; rules are rendered to nftables at apply time.\n\n")
	keys := []struct {
		name  string
		value string
	}{
		{"DIONYSUS_FIREWALL_ENABLED", boolEnv(config.Enabled)},
		{"DIONYSUS_FIREWALL_DEFAULT_INCOMING", config.DefaultIncoming},
		{"DIONYSUS_FIREWALL_DEFAULT_OUTGOING", config.DefaultOutgoing},
		{"DIONYSUS_FIREWALL_ALLOW_ESTABLISHED", boolEnv(config.AllowEstablished)},
		{"DIONYSUS_FIREWALL_ALLOW_LOOPBACK", boolEnv(config.AllowLoopback)},
		{"DIONYSUS_FIREWALL_ALLOW_PING", boolEnv(config.AllowPing)},
		{"DIONYSUS_FIREWALL_RULES", string(rulesJSON)},
	}
	for _, item := range keys {
		builder.WriteString(item.name)
		builder.WriteByte('=')
		builder.WriteString(shellQuote(item.value))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func renderNftablesConfig(config firewallConfig) string {
	if !config.Enabled {
		return "# Dionysus firewall is disabled; applying this config clears table inet dionysus_filter.\n"
	}

	var builder strings.Builder
	builder.WriteString("table inet dionysus_filter {\n")
	writeFirewallChain(&builder, "input", "input", config.DefaultIncoming, config)
	builder.WriteString("\n")
	builder.WriteString("    chain forward {\n")
	builder.WriteString("        type filter hook forward priority 0; policy drop;\n")
	builder.WriteString("    }\n\n")
	writeFirewallChain(&builder, "output", "output", config.DefaultOutgoing, config)
	builder.WriteString("}\n")
	return builder.String()
}

func writeFirewallChain(builder *strings.Builder, name string, hook string, policy string, config firewallConfig) {
	builder.WriteString("    chain ")
	builder.WriteString(name)
	builder.WriteString(" {\n")
	builder.WriteString("        type filter hook ")
	builder.WriteString(hook)
	builder.WriteString(" priority 0; policy ")
	builder.WriteString(nftPolicy(policy))
	builder.WriteString(";\n")

	if config.AllowLoopback {
		if name == "input" {
			builder.WriteString("        iifname \"lo\" accept comment \"allow loopback\"\n")
		} else {
			builder.WriteString("        oifname \"lo\" accept comment \"allow loopback\"\n")
		}
	}
	if name == "input" && config.AllowEstablished {
		builder.WriteString("        ct state established,related accept comment \"allow established traffic\"\n")
	}
	if name == "input" && config.AllowPing {
		builder.WriteString("        ip protocol icmp accept comment \"allow IPv4 ping\"\n")
		builder.WriteString("        ip6 nexthdr ipv6-icmp accept comment \"allow IPv6 ICMP\"\n")
	}

	direction := "in"
	if name == "output" {
		direction = "out"
	}
	for _, rule := range config.Rules {
		if !rule.Enabled || rule.Direction != direction {
			continue
		}
		for _, line := range renderFirewallRuleLines(rule) {
			builder.WriteString("        ")
			builder.WriteString(line)
			builder.WriteByte('\n')
		}
	}

	if policy == "reject" {
		builder.WriteString("        reject comment \"default reject\"\n")
	}
	builder.WriteString("    }\n")
}

func nftPolicy(policy string) string {
	if policy == "accept" {
		return "accept"
	}
	return "drop"
}

func renderFirewallRuleLines(rule firewallRule) []string {
	base := firewallAddressExpressions(rule)
	comment := nftComment(firstNonEmpty(rule.Comment, rule.Name))
	action := rule.Action

	switch rule.Protocol {
	case "icmp":
		family := firewallRuleFamily(rule)
		if family == "ip6" {
			return []string{strings.Join(append(append([]string{}, base...), "ip6 nexthdr ipv6-icmp", action, comment), " ")}
		}
		if family == "ip" {
			return []string{strings.Join(append(append([]string{}, base...), "ip protocol icmp", action, comment), " ")}
		}
		return []string{
			strings.Join(append(append([]string{}, base...), "ip protocol icmp", action, comment), " "),
			strings.Join(append(append([]string{}, base...), "ip6 nexthdr ipv6-icmp", action, comment), " "),
		}
	case "tcp", "udp":
		parts := append([]string{}, base...)
		if rule.Port == "" {
			parts = append(parts, "meta l4proto", rule.Protocol)
		} else {
			parts = append(parts, rule.Protocol, "dport", renderNftPortSpec(rule.Port))
		}
		parts = append(parts, action, comment)
		return []string{strings.Join(parts, " ")}
	default:
		parts := append(append([]string{}, base...), action, comment)
		return []string{strings.Join(parts, " ")}
	}
}

func firewallAddressExpressions(rule firewallRule) []string {
	var parts []string
	if rule.Source != "" {
		parts = append(parts, firewallAddressFamily(rule.Source), "saddr", rule.Source)
	}
	if rule.Destination != "" {
		parts = append(parts, firewallAddressFamily(rule.Destination), "daddr", rule.Destination)
	}
	return parts
}

func firewallRuleFamily(rule firewallRule) string {
	if rule.Source != "" {
		return firewallAddressFamily(rule.Source)
	}
	if rule.Destination != "" {
		return firewallAddressFamily(rule.Destination)
	}
	return ""
}

func firewallAddressFamily(value string) string {
	if strings.Contains(value, ":") {
		return "ip6"
	}
	return "ip"
}

func renderNftPortSpec(value string) string {
	if !strings.Contains(value, ",") {
		return value
	}
	return "{ " + strings.Join(strings.Split(value, ","), ", ") + " }"
}

func nftComment(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return "comment \"" + value + "\""
}

func applyFirewallRuntime(cfg Config, dryRun bool) map[string]any {
	config := readFirewallConfig(cfg.FirewallConfig)
	return applyFirewallConfig(config, cfg.DevAllowHost, dryRun)
}

func applyFirewallConfig(config firewallConfig, devSkip bool, dryRun bool) map[string]any {
	rendered := renderNftablesConfig(config)
	if dryRun {
		return map[string]any{
			"dryRun":   true,
			"status":   "dry-run",
			"command":  firewallApplyCommandLabel(config.Enabled),
			"rendered": rendered,
		}
	}
	if devSkip {
		return map[string]any{
			"dryRun":   false,
			"status":   "dev-skip",
			"command":  firewallApplyCommandLabel(config.Enabled),
			"rendered": rendered,
			"reason":   "development host override is enabled; nftables was not changed",
		}
	}
	if !commandAvailable("nft") {
		return map[string]any{
			"dryRun":   false,
			"status":   "missing",
			"command":  "nft",
			"error":    "nft command is not available",
			"rendered": rendered,
		}
	}
	if err := clearFirewallTable(); err != nil {
		return map[string]any{
			"dryRun":   false,
			"status":   "failed",
			"command":  "nft delete table inet dionysus_filter",
			"error":    err.Error(),
			"rendered": rendered,
		}
	}
	if !config.Enabled {
		return map[string]any{
			"dryRun":   false,
			"status":   "cleared",
			"command":  "nft delete table inet dionysus_filter",
			"rendered": rendered,
		}
	}

	command := newCommand("nft", "-f", "-")
	command.Stdin = strings.NewReader(rendered)
	output, err := command.CombinedOutput()
	if err != nil {
		return map[string]any{
			"dryRun":   false,
			"status":   "failed",
			"command":  "nft -f -",
			"error":    strings.TrimSpace(string(output)),
			"rendered": rendered,
		}
	}
	return map[string]any{
		"dryRun":   false,
		"status":   "applied",
		"command":  "nft -f -",
		"stdout":   strings.TrimSpace(string(output)),
		"rendered": rendered,
	}
}

func firewallApplyCommandLabel(enabled bool) string {
	if enabled {
		return "nft delete table inet dionysus_filter; nft -f -"
	}
	return "nft delete table inet dionysus_filter"
}

func runFirewallApplyCommand(cfg Config) error {
	result := applyFirewallRuntime(cfg, false)
	status := asString(result["status"])
	if status != "applied" && status != "cleared" {
		return fmt.Errorf("firewall apply %s: %s", status, firstNonEmpty(asString(result["error"]), asString(result["reason"])))
	}
	return nil
}

func runFirewallClearCommand() error {
	return clearFirewallTable()
}

func clearFirewallTable() error {
	output, err := newCommand("nft", "delete", "table", "inet", "dionysus_filter").CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if strings.Contains(text, "No such file") || strings.Contains(text, "does not exist") {
			return nil
		}
		return fmt.Errorf("%s", firstNonEmpty(text, err.Error()))
	}
	return nil
}

func firewallTools() map[string]any {
	tools := map[string]any{}
	for _, name := range []string{"nft", "iptables", "ip6tables", "ufw"} {
		tools[name] = commandAvailable(name)
	}
	return tools
}

func firewallActiveRuleset() map[string]any {
	if !commandAvailable("nft") {
		return map[string]any{
			"available": false,
			"status":    "missing",
			"command":   "nft list ruleset",
			"stdout":    "",
			"truncated": false,
		}
	}
	output, err := commandOutput("nft", "list", "ruleset")
	limited, truncated := limitFirewallText(output, firewallRulesetTextLimit)
	status := "ok"
	errorText := ""
	if err != nil {
		status = "failed"
		errorText = err.Error()
	}
	return map[string]any{
		"available": true,
		"status":    status,
		"command":   "nft list ruleset",
		"stdout":    limited,
		"error":     errorText,
		"truncated": truncated,
	}
}

func limitFirewallText(value string, limit int) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}
