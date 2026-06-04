package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func readMeminfo(procRoot string) map[string]int64 {
	values := map[string]int64{}
	content, _ := os.ReadFile(filepath.Join(procRoot, "meminfo"))
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, _ := strconv.ParseInt(fields[1], 10, 64)
		values[strings.TrimSuffix(fields[0], ":")] = value * 1024
	}
	return values
}

func readUptime(procRoot string) float64 {
	content, _ := os.ReadFile(filepath.Join(procRoot, "uptime"))
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0
	}
	value, _ := strconv.ParseFloat(fields[0], 64)
	return value
}

func readLoadavg(procRoot string) []string {
	content, _ := os.ReadFile(filepath.Join(procRoot, "loadavg"))
	fields := strings.Fields(string(content))
	if len(fields) > 3 {
		fields = fields[:3]
	}
	return fields
}

func readTrim(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

func readEnvFile(path string) map[string]string {
	values := map[string]string{}
	content, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if !isEnvKey(key) {
			continue
		}
		values[key] = unquoteEnvValue(strings.TrimSpace(value))
	}
	return values
}

func isEnvKey(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if !(ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_') {
			return false
		}
	}
	return true
}

func unquoteEnvValue(value string) string {
	if len(value) >= 2 && strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.ReplaceAll(value[1:len(value)-1], "'\\''", "'")
	}
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			return unquoted
		}
	}
	return value
}

func envString(values map[string]string, key string, fallback string) string {
	value := values[key]
	if value == "" {
		return fallback
	}
	return value
}

func envBoolValue(values map[string]string, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok || value == "" {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func commandOutput(command string, args ...string) (string, error) {
	output, err := exec.Command(command, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func commandOutputValue(command string, args ...string) string {
	value, err := commandOutput(command, args...)
	if err != nil {
		return ""
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func getNum(values map[string]int64, key string) int64 {
	return values[key]
}

func positive(value int64) int64 {
	if value > 0 {
		return value
	}
	return 0
}

func now() int64 {
	return time.Now().Unix()
}

func sqlQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func object(value map[string]any, key string) map[string]any {
	child, ok := value[key].(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return child
}

func asMap(value any) map[string]any {
	child, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return child
}

func asSlice(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return []any{}
	}
	return items
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func asInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		next, _ := typed.Int64()
		return next
	default:
		return 0
	}
}

func asBool(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func jsonString(value map[string]any, key string, fallback string) string {
	next := asString(value[key])
	if next == "" {
		return fallback
	}
	return next
}

func jsonBool(value map[string]any, key string, fallback bool) bool {
	next, ok := value[key].(bool)
	if !ok {
		return fallback
	}
	return next
}

func boolEnv(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func validateChoice(value string, allowed []string, label string) error {
	for _, item := range allowed {
		if value == item {
			return nil
		}
	}
	return fmt.Errorf("unsupported %s: %s; allowed values: %s", label, value, strings.Join(allowed, ", "))
}

func validateIface(value string) error {
	if value == "" || len(value) > 32 {
		return fmt.Errorf("invalid network interface name: %s", value)
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_' || ch == '-' || ch == '.' || ch == ':') {
			return fmt.Errorf("invalid network interface name: %s", value)
		}
	}
	return nil
}

func cleanSingleLine(value string) (string, error) {
	if strings.ContainsAny(value, "\n\r\x00") {
		return "", fmt.Errorf("network setting values must be single-line strings")
	}
	return strings.TrimSpace(value), nil
}

func mustSingleLine(value string) string {
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func encodeJSON(value any) string {
	var buffer bytes.Buffer
	_ = json.NewEncoder(&buffer).Encode(value)
	return strings.TrimSpace(buffer.String())
}
