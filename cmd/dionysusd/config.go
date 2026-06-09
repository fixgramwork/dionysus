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
	defaultProxyListen              = "0.0.0.0:8006"
	defaultAPIListen                = "127.0.0.1:8507"
	defaultProcRoot                 = "/proc"
	defaultSysRoot                  = "/sys"
	defaultEtcRoot                  = "/etc"
	defaultOllamaAPI                = "http://127.0.0.1:11434"
	defaultMetricsDB                = "/var/lib/dionysus/metrics/ollama.sqlite3"
	defaultTokenFile                = "/etc/dionysus/pve.token"
	defaultAuthUsersFile            = "/etc/dionysus/pve.users.json"
	defaultAuthUserFile             = "/etc/dionysus/pve.user"
	defaultAuthPasswordFile         = "/etc/dionysus/pve.password"
	defaultAuthUsername             = "root"
	defaultWWWRoot                  = "/usr/share/dionysus-pve-manager/www"
	defaultNetworkConfig            = "/etc/dionysus/network.env"
	defaultFirewallConfig           = "/etc/dionysus/firewall.env"
	devAllowHostEnv                 = "DIONYSUS_DEV_ALLOW_HOST"
	defaultIntervalSecs             = 30
	defaultRetentionDays            = 7
	defaultConsoleTimeoutSecs       = 300
	defaultJWTTTLSeconds            = 12 * 60 * 60
	maxHistoryLimit           int64 = 20160
)

type Config struct {
	Listen                string
	ProcRoot              string
	SysRoot               string
	EtcRoot               string
	OllamaAPI             string
	MetricsDB             string
	TokenFile             string
	AuthUsersFile         string
	AuthUserFile          string
	AuthPasswordFile      string
	AuthUsername          string
	AuthPassword          string
	WWWRoot               string
	NetworkConfig         string
	FirewallConfig        string
	IntervalSeconds       int
	RetentionDays         int
	ConsoleTimeoutSeconds int
	JWTTTLSeconds         int
	DevAllowHost          bool
}

func configFromArgs(command string, args []string) Config {
	defaultListen := defaultProxyListen
	if command == "api" || command == "pvedaemon" {
		defaultListen = defaultAPIListen
	}

	cfg := Config{
		Listen:                envDefault("DIONYSUS_PVE_PROXY_LISTEN", defaultListen),
		ProcRoot:              envDefault("DIONYSUS_PROC_ROOT", defaultProcRoot),
		SysRoot:               envDefault("DIONYSUS_SYS_ROOT", defaultSysRoot),
		EtcRoot:               envDefault("DIONYSUS_ETC_ROOT", defaultEtcRoot),
		OllamaAPI:             trimSlash(envDefault("DIONYSUS_OLLAMA_API", defaultOllamaAPI)),
		MetricsDB:             envDefault("DIONYSUS_METRICS_DB", defaultMetricsDB),
		TokenFile:             envDefault("DIONYSUS_PVE_TOKEN_FILE", defaultTokenFile),
		AuthUsersFile:         envDefault("DIONYSUS_PVE_USERS_FILE", defaultAuthUsersFile),
		AuthUserFile:          envDefault("DIONYSUS_PVE_USER_FILE", defaultAuthUserFile),
		AuthPasswordFile:      envDefault("DIONYSUS_PVE_PASSWORD_FILE", defaultAuthPasswordFile),
		AuthUsername:          envDefault("DIONYSUS_PVE_USERNAME", defaultAuthUsername),
		AuthPassword:          os.Getenv("DIONYSUS_PVE_PASSWORD"),
		WWWRoot:               envDefault("DIONYSUS_PVE_WWW", defaultWWWRoot),
		NetworkConfig:         envDefault("DIONYSUS_NETWORK_CONFIG", defaultNetworkConfig),
		FirewallConfig:        envDefault("DIONYSUS_FIREWALL_CONFIG", defaultFirewallConfig),
		IntervalSeconds:       envInt("DIONYSUS_METRICS_INTERVAL_SECONDS", defaultIntervalSecs),
		RetentionDays:         envInt("DIONYSUS_METRICS_RETENTION_DAYS", defaultRetentionDays),
		ConsoleTimeoutSeconds: envInt("DIONYSUS_CONSOLE_TIMEOUT_SECONDS", defaultConsoleTimeoutSecs),
		JWTTTLSeconds:         envInt("DIONYSUS_JWT_TTL_SECONDS", defaultJWTTTLSeconds),
		DevAllowHost:          envBool(devAllowHostEnv, false),
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
	flags.StringVar(&cfg.AuthUsersFile, "auth-users-file", cfg.AuthUsersFile, "")
	flags.StringVar(&cfg.AuthUserFile, "auth-user-file", cfg.AuthUserFile, "")
	flags.StringVar(&cfg.AuthPasswordFile, "auth-password-file", cfg.AuthPasswordFile, "")
	flags.StringVar(&cfg.AuthUsername, "auth-username", cfg.AuthUsername, "")
	flags.StringVar(&cfg.AuthPassword, "auth-password", cfg.AuthPassword, "")
	flags.StringVar(&cfg.WWWRoot, "www-root", cfg.WWWRoot, "")
	flags.StringVar(&cfg.NetworkConfig, "network-config", cfg.NetworkConfig, "")
	flags.StringVar(&cfg.FirewallConfig, "firewall-config", cfg.FirewallConfig, "")
	flags.IntVar(&cfg.IntervalSeconds, "interval", cfg.IntervalSeconds, "")
	flags.IntVar(&cfg.RetentionDays, "retention-days", cfg.RetentionDays, "")
	flags.IntVar(&cfg.ConsoleTimeoutSeconds, "console-timeout", cfg.ConsoleTimeoutSeconds, "")
	flags.IntVar(&cfg.JWTTTLSeconds, "jwt-ttl", cfg.JWTTTLSeconds, "")
	flags.BoolVar(&cfg.DevAllowHost, "dev-allow-host", cfg.DevAllowHost, "")
	_ = flags.Parse(args)

	cfg.OllamaAPI = trimSlash(cfg.OllamaAPI)
	if cfg.IntervalSeconds <= 0 {
		cfg.IntervalSeconds = defaultIntervalSecs
	}
	if cfg.RetentionDays < 1 {
		cfg.RetentionDays = defaultRetentionDays
	}
	if cfg.ConsoleTimeoutSeconds < 1 {
		cfg.ConsoleTimeoutSeconds = defaultConsoleTimeoutSecs
	}
	if cfg.JWTTTLSeconds < 60 {
		cfg.JWTTTLSeconds = defaultJWTTTLSeconds
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
