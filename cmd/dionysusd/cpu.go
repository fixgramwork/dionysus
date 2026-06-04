package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const cpuSampleInterval = 120 * time.Millisecond

type cpuTimes struct {
	user    int64
	nice    int64
	system  int64
	idle    int64
	iowait  int64
	irq     int64
	softirq int64
	steal   int64
	cores   int
}

func cpuStatus(cfg Config) map[string]any {
	first, ok := readCPUTimes(cfg.ProcRoot)
	if !ok {
		return map[string]any{
			"state": "unavailable",
			"cores": readCPUCoreCount(cfg.ProcRoot),
			"model": readCPUModel(cfg.ProcRoot),
		}
	}

	time.Sleep(cpuSampleInterval)
	second, ok := readCPUTimes(cfg.ProcRoot)
	if !ok {
		return map[string]any{
			"state": "unavailable",
			"cores": readCPUCoreCount(cfg.ProcRoot),
			"model": readCPUModel(cfg.ProcRoot),
		}
	}

	status := cpuUsage(first, second)
	cores := second.cores
	if cores == 0 {
		cores = readCPUCoreCount(cfg.ProcRoot)
	}
	status["state"] = "ready"
	status["cores"] = cores
	status["model"] = readCPUModel(cfg.ProcRoot)
	status["sampleMillis"] = int64(cpuSampleInterval / time.Millisecond)
	return status
}

func readCPUTimes(procRoot string) (cpuTimes, bool) {
	content, err := os.ReadFile(filepath.Join(procRoot, "stat"))
	if err != nil {
		return cpuTimes{}, false
	}
	return parseCPUTimes(string(content))
}

func parseCPUTimes(content string) (cpuTimes, bool) {
	var aggregate cpuTimes
	foundAggregate := false

	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[0] == "cpu" {
			aggregate = parseCPUFields(fields)
			foundAggregate = true
			continue
		}
		if isCPUCoreName(fields[0]) {
			aggregate.cores++
		}
	}

	return aggregate, foundAggregate
}

func parseCPUFields(fields []string) cpuTimes {
	values := make([]int64, 8)
	for index := 1; index < len(fields) && index <= len(values); index++ {
		values[index-1], _ = strconv.ParseInt(fields[index], 10, 64)
	}
	return cpuTimes{
		user:    values[0],
		nice:    values[1],
		system:  values[2],
		idle:    values[3],
		iowait:  values[4],
		irq:     values[5],
		softirq: values[6],
		steal:   values[7],
	}
}

func isCPUCoreName(value string) bool {
	if !strings.HasPrefix(value, "cpu") || len(value) == len("cpu") {
		return false
	}
	for _, ch := range strings.TrimPrefix(value, "cpu") {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func cpuUsage(first cpuTimes, second cpuTimes) map[string]any {
	delta := cpuTimes{
		user:    positive(second.user - first.user),
		nice:    positive(second.nice - first.nice),
		system:  positive(second.system - first.system),
		idle:    positive(second.idle - first.idle),
		iowait:  positive(second.iowait - first.iowait),
		irq:     positive(second.irq - first.irq),
		softirq: positive(second.softirq - first.softirq),
		steal:   positive(second.steal - first.steal),
	}
	totalDelta := delta.total()
	idleDelta := delta.idle + delta.iowait
	usedDelta := positive(totalDelta - idleDelta)

	return map[string]any{
		"usedPercent":   percentOf(usedDelta, totalDelta),
		"userPercent":   percentOf(delta.user+delta.nice, totalDelta),
		"systemPercent": percentOf(delta.system+delta.irq+delta.softirq, totalDelta),
		"idlePercent":   percentOf(delta.idle, totalDelta),
		"iowaitPercent": percentOf(delta.iowait, totalDelta),
		"stealPercent":  percentOf(delta.steal, totalDelta),
		"ticks": map[string]any{
			"user":    second.user,
			"nice":    second.nice,
			"system":  second.system,
			"idle":    second.idle,
			"iowait":  second.iowait,
			"irq":     second.irq,
			"softirq": second.softirq,
			"steal":   second.steal,
			"total":   second.total(),
		},
	}
}

func (times cpuTimes) total() int64 {
	return times.user + times.nice + times.system + times.idle + times.iowait + times.irq + times.softirq + times.steal
}

func percentOf(value int64, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(value) / float64(total) * 100
}

func readCPUModel(procRoot string) string {
	content, err := os.ReadFile(filepath.Join(procRoot, "cpuinfo"))
	if err != nil {
		return ""
	}

	values := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value != "" && values[key] == "" {
			values[key] = value
		}
	}
	for _, key := range []string{"model name", "hardware", "model"} {
		if values[key] != "" {
			return values[key]
		}
	}
	if _, err := strconv.Atoi(values["processor"]); err != nil {
		return values["processor"]
	}
	return ""
}

func readCPUCoreCount(procRoot string) int {
	times, ok := readCPUTimes(procRoot)
	if ok && times.cores > 0 {
		return times.cores
	}

	content, err := os.ReadFile(filepath.Join(procRoot, "cpuinfo"))
	if err != nil {
		return runtime.NumCPU()
	}
	count := 0
	for _, line := range strings.Split(string(content), "\n") {
		key, _, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), "processor") {
			count++
		}
	}
	if count > 0 {
		return count
	}
	return runtime.NumCPU()
}
