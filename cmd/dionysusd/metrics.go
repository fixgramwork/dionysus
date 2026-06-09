package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runMetricsd(cfg Config, once bool) error {
	fmt.Printf("[dionysusd-metricsd] writing Ollama RAM samples to %s\n", cfg.MetricsDB)
	for {
		summary, err := recordSample(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[dionysusd-metricsd] sample failed: %s\n", err)
		} else {
			fmt.Printf(
				"[dionysusd-metricsd] collected_at=%d rss=%d swap=%d models=%d\n",
				asInt64(summary["collectedAt"]),
				asInt64(summary["processRss"]),
				asInt64(summary["processSwap"]),
				asInt64(summary["loadedModelCount"]),
			)
		}
		if once {
			return nil
		}
		time.Sleep(time.Duration(cfg.IntervalSeconds) * time.Second)
	}
}

func recordSample(cfg Config) (map[string]any, error) {
	sample := collectSample(cfg)
	if err := initMetricsSchema(cfg.MetricsDB); err != nil {
		return nil, err
	}
	summary := sampleSummary(sample)
	retentionSeconds := int64(cfg.RetentionDays) * 24 * 60 * 60
	cutoff := asInt64(summary["collectedAt"]) - retentionSeconds
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
	if err := runSQLite(cfg.MetricsDB, sql); err != nil {
		return nil, err
	}
	return summary, nil
}

func collectSample(cfg Config) map[string]any {
	return map[string]any{
		"collectedAt": now(),
		"node":        nodeStatus(cfg),
		"ollama":      ollamaStatus(cfg),
	}
}

func sampleSummary(sample map[string]any) map[string]any {
	node := asMap(sample["node"])
	memory := asMap(node["memory"])
	swap := asMap(node["swap"])
	ollama := asMap(sample["ollama"])
	runtime := asMap(ollama["runtime"])
	loaded := asMap(ollama["loadedModelMemory"])
	reachable := int64(0)
	if asBool(ollama["apiReachable"]) {
		reachable = 1
	}
	return map[string]any{
		"collectedAt":      asInt64(sample["collectedAt"]),
		"memoryTotal":      asInt64(memory["total"]),
		"memoryUsed":       asInt64(memory["used"]),
		"memoryAvailable":  asInt64(memory["available"]),
		"swapTotal":        asInt64(swap["total"]),
		"swapUsed":         asInt64(swap["used"]),
		"ollamaReachable":  reachable,
		"processCount":     asInt64(runtime["processCount"]),
		"processRss":       asInt64(runtime["processRss"]),
		"processSwap":      asInt64(runtime["processSwap"]),
		"loadedModelCount": asInt64(loaded["count"]),
		"loadedModelSize":  asInt64(loaded["size"]),
		"loadedModelVram":  asInt64(loaded["sizeVram"]),
	}
}

func metricsHistory(dbPath string, limit int64) []any {
	if err := initMetricsSchema(dbPath); err != nil {
		return []any{}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}
	sql := fmt.Sprintf("SELECT id, collected_at AS collectedAt, memory_total AS memoryTotal, memory_used AS memoryUsed, memory_available AS memoryAvailable, swap_total AS swapTotal, swap_used AS swapUsed, ollama_reachable AS ollamaReachable, process_count AS processCount, process_rss AS processRss, process_swap AS processSwap, loaded_model_count AS loadedModelCount, loaded_model_size AS loadedModelSize, loaded_model_vram AS loadedModelVram, sample_json AS sampleJson FROM ollama_samples ORDER BY collected_at DESC LIMIT %d;", limit)
	rows, err := querySQLiteJSON(dbPath, sql)
	if err != nil {
		return []any{}
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	history := make([]any, 0, len(rows))
	for _, row := range rows {
		var sample map[string]any
		_ = json.Unmarshal([]byte(asString(row["sampleJson"])), &sample)
		history = append(history, map[string]any{
			"id":          asInt64(row["id"]),
			"collectedAt": asInt64(row["collectedAt"]),
			"memory": map[string]any{
				"total":     asInt64(row["memoryTotal"]),
				"used":      asInt64(row["memoryUsed"]),
				"available": asInt64(row["memoryAvailable"]),
			},
			"swap": map[string]any{
				"total": asInt64(row["swapTotal"]),
				"used":  asInt64(row["swapUsed"]),
			},
			"ollama": map[string]any{
				"reachable":        asInt64(row["ollamaReachable"]) != 0,
				"processCount":     asInt64(row["processCount"]),
				"processRss":       asInt64(row["processRss"]),
				"processSwap":      asInt64(row["processSwap"]),
				"loadedModelCount": asInt64(row["loadedModelCount"]),
				"loadedModelSize":  asInt64(row["loadedModelSize"]),
				"loadedModelVram":  asInt64(row["loadedModelVram"]),
			},
			"sample": sample,
		})
	}
	return history
}

func initMetricsSchema(dbPath string) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(dbPath), err)
	}
	return runSQLite(dbPath, "CREATE TABLE IF NOT EXISTS ollama_samples (id INTEGER PRIMARY KEY AUTOINCREMENT, collected_at INTEGER NOT NULL, memory_total INTEGER NOT NULL DEFAULT 0, memory_used INTEGER NOT NULL DEFAULT 0, memory_available INTEGER NOT NULL DEFAULT 0, swap_total INTEGER NOT NULL DEFAULT 0, swap_used INTEGER NOT NULL DEFAULT 0, ollama_reachable INTEGER NOT NULL DEFAULT 0, process_count INTEGER NOT NULL DEFAULT 0, process_rss INTEGER NOT NULL DEFAULT 0, process_swap INTEGER NOT NULL DEFAULT 0, loaded_model_count INTEGER NOT NULL DEFAULT 0, loaded_model_size INTEGER NOT NULL DEFAULT 0, loaded_model_vram INTEGER NOT NULL DEFAULT 0, sample_json TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_ollama_samples_collected_at ON ollama_samples(collected_at);")
}

func runSQLite(dbPath string, sql string) error {
	output, err := newCommand("sqlite3", dbPath, sql).CombinedOutput()
	if err != nil {
		return fmt.Errorf(strings.TrimSpace(string(output)))
	}
	return nil
}

func querySQLiteJSON(dbPath string, sql string) ([]map[string]any, error) {
	output, err := newCommand("sqlite3", "-json", dbPath, sql).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf(strings.TrimSpace(string(output)))
	}
	var rows []map[string]any
	if err := json.Unmarshal(output, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}
