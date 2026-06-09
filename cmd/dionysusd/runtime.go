package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func ensureTargetOSRuntime(cfg Config) error {
	if cfg.DevAllowHost {
		fmt.Fprintln(os.Stderr, "[dionysusd] development host override enabled; this mode is not a production OS runtime")
		return nil
	}

	var missing []string
	if runtime.GOOS != "linux" {
		missing = append(missing, "host OS is not Linux")
	}
	if !fileExists(filepath.Join(cfg.ProcRoot, "meminfo")) {
		missing = append(missing, fmt.Sprintf("%s is missing", filepath.Join(cfg.ProcRoot, "meminfo")))
	}
	if !dirExists(filepath.Join(cfg.SysRoot, "kernel")) {
		missing = append(missing, fmt.Sprintf("%s is missing", filepath.Join(cfg.SysRoot, "kernel")))
	}
	if !dirExists("/run/systemd/system") {
		missing = append(missing, "/run/systemd/system is missing")
	}
	if !commandAvailable("systemctl") {
		missing = append(missing, "systemctl is unavailable")
	}

	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("Dionysus manager must run inside the target Linux/systemd OS; %s. For explicit local UI-only development, pass --dev-allow-host or set %s=1", strings.Join(missing, ", "), devAllowHostEnv)
}
