package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultProxyListen         = "0.0.0.0:8006"
	defaultAPIListen           = "127.0.0.1:8507"
	defaultProcRoot            = "/proc"
	defaultSysRoot             = "/sys"
	defaultEtcRoot             = "/etc"
	defaultOllamaAPI           = "http://127.0.0.1:11434"
	defaultMetricsDB           = "/var/lib/dionysus/metrics/ollama.sqlite3"
	defaultTokenFile           = "/etc/dionysus/pve.token"
	defaultWWWRoot             = "/usr/share/dionysus-pve-manager/www"
	defaultNetworkConfig       = "/etc/dionysus/network.env"
	devAllowHostEnv            = "DIONYSUS_DEV_ALLOW_HOST"
	defaultIntervalSecs        = 30
	defaultRetentionDays       = 7
	maxHistoryLimit      int64 = 20160
)

type Config struct {
	Listen          string
	ProcRoot        string
	SysRoot         string
	EtcRoot         string
	OllamaAPI       string
	MetricsDB       string
	TokenFile       string
	WWWRoot         string
	NetworkConfig   string
	IntervalSeconds int
	RetentionDays   int
	DevAllowHost    bool
}

func configFromArgs(command string, args []string) Config {
	defaultListen := defaultProxyListen
	if command == "api" || command == "pvedaemon" {
		defaultListen = defaultAPIListen
	}

	cfg := Config{
		Listen:          envDefault("DIONYSUS_PVE_PROXY_LISTEN", defaultListen),
		ProcRoot:        envDefault("DIONYSUS_PROC_ROOT", defaultProcRoot),
		SysRoot:         envDefault("DIONYSUS_SYS_ROOT", defaultSysRoot),
		EtcRoot:         envDefault("DIONYSUS_ETC_ROOT", defaultEtcRoot),
		OllamaAPI:       trimSlash(envDefault("DIONYSUS_OLLAMA_API", defaultOllamaAPI)),
		MetricsDB:       envDefault("DIONYSUS_METRICS_DB", defaultMetricsDB),
		TokenFile:       envDefault("DIONYSUS_PVE_TOKEN_FILE", defaultTokenFile),
		WWWRoot:         envDefault("DIONYSUS_PVE_WWW", defaultWWWRoot),
		NetworkConfig:   envDefault("DIONYSUS_NETWORK_CONFIG", defaultNetworkConfig),
		IntervalSeconds: envInt("DIONYSUS_METRICS_INTERVAL_SECONDS", defaultIntervalSecs),
		RetentionDays:   envInt("DIONYSUS_METRICS_RETENTION_DAYS", defaultRetentionDays),
		DevAllowHost:    envBool(devAllowHostEnv, false),
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.Listen, "listen", cfg.Listen, "")
	flags.StringVar(&cfg.ProcRoot, "proc-root", cfg.ProcRoot, "")
	flags.StringVar(&cfg.SysRoot, "sys-root", cfg.SysRoot, "")
	flags.StringVar(&cfg.EtcRoot, "etc-root", cfg.EtcRoot, "")
	flags.StringVar(&cfg.OllamaAPI, "ollama-api", cfg.OllamaAPI, "")
	flags.StringVar(&cfg.MetricsDB, "metrics-db", cfg.MetricsDB, "")
	flags.StringVar(&cfg.TokenFile, "token-file", cfg.TokenFile, "")
	flags.StringVar(&cfg.WWWRoot, "www-root", cfg.WWWRoot, "")
	flags.StringVar(&cfg.NetworkConfig, "network-config", cfg.NetworkConfig, "")
	flags.IntVar(&cfg.IntervalSeconds, "interval", cfg.IntervalSeconds, "")
	flags.IntVar(&cfg.RetentionDays, "retention-days", cfg.RetentionDays, "")
	flags.BoolVar(&cfg.DevAllowHost, "dev-allow-host", cfg.DevAllowHost, "")
	_ = flags.Parse(args)

	cfg.OllamaAPI = trimSlash(cfg.OllamaAPI)
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = defaultIntervalSecs
	}
	if cfg.RetentionDays < 1 {
		cfg.RetentionDays = defaultRetentionDays
	}
	cfg.ProcRoot = filepath.Clean(cfg.ProcRoot)
	cfg.SysRoot = filepath.Clean(cfg.SysRoot)
	cfg.EtcRoot = filepath.Clean(cfg.EtcRoot)

	return cfg
}

func envDefault(name string, fallback string) string {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil {
		return fallback
	}
	return value
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func trimSlash(value string) string {
	return strings.TrimRight(value, "/")
}
