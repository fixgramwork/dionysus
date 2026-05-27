use serde_json::{json, Value};
use std::collections::HashMap;
use std::env;
use std::fs;
use std::io::{BufRead, BufReader, Read, Write};
use std::net::{TcpListener, TcpStream};
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

const DEFAULT_PROXY_LISTEN: &str = "0.0.0.0:8006";
const DEFAULT_API_LISTEN: &str = "127.0.0.1:8507";
const DEFAULT_PROC_ROOT: &str = "/proc";
const DEFAULT_SYS_ROOT: &str = "/sys";
const DEFAULT_ETC_ROOT: &str = "/etc";
const DEFAULT_OLLAMA_API: &str = "http://127.0.0.1:11434";
const DEFAULT_METRICS_DB: &str = "/var/lib/dionysus/metrics/ollama.sqlite3";
const DEFAULT_TOKEN_FILE: &str = "/etc/dionysus/pve.token";
const DEFAULT_WWW_ROOT: &str = "/usr/share/dionysus-pve-manager/www";
const DEFAULT_NETWORK_CONFIG: &str = "/etc/dionysus/network.env";
const DEV_ALLOW_HOST_ENV: &str = "DIONYSUS_DEV_ALLOW_HOST";
const DEFAULT_INTERVAL_SECONDS: u64 = 30;
const DEFAULT_RETENTION_DAYS: i64 = 7;
const MAX_HISTORY_LIMIT: i64 = 20_160;

#[derive(Clone, Debug)]
struct Config {
    listen: String,
    proc_root: PathBuf,
    sys_root: PathBuf,
    etc_root: PathBuf,
    ollama_api: String,
    metrics_db: PathBuf,
    token_file: PathBuf,
    www_root: PathBuf,
    network_config: PathBuf,
    interval_seconds: u64,
    retention_days: i64,
    dev_allow_host: bool,
}

#[derive(Debug)]
struct HttpRequest {
    method: String,
    path: String,
    query: HashMap<String, String>,
    headers: HashMap<String, String>,
    body: String,
}

#[derive(Debug)]
struct HttpResponse {
    status: u16,
    content_type: String,
    body: Vec<u8>,
}

#[derive(Debug)]
struct OllamaGet {
    reachable: bool,
    status: u16,
    data: Value,
    error: String,
}

#[derive(Clone, Debug)]
struct KernelTarget {
    id: &'static str,
    label: &'static str,
    path: PathBuf,
    value: &'static str,
    reason: &'static str,
}

fn main() {
    let mut args: Vec<String> = env::args().collect();
    let command = args.get(1).cloned().unwrap_or_else(|| "proxy".to_string());
    if args.len() > 1 {
        args.remove(1);
    }

    let default_listen = match command.as_str() {
        "api" | "pvedaemon" => DEFAULT_API_LISTEN,
        _ => DEFAULT_PROXY_LISTEN,
    };
    let config = Config::from_args(&args, default_listen);

    let result = match command.as_str() {
        "api" | "pvedaemon" => ensure_target_os_runtime(&config).and_then(|_| serve_api(&config)),
        "proxy" | "pveproxy" => {
            ensure_target_os_runtime(&config).and_then(|_| serve_proxy(&config))
        }
        "metricsd" => ensure_target_os_runtime(&config).and_then(|_| run_metricsd(&config, false)),
        "sample-once" => {
            ensure_target_os_runtime(&config).and_then(|_| run_metricsd(&config, true))
        }
        _ => {
            eprintln!("unsupported command: {command}");
            std::process::exit(2);
        }
    };

    if let Err(error) = result {
        eprintln!("[dionysusd] {error}");
        std::process::exit(1);
    }
}

impl Config {
    fn from_args(args: &[String], default_listen: &str) -> Self {
        let mut config = Self {
            listen: env_default("DIONYSUS_PVE_PROXY_LISTEN", default_listen),
            proc_root: PathBuf::from(env_default("DIONYSUS_PROC_ROOT", DEFAULT_PROC_ROOT)),
            sys_root: PathBuf::from(env_default("DIONYSUS_SYS_ROOT", DEFAULT_SYS_ROOT)),
            etc_root: PathBuf::from(env_default("DIONYSUS_ETC_ROOT", DEFAULT_ETC_ROOT)),
            ollama_api: trim_slash(&env_default("DIONYSUS_OLLAMA_API", DEFAULT_OLLAMA_API)),
            metrics_db: PathBuf::from(env_default("DIONYSUS_METRICS_DB", DEFAULT_METRICS_DB)),
            token_file: PathBuf::from(env_default("DIONYSUS_PVE_TOKEN_FILE", DEFAULT_TOKEN_FILE)),
            www_root: PathBuf::from(env_default("DIONYSUS_PVE_WWW", DEFAULT_WWW_ROOT)),
            network_config: PathBuf::from(env_default(
                "DIONYSUS_NETWORK_CONFIG",
                DEFAULT_NETWORK_CONFIG,
            )),
            interval_seconds: env_u64(
                "DIONYSUS_METRICS_INTERVAL_SECONDS",
                DEFAULT_INTERVAL_SECONDS,
            ),
            retention_days: env_i64("DIONYSUS_METRICS_RETENTION_DAYS", DEFAULT_RETENTION_DAYS),
            dev_allow_host: env_bool(DEV_ALLOW_HOST_ENV, false),
        };

        let mut index = 1;
        while index < args.len() {
            match args[index].as_str() {
                "--listen" => {
                    config.listen = read_arg(args, &mut index);
                }
                "--proc-root" => {
                    config.proc_root = PathBuf::from(read_arg(args, &mut index));
                }
                "--sys-root" => {
                    config.sys_root = PathBuf::from(read_arg(args, &mut index));
                }
                "--etc-root" => {
                    config.etc_root = PathBuf::from(read_arg(args, &mut index));
                }
                "--ollama-api" => {
                    config.ollama_api = trim_slash(&read_arg(args, &mut index));
                }
                "--metrics-db" => {
                    config.metrics_db = PathBuf::from(read_arg(args, &mut index));
                }
                "--token-file" => {
                    config.token_file = PathBuf::from(read_arg(args, &mut index));
                }
                "--www-root" => {
                    config.www_root = PathBuf::from(read_arg(args, &mut index));
                }
                "--network-config" => {
                    config.network_config = PathBuf::from(read_arg(args, &mut index));
                }
                "--interval" => {
                    config.interval_seconds = read_arg(args, &mut index)
                        .parse()
                        .unwrap_or(DEFAULT_INTERVAL_SECONDS);
                }
                "--retention-days" => {
                    config.retention_days = read_arg(args, &mut index)
                        .parse()
                        .unwrap_or(DEFAULT_RETENTION_DAYS);
                }
                "--dev-allow-host" => {
                    config.dev_allow_host = true;
                }
                _ => {}
            }
            index += 1;
        }

        if config.interval_seconds == 0 {
            config.interval_seconds = DEFAULT_INTERVAL_SECONDS;
        }
        if config.retention_days < 1 {
            config.retention_days = DEFAULT_RETENTION_DAYS;
        }

        config
    }
}

fn ensure_target_os_runtime(config: &Config) -> Result<(), String> {
    if config.dev_allow_host {
        eprintln!(
            "[dionysusd] development host override enabled; this mode is not a production OS runtime"
        );
        return Ok(());
    }

    let mut missing = Vec::new();
    if !cfg!(target_os = "linux") {
        missing.push("host OS is not Linux".to_string());
    }
    if !config.proc_root.join("meminfo").is_file() {
        missing.push(format!(
            "{} is missing",
            config.proc_root.join("meminfo").display()
        ));
    }
    if !config.sys_root.join("kernel").is_dir() {
        missing.push(format!(
            "{} is missing",
            config.sys_root.join("kernel").display()
        ));
    }
    if !Path::new("/run/systemd/system").is_dir() {
        missing.push("/run/systemd/system is missing".to_string());
    }
    if Command::new("systemctl").arg("--version").output().is_err() {
        missing.push("systemctl is unavailable".to_string());
    }

    if missing.is_empty() {
        Ok(())
    } else {
        Err(format!(
            "Dionysus manager must run inside the target Linux/systemd OS; {}. For explicit local UI-only development, pass --dev-allow-host or set {DEV_ALLOW_HOST_ENV}=1.",
            missing.join(", ")
        ))
    }
}

fn serve_api(config: &Config) -> Result<(), String> {
    let listener = TcpListener::bind(&config.listen)
        .map_err(|error| format!("failed to listen on {}: {error}", config.listen))?;
    println!("[dionysusd-api] listening on {}", config.listen);

    for stream in listener.incoming() {
        match stream {
            Ok(mut stream) => {
                if let Err(error) = handle_connection(&mut stream, config, false) {
                    eprintln!("[dionysusd-api] request failed: {error}");
                }
            }
            Err(error) => eprintln!("[dionysusd-api] accept failed: {error}"),
        }
    }

    Ok(())
}

fn serve_proxy(config: &Config) -> Result<(), String> {
    let listener = TcpListener::bind(&config.listen)
        .map_err(|error| format!("failed to listen on {}: {error}", config.listen))?;
    println!("[dionysusd-pveproxy] listening on {}", config.listen);

    for stream in listener.incoming() {
        match stream {
            Ok(mut stream) => {
                if let Err(error) = handle_connection(&mut stream, config, true) {
                    eprintln!("[dionysusd-pveproxy] request failed: {error}");
                }
            }
            Err(error) => eprintln!("[dionysusd-pveproxy] accept failed: {error}"),
        }
    }

    Ok(())
}

fn handle_connection(
    stream: &mut TcpStream,
    config: &Config,
    serve_static: bool,
) -> Result<(), String> {
    let request = read_request(stream)?;
    let response = if request.method == "OPTIONS" {
        HttpResponse::empty(204)
    } else if request.path.starts_with("/api2/json/") {
        if let Err(error) = authorize(config, &request) {
            json_response(401, json!({ "errors": { "auth": error }, "data": null }))
        } else {
            handle_api(config, &request)
        }
    } else if serve_static && (request.path == "/" || request.path == "/index.html") {
        file_response(
            &config.www_root.join("index.html"),
            "text/html; charset=utf-8",
        )
    } else if serve_static && request.path.starts_with("/static/") {
        let name = request.path.trim_start_matches("/static/");
        if name
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || "._-".contains(c))
        {
            file_response(
                &config.www_root.join("static").join(name),
                content_type(name),
            )
        } else {
            json_response(
                404,
                json!({ "errors": { "path": "not found" }, "data": null }),
            )
        }
    } else {
        json_response(
            404,
            json!({ "errors": { "path": "not found" }, "data": null }),
        )
    };

    write_response(stream, response)
}

fn handle_api(config: &Config, request: &HttpRequest) -> HttpResponse {
    let result = match (request.method.as_str(), request.path.as_str()) {
        ("GET", "/api2/json/version") => Ok(
            json!({ "version": "0.1", "release": "dionysus-rust", "stack": "rust-systemd", "api": "api2-json" }),
        ),
        ("GET", "/api2/json/nodes") => Ok(
            json!([{ "node": "localhost", "type": "node", "status": "online", "id": "node/localhost" }]),
        ),
        ("GET", "/api2/json/nodes/localhost/status") => Ok(node_status(config)),
        ("GET", "/api2/json/nodes/localhost/services") => Ok(service_status()),
        ("GET", "/api2/json/nodes/localhost/network/status") => Ok(network_status(config)),
        ("GET", "/api2/json/nodes/localhost/network/config") => {
            Ok(network_config_payload(&config.network_config))
        }
        ("POST", "/api2/json/nodes/localhost/network/config") => {
            let body = serde_json::from_str::<Value>(&request.body).unwrap_or_else(|_| json!({}));
            update_network_config(config, &body)
        }
        ("POST", "/api2/json/nodes/localhost/network/apply") => {
            let body = serde_json::from_str::<Value>(&request.body).unwrap_or_else(|_| json!({}));
            let dry_run = body.get("dryRun").and_then(Value::as_bool).unwrap_or(false);
            Ok(apply_network_service(config, dry_run))
        }
        ("GET", "/api2/json/nodes/localhost/ollama/status") => Ok(ollama_status(config)),
        ("GET", "/api2/json/nodes/localhost/ollama/kv-cache/profile") => {
            Ok(kv_cache_profile_status(config))
        }
        ("GET", "/api2/json/nodes/localhost/ollama/history") => {
            let limit = request
                .query
                .get("limit")
                .and_then(|value| value.parse::<i64>().ok())
                .unwrap_or(120);
            Ok(metrics_history(&config.metrics_db, limit))
        }
        ("POST", "/api2/json/nodes/localhost/ollama/optimize") => {
            let body = serde_json::from_str::<Value>(&request.body).unwrap_or_else(|_| json!({}));
            let dry_run = body.get("dryRun").and_then(Value::as_bool).unwrap_or(false);
            Ok(apply_ollama_profile(config, dry_run))
        }
        _ => Err("not found".to_string()),
    };

    match result {
        Ok(data) => json_response(200, json!({ "data": data })),
        Err(error) => json_response(404, json!({ "errors": { "path": error }, "data": null })),
    }
}

fn run_metricsd(config: &Config, once: bool) -> Result<(), String> {
    println!(
        "[dionysusd-metricsd] writing Ollama RAM samples to {}",
        config.metrics_db.display()
    );

    loop {
        match record_sample(config) {
            Ok(summary) => {
                println!(
                    "[dionysusd-metricsd] collected_at={} rss={} swap={} models={}",
                    summary["collectedAt"],
                    summary["processRss"],
                    summary["processSwap"],
                    summary["loadedModelCount"]
                );
            }
            Err(error) => eprintln!("[dionysusd-metricsd] sample failed: {error}"),
        }

        if once {
            return Ok(());
        }
        std::thread::sleep(Duration::from_secs(config.interval_seconds));
    }
}

fn record_sample(config: &Config) -> Result<Value, String> {
    let sample = collect_sample(config);
    init_metrics_schema(&config.metrics_db)?;
    let summary = sample_summary(&sample);
    let retention_seconds = config.retention_days * 24 * 60 * 60;
    let cutoff = summary["collectedAt"].as_i64().unwrap_or(now()) - retention_seconds;
    let sql = format!(
        "INSERT INTO ollama_samples (collected_at, memory_total, memory_used, memory_available, swap_total, swap_used, ollama_reachable, process_count, process_rss, process_swap, loaded_model_count, loaded_model_size, loaded_model_vram, sample_json) VALUES ({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}); DELETE FROM ollama_samples WHERE collected_at < {};",
        summary["collectedAt"],
        summary["memoryTotal"],
        summary["memoryUsed"],
        summary["memoryAvailable"],
        summary["swapTotal"],
        summary["swapUsed"],
        summary["ollamaReachable"],
        summary["processCount"],
        summary["processRss"],
        summary["processSwap"],
        summary["loadedModelCount"],
        summary["loadedModelSize"],
        summary["loadedModelVram"],
        sql_quote(&sample.to_string()),
        cutoff
    );
    run_sqlite(&config.metrics_db, &sql)?;
    Ok(summary)
}

fn collect_sample(config: &Config) -> Value {
    json!({
        "collectedAt": now(),
        "node": node_status(config),
        "ollama": ollama_status(config),
    })
}

fn node_status(config: &Config) -> Value {
    let meminfo = read_meminfo(&config.proc_root);
    let total = get_num(&meminfo, "MemTotal");
    let available = get_num(&meminfo, "MemAvailable");
    let free = get_num(&meminfo, "MemFree");
    let cached = get_num(&meminfo, "Cached");
    let swap_total = get_num(&meminfo, "SwapTotal");
    let swap_free = get_num(&meminfo, "SwapFree");
    let swap_used = positive(swap_total - swap_free);

    json!({
        "node": "localhost",
        "os": os_status(config),
        "controlPlane": control_plane_status(config),
        "uptime": read_uptime(&config.proc_root),
        "loadavg": read_loadavg(&config.proc_root),
        "memory": {
            "total": total,
            "free": free,
            "available": available,
            "cached": cached,
            "used": positive(total - available),
        },
        "swap": {
            "total": swap_total,
            "free": swap_free,
            "used": swap_used,
            "cached": get_num(&meminfo, "SwapCached"),
            "usedPercent": if swap_total > 0 { swap_used as f64 / swap_total as f64 * 100.0 } else { 0.0 },
        },
        "localtime": now(),
    })
}

fn os_status(config: &Config) -> Value {
    let os_release = read_env_file(&config.etc_root.join("os-release"));
    let hostname = first_non_empty(&[
        read_trim(config.proc_root.join("sys/kernel/hostname")),
        read_trim(config.etc_root.join("hostname")),
        command_stdout("hostname", &[]).unwrap_or_default(),
    ]);
    let kernel_name = first_non_empty(&[
        read_trim(config.proc_root.join("sys/kernel/ostype")),
        command_stdout("uname", &["-s"]).unwrap_or_default(),
    ]);
    let kernel_release = first_non_empty(&[
        read_trim(config.proc_root.join("sys/kernel/osrelease")),
        command_stdout("uname", &["-r"]).unwrap_or_default(),
    ]);
    let kernel_version = first_non_empty(&[
        read_trim(config.proc_root.join("sys/kernel/version")),
        command_stdout("uname", &["-v"]).unwrap_or_default(),
    ]);
    let architecture =
        command_stdout("uname", &["-m"]).unwrap_or_else(|_| env::consts::ARCH.to_string());
    let os_name = env_string(&os_release, "NAME", "");
    let os_pretty_name = first_non_empty(&[
        env_string(&os_release, "PRETTY_NAME", ""),
        os_name.clone(),
        kernel_name.clone(),
    ]);

    json!({
        "hostname": hostname,
        "name": os_name,
        "prettyName": os_pretty_name,
        "id": env_string(&os_release, "ID", ""),
        "version": env_string(&os_release, "VERSION", ""),
        "versionId": env_string(&os_release, "VERSION_ID", ""),
        "kernel": {
            "name": kernel_name,
            "release": kernel_release,
            "version": kernel_version,
            "architecture": architecture,
        },
    })
}

fn control_plane_status(config: &Config) -> Value {
    json!({
        "stack": "rust-systemd",
        "api": "api2-json",
        "runtimeMode": if config.dev_allow_host { "development" } else { "target-os" },
        "listen": config.listen,
        "wwwRoot": config.www_root.display().to_string(),
        "metricsDb": config.metrics_db.display().to_string(),
        "networkConfig": config.network_config.display().to_string(),
        "tokenAuth": config.token_file.exists(),
    })
}

fn service_status() -> Value {
    json!([
        service("dionysus-pvedaemon"),
        service("dionysus-pveproxy"),
        service("dionysus-metricsd"),
        service("dionysus-network"),
        service("dionysus-llm-swap"),
        service("ollama"),
        service("docker"),
        service("pve-qemu-kvm"),
        service("lxc"),
    ])
}

fn network_status(config: &Config) -> Value {
    json!({
        "config": network_config_payload(&config.network_config),
        "service": service("dionysus-network"),
        "interfaces": network_interfaces(config),
        "routes": command_stdout("ip", &["route"]).unwrap_or_default(),
        "resolvConf": read_trim(PathBuf::from("/etc/resolv.conf")),
    })
}

fn network_config_payload(path: &Path) -> Value {
    let env = read_env_file(path);
    let wifi_enabled = env_bool_value(&env, "DIONYSUS_WIFI_ENABLED", false);
    let lan_enabled = env_bool_value(&env, "DIONYSUS_LAN_ENABLED", false);
    let mode = env
        .get("DIONYSUS_NETWORK_MODE")
        .filter(|value| !value.is_empty())
        .cloned()
        .unwrap_or_else(|| {
            if wifi_enabled {
                "wifi".to_string()
            } else if lan_enabled {
                "lan".to_string()
            } else {
                "none".to_string()
            }
        });

    json!({
        "path": path.display().to_string(),
        "exists": path.exists(),
        "networkEnabled": env_bool_value(&env, "DIONYSUS_NETWORK_ENABLED", true),
        "mode": mode,
        "lan": {
            "enabled": lan_enabled,
            "iface": env_string(&env, "DIONYSUS_LAN_IFACE", "eth0"),
            "ipv4": {
                "method": env_string(&env, "DIONYSUS_LAN_IPV4_METHOD", "dhcp"),
                "address": env_string(&env, "DIONYSUS_LAN_IPV4_ADDRESS", ""),
                "gateway": env_string(&env, "DIONYSUS_LAN_IPV4_GATEWAY", ""),
                "dns": env_string(&env, "DIONYSUS_LAN_DNS", ""),
            },
        },
        "wifi": {
            "enabled": wifi_enabled,
            "iface": env_string(&env, "DIONYSUS_WIFI_IFACE", "wlan0"),
            "country": env_string(&env, "DIONYSUS_WIFI_COUNTRY", "KR"),
            "ssid": env_string(&env, "DIONYSUS_WIFI_SSID", ""),
            "scanSsid": env_bool_value(&env, "DIONYSUS_WIFI_SCAN_SSID", false),
            "pskStored": env.get("DIONYSUS_WIFI_PSK").map(|value| !value.is_empty()).unwrap_or(false),
            "pskHashStored": env.get("DIONYSUS_WIFI_PSK_HASH").map(|value| !value.is_empty()).unwrap_or(false),
            "ipv4": {
                "method": env_string(&env, "DIONYSUS_WIFI_IPV4_METHOD", "dhcp"),
                "address": env_string(&env, "DIONYSUS_WIFI_IPV4_ADDRESS", ""),
                "gateway": env_string(&env, "DIONYSUS_WIFI_IPV4_GATEWAY", ""),
                "dns": env_string(&env, "DIONYSUS_WIFI_DNS", ""),
            },
        },
        "dhcp": {
            "client": env_string(&env, "DIONYSUS_DHCP_CLIENT", "auto"),
            "tries": env_string(&env, "DIONYSUS_DHCP_TRIES", "6"),
            "timeout": env_string(&env, "DIONYSUS_DHCP_TIMEOUT", "5"),
        },
        "resolvConf": env_string(&env, "DIONYSUS_RESOLV_CONF", "/etc/resolv.conf"),
    })
}

fn update_network_config(config: &Config, body: &Value) -> Result<Value, String> {
    let dry_run = body.get("dryRun").and_then(Value::as_bool).unwrap_or(false);
    let apply = body.get("apply").and_then(Value::as_bool).unwrap_or(false);
    let current = read_env_file(&config.network_config);
    let next = network_env_from_payload(body, &current)?;
    let rendered = render_network_env(&next);

    if !dry_run {
        if let Some(parent) = config.network_config.parent() {
            fs::create_dir_all(parent)
                .map_err(|error| format!("failed to create {}: {error}", parent.display()))?;
        }
        fs::write(&config.network_config, rendered.as_bytes()).map_err(|error| {
            format!(
                "failed to write {}: {error}",
                config.network_config.display()
            )
        })?;
        set_private_mode(&config.network_config);
    }

    let apply_result = if apply {
        Some(apply_network_service(config, dry_run))
    } else {
        None
    };

    Ok(json!({
        "dryRun": dry_run,
        "written": !dry_run,
        "apply": apply,
        "config": network_config_from_env(&next, &config.network_config),
        "rendered": "",
        "applyResult": apply_result,
    }))
}

fn apply_network_service(config: &Config, dry_run: bool) -> Value {
    if dry_run {
        return json!({
            "dryRun": true,
            "status": "dry-run",
            "command": "systemctl restart dionysus-network.service",
        });
    }
    if config.dev_allow_host {
        return json!({
            "dryRun": false,
            "status": "dev-skip",
            "command": "systemctl restart dionysus-network.service",
            "reason": "development host override is enabled; service restart was not executed",
        });
    }

    let output = Command::new("systemctl")
        .arg("restart")
        .arg("dionysus-network.service")
        .output();
    match output {
        Ok(output) => json!({
            "dryRun": false,
            "status": if output.status.success() { "restarted" } else { "failed" },
            "exitCode": output.status.code().unwrap_or(-1),
            "stdout": String::from_utf8_lossy(&output.stdout).trim().to_string(),
            "stderr": String::from_utf8_lossy(&output.stderr).trim().to_string(),
        }),
        Err(error) => json!({
            "dryRun": false,
            "status": "failed",
            "error": error.to_string(),
        }),
    }
}

fn network_env_from_payload(
    body: &Value,
    current: &HashMap<String, String>,
) -> Result<HashMap<String, String>, String> {
    let mode = json_string(
        body,
        "mode",
        env_string(current, "DIONYSUS_NETWORK_MODE", "none"),
    );
    validate_choice(&mode, &["none", "lan", "wifi"], "network mode")?;
    let network_enabled = json_bool(body, "networkEnabled", true);
    let lan = &body["lan"];
    let wifi = &body["wifi"];
    let dhcp = &body["dhcp"];

    let lan_iface = json_string(
        lan,
        "iface",
        env_string(current, "DIONYSUS_LAN_IFACE", "eth0"),
    );
    let wifi_iface = json_string(
        wifi,
        "iface",
        env_string(current, "DIONYSUS_WIFI_IFACE", "wlan0"),
    );
    validate_iface(&lan_iface)?;
    validate_iface(&wifi_iface)?;

    let lan_ipv4 = &lan["ipv4"];
    let wifi_ipv4 = &wifi["ipv4"];
    let lan_method = json_string(
        lan_ipv4,
        "method",
        env_string(current, "DIONYSUS_LAN_IPV4_METHOD", "dhcp"),
    );
    let wifi_method = json_string(
        wifi_ipv4,
        "method",
        env_string(current, "DIONYSUS_WIFI_IPV4_METHOD", "dhcp"),
    );
    validate_choice(&lan_method, &["dhcp", "static", "none"], "LAN IPv4 method")?;
    validate_choice(
        &wifi_method,
        &["dhcp", "static", "none"],
        "Wi-Fi IPv4 method",
    )?;

    let wifi_country = json_string(
        wifi,
        "country",
        env_string(current, "DIONYSUS_WIFI_COUNTRY", "KR"),
    )
    .to_ascii_uppercase();
    if wifi_country.len() != 2 || !wifi_country.chars().all(|c| c.is_ascii_alphabetic()) {
        return Err("Wi-Fi country must be a two-letter country code".to_string());
    }

    let mut next = HashMap::new();
    next.insert(
        "DIONYSUS_NETWORK_ENABLED".to_string(),
        bool_env(network_enabled),
    );
    next.insert("DIONYSUS_NETWORK_MODE".to_string(), mode.clone());
    next.insert(
        "DIONYSUS_LAN_ENABLED".to_string(),
        bool_env(json_bool(lan, "enabled", mode == "lan")),
    );
    next.insert("DIONYSUS_LAN_IFACE".to_string(), lan_iface);
    next.insert("DIONYSUS_LAN_IPV4_METHOD".to_string(), lan_method);
    next.insert(
        "DIONYSUS_LAN_IPV4_ADDRESS".to_string(),
        clean_single_line(&json_string(lan_ipv4, "address", "".to_string()))?,
    );
    next.insert(
        "DIONYSUS_LAN_IPV4_GATEWAY".to_string(),
        clean_single_line(&json_string(lan_ipv4, "gateway", "".to_string()))?,
    );
    next.insert(
        "DIONYSUS_LAN_DNS".to_string(),
        clean_single_line(&json_string(lan_ipv4, "dns", "".to_string()))?,
    );

    next.insert(
        "DIONYSUS_WIFI_ENABLED".to_string(),
        bool_env(json_bool(wifi, "enabled", mode == "wifi")),
    );
    next.insert("DIONYSUS_WIFI_IFACE".to_string(), wifi_iface);
    next.insert("DIONYSUS_WIFI_COUNTRY".to_string(), wifi_country);
    next.insert(
        "DIONYSUS_WIFI_SSID".to_string(),
        clean_single_line(&json_string(wifi, "ssid", "".to_string()))?,
    );
    next.insert(
        "DIONYSUS_WIFI_SCAN_SSID".to_string(),
        bool_env(json_bool(wifi, "scanSsid", false)),
    );
    next.insert(
        "DIONYSUS_WIFI_IPV4_METHOD".to_string(),
        wifi_method.to_string(),
    );
    next.insert(
        "DIONYSUS_WIFI_IPV4_ADDRESS".to_string(),
        clean_single_line(&json_string(wifi_ipv4, "address", "".to_string()))?,
    );
    next.insert(
        "DIONYSUS_WIFI_IPV4_GATEWAY".to_string(),
        clean_single_line(&json_string(wifi_ipv4, "gateway", "".to_string()))?,
    );
    next.insert(
        "DIONYSUS_WIFI_DNS".to_string(),
        clean_single_line(&json_string(wifi_ipv4, "dns", "".to_string()))?,
    );

    let psk_action = json_string(wifi, "pskAction", "preserve".to_string());
    validate_choice(
        &psk_action,
        &["preserve", "set", "clear"],
        "Wi-Fi PSK action",
    )?;
    let wifi_psk = match psk_action.as_str() {
        "set" => clean_single_line(&json_string(wifi, "psk", "".to_string()))?,
        "clear" => String::new(),
        _ => env_string(current, "DIONYSUS_WIFI_PSK", ""),
    };
    next.insert("DIONYSUS_WIFI_PSK".to_string(), wifi_psk);
    next.insert(
        "DIONYSUS_WIFI_PSK_HASH".to_string(),
        clean_single_line(&json_string(
            wifi,
            "pskHash",
            env_string(current, "DIONYSUS_WIFI_PSK_HASH", ""),
        ))?,
    );

    next.insert(
        "DIONYSUS_DHCP_CLIENT".to_string(),
        clean_single_line(&json_string(
            dhcp,
            "client",
            env_string(current, "DIONYSUS_DHCP_CLIENT", "auto"),
        ))?,
    );
    next.insert(
        "DIONYSUS_DHCP_TRIES".to_string(),
        clean_single_line(&json_string(
            dhcp,
            "tries",
            env_string(current, "DIONYSUS_DHCP_TRIES", "6"),
        ))?,
    );
    next.insert(
        "DIONYSUS_DHCP_TIMEOUT".to_string(),
        clean_single_line(&json_string(
            dhcp,
            "timeout",
            env_string(current, "DIONYSUS_DHCP_TIMEOUT", "5"),
        ))?,
    );
    next.insert(
        "DIONYSUS_RESOLV_CONF".to_string(),
        clean_single_line(&json_string(
            body,
            "resolvConf",
            env_string(current, "DIONYSUS_RESOLV_CONF", "/etc/resolv.conf"),
        ))?,
    );

    Ok(next)
}

fn network_config_from_env(env: &HashMap<String, String>, path: &Path) -> Value {
    network_config_payload_from_env(env, path)
}

fn network_config_payload_from_env(env: &HashMap<String, String>, path: &Path) -> Value {
    let mode = env_string(env, "DIONYSUS_NETWORK_MODE", "none");
    json!({
        "path": path.display().to_string(),
        "exists": path.exists(),
        "networkEnabled": env_bool_value(env, "DIONYSUS_NETWORK_ENABLED", true),
        "mode": mode,
        "lan": {
            "enabled": env_bool_value(env, "DIONYSUS_LAN_ENABLED", false),
            "iface": env_string(env, "DIONYSUS_LAN_IFACE", "eth0"),
            "ipv4": {
                "method": env_string(env, "DIONYSUS_LAN_IPV4_METHOD", "dhcp"),
                "address": env_string(env, "DIONYSUS_LAN_IPV4_ADDRESS", ""),
                "gateway": env_string(env, "DIONYSUS_LAN_IPV4_GATEWAY", ""),
                "dns": env_string(env, "DIONYSUS_LAN_DNS", ""),
            },
        },
        "wifi": {
            "enabled": env_bool_value(env, "DIONYSUS_WIFI_ENABLED", false),
            "iface": env_string(env, "DIONYSUS_WIFI_IFACE", "wlan0"),
            "country": env_string(env, "DIONYSUS_WIFI_COUNTRY", "KR"),
            "ssid": env_string(env, "DIONYSUS_WIFI_SSID", ""),
            "scanSsid": env_bool_value(env, "DIONYSUS_WIFI_SCAN_SSID", false),
            "pskStored": env.get("DIONYSUS_WIFI_PSK").map(|value| !value.is_empty()).unwrap_or(false),
            "pskHashStored": env.get("DIONYSUS_WIFI_PSK_HASH").map(|value| !value.is_empty()).unwrap_or(false),
            "ipv4": {
                "method": env_string(env, "DIONYSUS_WIFI_IPV4_METHOD", "dhcp"),
                "address": env_string(env, "DIONYSUS_WIFI_IPV4_ADDRESS", ""),
                "gateway": env_string(env, "DIONYSUS_WIFI_IPV4_GATEWAY", ""),
                "dns": env_string(env, "DIONYSUS_WIFI_DNS", ""),
            },
        },
        "dhcp": {
            "client": env_string(env, "DIONYSUS_DHCP_CLIENT", "auto"),
            "tries": env_string(env, "DIONYSUS_DHCP_TRIES", "6"),
            "timeout": env_string(env, "DIONYSUS_DHCP_TIMEOUT", "5"),
        },
        "resolvConf": env_string(env, "DIONYSUS_RESOLV_CONF", "/etc/resolv.conf"),
    })
}

fn network_interfaces(config: &Config) -> Value {
    let mut stats = HashMap::new();
    let net_dev = config.proc_root.join("net/dev");
    if let Ok(content) = fs::read_to_string(net_dev) {
        for line in content.lines().skip(2) {
            if let Some((name, data)) = line.split_once(':') {
                let fields = data.split_whitespace().collect::<Vec<_>>();
                if fields.len() >= 16 {
                    stats.insert(
                        name.trim().to_string(),
                        (
                            fields[0].parse::<i64>().unwrap_or(0),
                            fields[8].parse::<i64>().unwrap_or(0),
                        ),
                    );
                }
            }
        }
    }

    let mut addresses: HashMap<String, Vec<String>> = HashMap::new();
    if let Ok(content) = command_stdout("ip", &["-o", "addr", "show"]) {
        for line in content.lines() {
            let fields = line.split_whitespace().collect::<Vec<_>>();
            if fields.len() >= 4 && matches!(fields[2], "inet" | "inet6") {
                let iface = fields[1]
                    .trim_end_matches(':')
                    .split('@')
                    .next()
                    .unwrap_or(fields[1])
                    .to_string();
                addresses
                    .entry(iface)
                    .or_default()
                    .push(format!("{} {}", fields[2], fields[3]));
            }
        }
    }

    let class_net = config.sys_root.join("class/net");
    let mut interfaces = Vec::new();
    if let Ok(entries) = fs::read_dir(class_net) {
        for entry in entries.flatten() {
            let name = entry.file_name().to_string_lossy().to_string();
            let path = entry.path();
            let (rx_bytes, tx_bytes) = stats.get(&name).copied().unwrap_or((0, 0));
            let iface_addresses = addresses.get(&name).cloned().unwrap_or_default();
            interfaces.push(json!({
                "name": name,
                "address": read_trim(path.join("address")),
                "addresses": iface_addresses,
                "operstate": read_trim(path.join("operstate")),
                "mtu": read_trim(path.join("mtu")),
                "rxBytes": rx_bytes,
                "txBytes": tx_bytes,
            }));
        }
    } else {
        for (name, (rx_bytes, tx_bytes)) in stats {
            let iface_addresses = addresses.get(&name).cloned().unwrap_or_default();
            interfaces.push(json!({
                "name": name,
                "address": "",
                "addresses": iface_addresses,
                "operstate": "",
                "mtu": "",
                "rxBytes": rx_bytes,
                "txBytes": tx_bytes,
            }));
        }
    }
    interfaces.sort_by(|a, b| {
        a["name"]
            .as_str()
            .unwrap_or("")
            .cmp(b["name"].as_str().unwrap_or(""))
    });
    json!(interfaces)
}

fn ollama_status(config: &Config) -> Value {
    let meminfo = read_meminfo(&config.proc_root);
    let swap_total = get_num(&meminfo, "SwapTotal");
    let swap_free = get_num(&meminfo, "SwapFree");
    let swap_used = positive(swap_total - swap_free);
    let processes = ollama_processes(&config.proc_root);
    let api = ollama_api_status(&config.ollama_api);
    let service = service("ollama");
    let platform = ollama_platform(&config.ollama_api, &api, &processes, &service);
    let runtime = runtime_summary(&processes);
    let loaded_model_memory = loaded_model_summary(array_or_empty(&api["loadedModels"]));

    let mut status = json!({
        "platform": platform,
        "apiBase": config.ollama_api,
        "apiReachable": api["reachable"],
        "apiStatus": api["status"],
        "version": api["version"],
        "running": !processes.is_empty() || api["reachable"].as_bool().unwrap_or(false) || service["state"] == "active",
        "models": api["models"],
        "loadedModels": api["loadedModels"],
        "loadedModelsReachable": api["loadedModelsReachable"],
        "loadedModelsError": api["loadedModelsError"],
        "processes": processes,
        "runtime": runtime,
        "loadedModelMemory": loaded_model_memory,
        "service": service,
        "swap": {
            "total": swap_total,
            "free": swap_free,
            "used": swap_used,
            "cached": get_num(&meminfo, "SwapCached"),
            "usedPercent": if swap_total > 0 { swap_used as f64 / swap_total as f64 * 100.0 } else { 0.0 },
        },
        "kernel": kernel_tuning(config),
        "kvCache": kv_cache_profile_status(config),
    });

    if !api["error"].as_str().unwrap_or("").is_empty() {
        status["error"] = api["error"].clone();
    }
    status["recommendations"] = recommendations(&status);
    status
}

fn ollama_api_status(base: &str) -> Value {
    let tags = ollama_get_json(base, "/api/tags");
    let version = ollama_get_json(base, "/api/version");
    let ps = ollama_get_json(base, "/api/ps");
    parse_ollama_api(&tags, &version, &ps)
}

fn parse_ollama_api(tags: &OllamaGet, version: &OllamaGet, ps: &OllamaGet) -> Value {
    let models = tags
        .data
        .get("models")
        .and_then(Value::as_array)
        .unwrap_or(&Vec::new())
        .iter()
        .map(|model| {
            json!({
                "name": model["name"].as_str().unwrap_or(""),
                "size": model["size"].as_i64().unwrap_or(0),
                "digest": model["digest"].as_str().unwrap_or(""),
                "modifiedAt": model["modified_at"].as_str().unwrap_or(""),
            })
        })
        .collect::<Vec<_>>();

    let loaded_models = ps
        .data
        .get("models")
        .and_then(Value::as_array)
        .unwrap_or(&Vec::new())
        .iter()
        .map(|model| {
            let details = &model["details"];
            json!({
                "name": model["name"].as_str().unwrap_or(""),
                "model": model["model"].as_str().or_else(|| model["name"].as_str()).unwrap_or(""),
                "size": model["size"].as_i64().unwrap_or(0),
                "sizeVram": model["size_vram"].as_i64().unwrap_or(0),
                "digest": model["digest"].as_str().unwrap_or(""),
                "expiresAt": model["expires_at"].as_str().unwrap_or(""),
                "contextLength": model["context_length"].as_i64().unwrap_or(0),
                "details": {
                    "format": details["format"].as_str().unwrap_or(""),
                    "family": details["family"].as_str().unwrap_or(""),
                    "parameterSize": details["parameter_size"].as_str().unwrap_or(""),
                    "quantizationLevel": details["quantization_level"].as_str().unwrap_or(""),
                },
            })
        })
        .collect::<Vec<_>>();

    let reachable = tags.reachable || version.reachable;
    let status = if tags.reachable {
        tags.status
    } else if version.reachable {
        version.status
    } else if tags.status != 0 {
        tags.status
    } else {
        version.status
    };
    let errors = [tags.error.as_str(), version.error.as_str()]
        .into_iter()
        .filter(|error| !error.is_empty())
        .collect::<Vec<_>>()
        .join("; ");

    json!({
        "reachable": reachable,
        "status": status,
        "version": version.data["version"].as_str().unwrap_or(""),
        "models": models,
        "loadedModels": loaded_models,
        "loadedModelsReachable": ps.reachable,
        "loadedModelsError": ps.error,
        "error": errors,
    })
}

fn apply_ollama_profile(config: &Config, dry_run: bool) -> Value {
    let targets = kv_cache_targets(config);
    let before = kv_cache_profile_status(config);
    let results = targets
        .iter()
        .map(|target| {
            let current = read_trim(target.path.clone());
            if !target.path.exists() {
                return json!({
                    "id": target.id,
                    "label": target.label,
                    "path": target.path.display().to_string(),
                    "current": current,
                    "value": target.value,
                    "status": "skipped",
                    "error": "path does not exist",
                });
            }
            if dry_run {
                return json!({
                    "id": target.id,
                    "label": target.label,
                    "path": target.path.display().to_string(),
                    "current": current,
                    "value": target.value,
                    "status": "dry-run",
                });
            }
            match fs::write(&target.path, target.value) {
                Ok(()) => json!({
                    "id": target.id,
                    "label": target.label,
                    "path": target.path.display().to_string(),
                    "current": current,
                    "value": target.value,
                    "status": "written",
                }),
                Err(error) => json!({
                    "id": target.id,
                    "label": target.label,
                    "path": target.path.display().to_string(),
                    "current": current,
                    "value": target.value,
                    "status": "failed",
                    "error": error.to_string(),
                }),
            }
        })
        .collect::<Vec<_>>();
    let after = kv_cache_profile_status(config);
    let failed = count_write_status(&results, "failed");
    let skipped = count_write_status(&results, "skipped");
    let written = count_write_status(&results, "written");
    let dry = count_write_status(&results, "dry-run");
    let status = if failed > 0 {
        "failed"
    } else if dry_run {
        "dry-run"
    } else if written > 0 {
        "applied"
    } else {
        "no-op"
    };

    json!({
        "profile": "ollama-kv-cache",
        "description": "Manual kernel profile for bounded Ollama KV-cache overflow into Dionysus-managed swap.",
        "dryRun": dry_run,
        "status": status,
        "summary": {
            "written": written,
            "dryRun": dry,
            "skipped": skipped,
            "failed": failed,
            "pendingBefore": before["pendingCount"].as_i64().unwrap_or(0),
            "pendingAfter": after["pendingCount"].as_i64().unwrap_or(0),
            "ready": after["ready"].as_bool().unwrap_or(false),
        },
        "before": before,
        "after": after,
        "writes": results,
        "console": kv_cache_console(status, dry_run, written, skipped, failed, &after),
    })
}

fn kv_cache_targets(config: &Config) -> Vec<KernelTarget> {
    vec![
        KernelTarget {
            id: "swappiness",
            label: "vm.swappiness",
            path: config.proc_root.join("sys/vm/swappiness"),
            value: "80",
            reason: "prefer earlier bounded swap use when RAM pressure rises",
        },
        KernelTarget {
            id: "page-cluster",
            label: "vm.page-cluster",
            path: config.proc_root.join("sys/vm/page-cluster"),
            value: "0",
            reason: "avoid swap readahead for random KV-cache access",
        },
        KernelTarget {
            id: "vfs-cache-pressure",
            label: "vm.vfs_cache_pressure",
            path: config.proc_root.join("sys/vm/vfs_cache_pressure"),
            value: "40",
            reason: "keep model file cache warmer under memory pressure",
        },
        KernelTarget {
            id: "watermark-scale-factor",
            label: "vm.watermark_scale_factor",
            path: config.proc_root.join("sys/vm/watermark_scale_factor"),
            value: "125",
            reason: "give reclaim more headroom before hard pressure",
        },
        KernelTarget {
            id: "transparent-hugepage",
            label: "transparent_hugepage/enabled",
            path: config
                .sys_root
                .join("kernel/mm/transparent_hugepage/enabled"),
            value: "madvise",
            reason: "avoid unconditional THP while preserving opt-in huge pages",
        },
    ]
}

fn kv_cache_profile_status(config: &Config) -> Value {
    let targets = kv_cache_targets(config)
        .into_iter()
        .map(|target| kv_cache_target_status(&target))
        .collect::<Vec<_>>();
    let pending_count = targets
        .iter()
        .filter(|target| target["status"] == "pending")
        .count();
    let missing_count = targets
        .iter()
        .filter(|target| target["status"] == "missing")
        .count();
    let ready_count = targets
        .iter()
        .filter(|target| target["status"] == "ready")
        .count();
    let meminfo = read_meminfo(&config.proc_root);
    let swap_total = get_num(&meminfo, "SwapTotal");
    let swap_free = get_num(&meminfo, "SwapFree");
    let swap_used = positive(swap_total - swap_free);
    let swap_service = service("dionysus-llm-swap");
    let ready = pending_count == 0 && missing_count == 0 && swap_total > 0;
    let mut warnings = Vec::new();
    if swap_total == 0 {
        warnings.push(json!({
            "severity": "warning",
            "text": "Dionysus-managed swap is not visible in /proc/meminfo.",
            "action": "enable dionysus-llm-swap.service before heavy Ollama workloads",
        }));
    }
    if missing_count > 0 {
        warnings.push(json!({
            "severity": "warning",
            "text": "Some kernel tuning paths are missing on this host.",
            "action": "verify the target is a Linux systemd host with procfs/sysfs mounted",
        }));
    }
    if pending_count > 0 {
        warnings.push(json!({
            "severity": "info",
            "text": "KV-cache kernel profile has unapplied values.",
            "action": "preview, then apply the Ollama KV-cache profile manually",
        }));
    }

    json!({
        "profile": "ollama-kv-cache",
        "ready": ready,
        "readyCount": ready_count,
        "pendingCount": pending_count,
        "missingCount": missing_count,
        "targets": targets,
        "swap": {
            "service": swap_service,
            "total": swap_total,
            "free": swap_free,
            "used": swap_used,
            "enabled": swap_total > 0,
            "usedPercent": if swap_total > 0 { swap_used as f64 / swap_total as f64 * 100.0 } else { 0.0 },
        },
        "warnings": warnings,
    })
}

fn kv_cache_target_status(target: &KernelTarget) -> Value {
    let current = read_trim(target.path.clone());
    let exists = target.path.exists();
    let status = if !exists {
        "missing"
    } else if kernel_value_matches(&current, target.value) {
        "ready"
    } else {
        "pending"
    };
    json!({
        "id": target.id,
        "label": target.label,
        "path": target.path.display().to_string(),
        "current": current,
        "target": target.value,
        "status": status,
        "reason": target.reason,
    })
}

fn kernel_value_matches(current: &str, target: &str) -> bool {
    if target == "madvise" {
        current == "madvise" || current.contains("[madvise]")
    } else {
        current == target
    }
}

fn count_write_status(writes: &[Value], status: &str) -> usize {
    writes
        .iter()
        .filter(|write| write["status"].as_str().unwrap_or("") == status)
        .count()
}

fn kv_cache_console(
    status: &str,
    dry_run: bool,
    written: usize,
    skipped: usize,
    failed: usize,
    after: &Value,
) -> Value {
    let mut lines = Vec::new();
    lines.push(json!({
        "level": if failed > 0 { "error" } else if dry_run { "warn" } else { "info" },
        "message": format!("ollama-kv-cache profile {status}"),
    }));
    lines.push(json!({
        "level": "info",
        "message": format!("writes={written} skipped={skipped} failed={failed} pending={}", after["pendingCount"].as_i64().unwrap_or(0)),
    }));
    lines.push(json!({
        "level": if after["swap"]["enabled"].as_bool().unwrap_or(false) { "info" } else { "warn" },
        "message": format!(
            "swap={} used={} service={}",
            after["swap"]["total"].as_i64().unwrap_or(0),
            after["swap"]["used"].as_i64().unwrap_or(0),
            after["swap"]["service"]["state"].as_str().unwrap_or("unknown")
        ),
    }));
    json!(lines)
}

fn metrics_history(db_path: &Path, limit: i64) -> Value {
    if init_metrics_schema(db_path).is_err() {
        return json!([]);
    }
    let limit = limit.clamp(1, MAX_HISTORY_LIMIT);
    let sql = format!(
        "SELECT id, collected_at AS collectedAt, memory_total AS memoryTotal, memory_used AS memoryUsed, memory_available AS memoryAvailable, swap_total AS swapTotal, swap_used AS swapUsed, ollama_reachable AS ollamaReachable, process_count AS processCount, process_rss AS processRss, process_swap AS processSwap, loaded_model_count AS loadedModelCount, loaded_model_size AS loadedModelSize, loaded_model_vram AS loadedModelVram, sample_json AS sampleJson FROM ollama_samples ORDER BY collected_at DESC LIMIT {limit};"
    );
    let output = match query_sqlite_json(db_path, &sql) {
        Ok(value) => value,
        Err(_) => return json!([]),
    };
    let mut rows = output.as_array().cloned().unwrap_or_default();
    rows.reverse();
    json!(rows
        .into_iter()
        .map(|row| {
            json!({
                "id": row["id"].as_i64().unwrap_or(0),
                "collectedAt": row["collectedAt"].as_i64().unwrap_or(0),
                "memory": {
                    "total": row["memoryTotal"].as_i64().unwrap_or(0),
                    "used": row["memoryUsed"].as_i64().unwrap_or(0),
                    "available": row["memoryAvailable"].as_i64().unwrap_or(0),
                },
                "swap": {
                    "total": row["swapTotal"].as_i64().unwrap_or(0),
                    "used": row["swapUsed"].as_i64().unwrap_or(0),
                },
                "ollama": {
                    "reachable": row["ollamaReachable"].as_i64().unwrap_or(0) != 0,
                    "processCount": row["processCount"].as_i64().unwrap_or(0),
                    "processRss": row["processRss"].as_i64().unwrap_or(0),
                    "processSwap": row["processSwap"].as_i64().unwrap_or(0),
                    "loadedModelCount": row["loadedModelCount"].as_i64().unwrap_or(0),
                    "loadedModelSize": row["loadedModelSize"].as_i64().unwrap_or(0),
                    "loadedModelVram": row["loadedModelVram"].as_i64().unwrap_or(0),
                },
                "sample": serde_json::from_str::<Value>(row["sampleJson"].as_str().unwrap_or("{}")).unwrap_or_else(|_| json!({})),
            })
        })
        .collect::<Vec<_>>())
}

fn init_metrics_schema(db_path: &Path) -> Result<(), String> {
    if let Some(parent) = db_path.parent() {
        fs::create_dir_all(parent)
            .map_err(|error| format!("failed to create {}: {error}", parent.display()))?;
    }
    run_sqlite(
        db_path,
        "CREATE TABLE IF NOT EXISTS ollama_samples (id INTEGER PRIMARY KEY AUTOINCREMENT, collected_at INTEGER NOT NULL, memory_total INTEGER NOT NULL DEFAULT 0, memory_used INTEGER NOT NULL DEFAULT 0, memory_available INTEGER NOT NULL DEFAULT 0, swap_total INTEGER NOT NULL DEFAULT 0, swap_used INTEGER NOT NULL DEFAULT 0, ollama_reachable INTEGER NOT NULL DEFAULT 0, process_count INTEGER NOT NULL DEFAULT 0, process_rss INTEGER NOT NULL DEFAULT 0, process_swap INTEGER NOT NULL DEFAULT 0, loaded_model_count INTEGER NOT NULL DEFAULT 0, loaded_model_size INTEGER NOT NULL DEFAULT 0, loaded_model_vram INTEGER NOT NULL DEFAULT 0, sample_json TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_ollama_samples_collected_at ON ollama_samples(collected_at);",
    )
}

fn run_sqlite(db_path: &Path, sql: &str) -> Result<(), String> {
    let output = Command::new("sqlite3")
        .arg(db_path)
        .arg(sql)
        .output()
        .map_err(|error| format!("failed to run sqlite3: {error}"))?;
    if output.status.success() {
        Ok(())
    } else {
        Err(String::from_utf8_lossy(&output.stderr).trim().to_string())
    }
}

fn query_sqlite_json(db_path: &Path, sql: &str) -> Result<Value, String> {
    let output = Command::new("sqlite3")
        .arg("-json")
        .arg(db_path)
        .arg(sql)
        .output()
        .map_err(|error| format!("failed to run sqlite3: {error}"))?;
    if !output.status.success() {
        return Err(String::from_utf8_lossy(&output.stderr).trim().to_string());
    }
    let stdout = String::from_utf8_lossy(&output.stdout);
    serde_json::from_str(stdout.trim()).map_err(|error| error.to_string())
}

fn sample_summary(sample: &Value) -> Value {
    json!({
        "collectedAt": sample["collectedAt"].as_i64().unwrap_or_else(now),
        "memoryTotal": sample["node"]["memory"]["total"].as_i64().unwrap_or(0),
        "memoryUsed": sample["node"]["memory"]["used"].as_i64().unwrap_or(0),
        "memoryAvailable": sample["node"]["memory"]["available"].as_i64().unwrap_or(0),
        "swapTotal": sample["node"]["swap"]["total"].as_i64().unwrap_or(0),
        "swapUsed": sample["node"]["swap"]["used"].as_i64().unwrap_or(0),
        "ollamaReachable": if sample["ollama"]["apiReachable"].as_bool().unwrap_or(false) { 1 } else { 0 },
        "processCount": sample["ollama"]["runtime"]["processCount"].as_i64().unwrap_or(0),
        "processRss": sample["ollama"]["runtime"]["processRss"].as_i64().unwrap_or(0),
        "processSwap": sample["ollama"]["runtime"]["processSwap"].as_i64().unwrap_or(0),
        "loadedModelCount": sample["ollama"]["loadedModelMemory"]["count"].as_i64().unwrap_or(0),
        "loadedModelSize": sample["ollama"]["loadedModelMemory"]["size"].as_i64().unwrap_or(0),
        "loadedModelVram": sample["ollama"]["loadedModelMemory"]["sizeVram"].as_i64().unwrap_or(0),
    })
}

fn read_meminfo(proc_root: &Path) -> HashMap<String, i64> {
    let mut values = HashMap::new();
    let content = fs::read_to_string(proc_root.join("meminfo")).unwrap_or_default();
    for line in content.lines() {
        let mut parts = line.split_whitespace();
        if let (Some(key), Some(value)) = (parts.next(), parts.next()) {
            let key = key.trim_end_matches(':').to_string();
            let bytes = value.parse::<i64>().unwrap_or(0) * 1024;
            values.insert(key, bytes);
        }
    }
    values
}

fn read_uptime(proc_root: &Path) -> f64 {
    fs::read_to_string(proc_root.join("uptime"))
        .ok()
        .and_then(|content| {
            content
                .split_whitespace()
                .next()
                .and_then(|value| value.parse::<f64>().ok())
        })
        .unwrap_or(0.0)
}

fn read_loadavg(proc_root: &Path) -> Value {
    let parts = fs::read_to_string(proc_root.join("loadavg"))
        .unwrap_or_default()
        .split_whitespace()
        .take(3)
        .map(|value| value.to_string())
        .collect::<Vec<_>>();
    json!(parts)
}

fn kernel_tuning(config: &Config) -> Value {
    json!({
        "swappiness": read_trim(config.proc_root.join("sys/vm/swappiness")),
        "pageCluster": read_trim(config.proc_root.join("sys/vm/page-cluster")),
        "vfsCachePressure": read_trim(config.proc_root.join("sys/vm/vfs_cache_pressure")),
        "watermarkScaleFactor": read_trim(config.proc_root.join("sys/vm/watermark_scale_factor")),
        "transparentHugepage": read_trim(config.sys_root.join("kernel/mm/transparent_hugepage/enabled")),
    })
}

fn ollama_processes(proc_root: &Path) -> Vec<Value> {
    let mut processes = Vec::new();
    let entries = match fs::read_dir(proc_root) {
        Ok(entries) => entries,
        Err(_) => return processes,
    };
    for entry in entries.flatten() {
        let name = entry.file_name().to_string_lossy().to_string();
        if !name.chars().all(|c| c.is_ascii_digit()) {
            continue;
        }
        let dir = entry.path();
        let comm = read_trim(dir.join("comm"));
        let cmdline = read_trim(dir.join("cmdline")).replace('\0', " ");
        let search = format!("{comm} {cmdline}").to_lowercase();
        if !search.contains("ollama") && !search.contains("llama") {
            continue;
        }
        let status = process_status(&dir.join("status"));
        processes.push(json!({
            "pid": name.parse::<i64>().unwrap_or(0),
            "name": comm,
            "command": cmdline,
            "rss": get_num(&status, "VmRSS"),
            "peakRss": get_num(&status, "VmHWM"),
            "swap": get_num(&status, "VmSwap"),
        }));
    }
    processes
}

fn process_status(path: &Path) -> HashMap<String, i64> {
    let mut values = HashMap::new();
    let content = fs::read_to_string(path).unwrap_or_default();
    for line in content.lines() {
        let mut parts = line.split_whitespace();
        if let (Some(key), Some(value)) = (parts.next(), parts.next()) {
            let key = key.trim_end_matches(':');
            if matches!(key, "VmRSS" | "VmHWM" | "VmSwap") {
                values.insert(key.to_string(), value.parse::<i64>().unwrap_or(0) * 1024);
            }
        }
    }
    values
}

fn service(name: &str) -> Value {
    let state = Command::new("systemctl")
        .arg("is-active")
        .arg(name)
        .output()
        .ok()
        .map(|output| String::from_utf8_lossy(&output.stdout).trim().to_string())
        .filter(|state| !state.is_empty())
        .unwrap_or_else(|| "unknown".to_string());
    json!({ "name": name, "state": state })
}

fn ollama_platform(api_base: &str, api: &Value, processes: &[Value], service: &Value) -> Value {
    let mut sources = Vec::new();
    if api["reachable"].as_bool().unwrap_or(false) {
        sources.push("api");
    }
    if !processes.is_empty() {
        sources.push("process");
    }
    if service["state"] == "active" {
        sources.push("systemd");
    }

    let state = if api["reachable"].as_bool().unwrap_or(false) {
        "api-online".to_string()
    } else if !processes.is_empty() {
        "process-only".to_string()
    } else if service["state"] == "active" {
        "service-active".to_string()
    } else if service["state"] != "unknown" {
        format!("service-{}", service["state"].as_str().unwrap_or("unknown"))
    } else {
        "not-detected".to_string()
    };

    json!({
        "name": "Ollama",
        "type": "local-llm",
        "detected": !sources.is_empty(),
        "state": state,
        "apiBase": api_base,
        "apiReachable": api["reachable"],
        "apiStatus": api["status"],
        "version": api["version"],
        "service": service,
        "detectionSources": sources,
        "modelCount": api["models"].as_array().map(Vec::len).unwrap_or(0),
        "loadedModelCount": api["loadedModels"].as_array().map(Vec::len).unwrap_or(0),
        "processCount": processes.len(),
    })
}

fn runtime_summary(processes: &[Value]) -> Value {
    let process_rss = processes
        .iter()
        .map(|p| p["rss"].as_i64().unwrap_or(0))
        .sum::<i64>();
    let process_peak = processes
        .iter()
        .map(|p| p["peakRss"].as_i64().unwrap_or(0))
        .sum::<i64>();
    let process_swap = processes
        .iter()
        .map(|p| p["swap"].as_i64().unwrap_or(0))
        .sum::<i64>();
    json!({
        "processCount": processes.len(),
        "processRss": process_rss,
        "processPeakRss": process_peak,
        "processSwap": process_swap,
    })
}

fn loaded_model_summary(models: &[Value]) -> Value {
    let size = models
        .iter()
        .map(|m| m["size"].as_i64().unwrap_or(0))
        .sum::<i64>();
    let size_vram = models
        .iter()
        .map(|m| m["sizeVram"].as_i64().unwrap_or(0))
        .sum::<i64>();
    json!({ "count": models.len(), "size": size, "sizeVram": size_vram })
}

fn recommendations(status: &Value) -> Value {
    let mut items = Vec::new();
    let platform = &status["platform"];
    if !platform["detected"].as_bool().unwrap_or(false) {
        items.push(json!({
            "severity": "warning",
            "text": "Ollama local LLM platform is not detected on this node.",
            "action": "start ollama.service or set DIONYSUS_OLLAMA_API to the local Ollama endpoint",
        }));
    } else if !status["apiReachable"].as_bool().unwrap_or(false) {
        items.push(json!({
            "severity": "warning",
            "text": "Ollama was detected, but its HTTP API is not reachable.",
            "action": "check ollama.service, OLLAMA_HOST, and DIONYSUS_OLLAMA_API",
        }));
    }
    if status["apiReachable"].as_bool().unwrap_or(false)
        && status["models"].as_array().map(Vec::len).unwrap_or(0) == 0
    {
        items.push(json!({
            "severity": "info",
            "text": "Ollama API is reachable, but no local models are listed.",
            "action": "pull a model with ollama pull before serving workloads",
        }));
    }
    if status["swap"]["total"].as_i64().unwrap_or(0) == 0 {
        items.push(json!({
            "severity": "warning",
            "text": "LLM swap backing store is disabled.",
            "action": "enable dionysus-llm-swap.service before starting ollama.service",
        }));
    }
    if status["kernel"]["swappiness"]
        .as_str()
        .unwrap_or("0")
        .parse::<i64>()
        .unwrap_or(0)
        < 60
    {
        items.push(json!({
            "severity": "info",
            "text": "swappiness is conservative for KV-cache overflow.",
            "action": "apply /api2/json/nodes/localhost/ollama/optimize",
        }));
    }
    if status["kernel"]["pageCluster"]
        .as_str()
        .unwrap_or("0")
        .parse::<i64>()
        .unwrap_or(0)
        > 0
    {
        items.push(json!({
            "severity": "info",
            "text": "swap readahead is enabled; KV-cache access is often random.",
            "action": "set vm.page-cluster=0",
        }));
    }
    if status["swap"]["usedPercent"].as_f64().unwrap_or(0.0) >= 80.0 {
        items.push(json!({
            "severity": "warning",
            "text": "LLM swap backing store is close to full.",
            "action": "increase DIONYSUS_LLM_SWAP_SIZE_MB or reduce concurrent model load",
        }));
    }
    if status["runtime"]["processSwap"].as_i64().unwrap_or(0) > 0 {
        items.push(json!({
            "severity": "info",
            "text": "Ollama processes are using swap.",
            "action": "watch latency and keep swap as bounded overflow, not a speed path",
        }));
    }
    json!(items)
}

fn ollama_get_json(base: &str, path: &str) -> OllamaGet {
    match http_get_json(base, path) {
        Ok(get) => get,
        Err(error) => OllamaGet {
            reachable: false,
            status: 0,
            data: json!({}),
            error,
        },
    }
}

fn http_get_json(base: &str, path: &str) -> Result<OllamaGet, String> {
    let (host, port) = parse_http_base(base)?;
    let mut stream = TcpStream::connect((&host[..], port)).map_err(|error| error.to_string())?;
    stream
        .set_read_timeout(Some(Duration::from_secs(1)))
        .map_err(|error| error.to_string())?;
    stream
        .set_write_timeout(Some(Duration::from_secs(1)))
        .map_err(|error| error.to_string())?;
    let request = format!("GET {path} HTTP/1.1\r\nHost: {host}\r\nConnection: close\r\n\r\n");
    stream
        .write_all(request.as_bytes())
        .map_err(|error| error.to_string())?;
    let mut raw = String::new();
    stream
        .read_to_string(&mut raw)
        .map_err(|error| error.to_string())?;
    let (head, body) = raw
        .split_once("\r\n\r\n")
        .ok_or_else(|| "invalid http response".to_string())?;
    let status = head
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(0);
    if !(200..300).contains(&status) {
        return Ok(OllamaGet {
            reachable: false,
            status,
            data: json!({}),
            error: format!("http status {status}"),
        });
    }
    let data = serde_json::from_str(body).map_err(|_| "invalid json".to_string())?;
    Ok(OllamaGet {
        reachable: true,
        status,
        data,
        error: String::new(),
    })
}

fn parse_http_base(base: &str) -> Result<(String, u16), String> {
    let rest = base
        .strip_prefix("http://")
        .ok_or_else(|| "only http Ollama endpoints are supported".to_string())?;
    let host_port = rest.split('/').next().unwrap_or(rest);
    let (host, port) = match host_port.split_once(':') {
        Some((host, port)) => (host.to_string(), port.parse::<u16>().unwrap_or(80)),
        None => (host_port.to_string(), 80),
    };
    Ok((host, port))
}

fn read_request(stream: &mut TcpStream) -> Result<HttpRequest, String> {
    let mut reader = BufReader::new(stream);
    let mut first = String::new();
    reader
        .read_line(&mut first)
        .map_err(|error| error.to_string())?;
    if first.trim().is_empty() {
        return Err("empty request".to_string());
    }
    let mut first_parts = first.split_whitespace();
    let method = first_parts.next().unwrap_or("").to_string();
    let target = first_parts.next().unwrap_or("/").to_string();
    let (path, query) = split_target(&target);
    let mut headers = HashMap::new();
    loop {
        let mut line = String::new();
        reader
            .read_line(&mut line)
            .map_err(|error| error.to_string())?;
        let line = line.trim_end_matches(['\r', '\n']);
        if line.is_empty() {
            break;
        }
        if let Some((key, value)) = line.split_once(':') {
            headers.insert(key.trim().to_ascii_lowercase(), value.trim().to_string());
        }
    }
    let length = headers
        .get("content-length")
        .and_then(|value| value.parse::<usize>().ok())
        .unwrap_or(0);
    let mut body = vec![0; length];
    if length > 0 {
        reader
            .read_exact(&mut body)
            .map_err(|error| error.to_string())?;
    }
    Ok(HttpRequest {
        method,
        path,
        query,
        headers,
        body: String::from_utf8_lossy(&body).to_string(),
    })
}

fn write_response(stream: &mut TcpStream, response: HttpResponse) -> Result<(), String> {
    let status_text = match response.status {
        200 => "OK",
        204 => "No Content",
        401 => "Unauthorized",
        404 => "Not Found",
        _ => "OK",
    };
    let headers = format!(
        "HTTP/1.1 {} {}\r\nContent-Length: {}\r\nContent-Type: {}\r\nAccess-Control-Allow-Origin: *\r\nAccess-Control-Allow-Headers: Content-Type, Authorization\r\nAccess-Control-Allow-Methods: GET, POST, OPTIONS\r\nConnection: close\r\n{}\r\n",
        response.status,
        status_text,
        response.body.len(),
        response.content_type,
        if response.status == 401 {
            "WWW-Authenticate: Bearer realm=\"Dionysus PVE\"\r\n"
        } else {
            ""
        }
    );
    stream
        .write_all(headers.as_bytes())
        .and_then(|_| stream.write_all(&response.body))
        .map_err(|error| error.to_string())
}

impl HttpResponse {
    fn empty(status: u16) -> Self {
        Self {
            status,
            content_type: "text/plain".to_string(),
            body: Vec::new(),
        }
    }
}

fn json_response(status: u16, value: Value) -> HttpResponse {
    HttpResponse {
        status,
        content_type: "application/json".to_string(),
        body: value.to_string().into_bytes(),
    }
}

fn file_response(path: &Path, content_type: &str) -> HttpResponse {
    match fs::read(path) {
        Ok(body) => HttpResponse {
            status: 200,
            content_type: content_type.to_string(),
            body,
        },
        Err(_) => json_response(
            404,
            json!({ "errors": { "path": "not found" }, "data": null }),
        ),
    }
}

fn authorize(config: &Config, request: &HttpRequest) -> Result<(), String> {
    if !config.token_file.exists() {
        return Ok(());
    }
    let expected = fs::read_to_string(&config.token_file)
        .map_err(|_| "token file is unreadable".to_string())?
        .trim()
        .to_string();
    let header = request
        .headers
        .get("authorization")
        .ok_or_else(|| "missing bearer token".to_string())?;
    let provided = header
        .strip_prefix("Bearer ")
        .or_else(|| header.strip_prefix("bearer "))
        .ok_or_else(|| "missing bearer token".to_string())?;
    if constant_time_equal(expected.as_bytes(), provided.as_bytes()) {
        Ok(())
    } else {
        Err("invalid bearer token".to_string())
    }
}

fn constant_time_equal(left: &[u8], right: &[u8]) -> bool {
    let mut diff = left.len() ^ right.len();
    let max = left.len().max(right.len());
    for index in 0..max {
        let a = *left.get(index).unwrap_or(&0);
        let b = *right.get(index).unwrap_or(&0);
        diff |= (a ^ b) as usize;
    }
    diff == 0
}

fn split_target(target: &str) -> (String, HashMap<String, String>) {
    let (path, query) = target.split_once('?').unwrap_or((target, ""));
    let mut params = HashMap::new();
    for part in query.split('&') {
        if part.is_empty() {
            continue;
        }
        let (key, value) = part.split_once('=').unwrap_or((part, ""));
        params.insert(url_decode(key), url_decode(value));
    }
    (path.to_string(), params)
}

fn url_decode(value: &str) -> String {
    let mut output = String::new();
    let mut chars = value.chars().peekable();
    while let Some(ch) = chars.next() {
        if ch == '%' {
            let hex = format!(
                "{}{}",
                chars.next().unwrap_or('0'),
                chars.next().unwrap_or('0')
            );
            if let Ok(byte) = u8::from_str_radix(&hex, 16) {
                output.push(byte as char);
            }
        } else if ch == '+' {
            output.push(' ');
        } else {
            output.push(ch);
        }
    }
    output
}

fn read_trim(path: PathBuf) -> String {
    fs::read_to_string(path)
        .unwrap_or_default()
        .trim()
        .to_string()
}

fn content_type(name: &str) -> &str {
    if name.ends_with(".css") {
        "text/css"
    } else if name.ends_with(".js") {
        "application/javascript"
    } else if name.ends_with(".svg") {
        "image/svg+xml"
    } else {
        "application/octet-stream"
    }
}

fn array_or_empty(value: &Value) -> &[Value] {
    value.as_array().map(Vec::as_slice).unwrap_or(&[])
}

fn get_num(values: &HashMap<String, i64>, key: &str) -> i64 {
    *values.get(key).unwrap_or(&0)
}

fn positive(value: i64) -> i64 {
    if value > 0 {
        value
    } else {
        0
    }
}

fn first_non_empty(values: &[String]) -> String {
    values
        .iter()
        .find(|value| !value.trim().is_empty())
        .cloned()
        .unwrap_or_default()
}

fn now() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64
}

fn sql_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "''"))
}

fn env_default(name: &str, default: &str) -> String {
    env::var(name).unwrap_or_else(|_| default.to_string())
}

fn env_u64(name: &str, default: u64) -> u64 {
    env::var(name)
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(default)
}

fn env_i64(name: &str, default: i64) -> i64 {
    env::var(name)
        .ok()
        .and_then(|value| value.parse().ok())
        .unwrap_or(default)
}

fn env_bool(name: &str, default: bool) -> bool {
    env::var(name)
        .ok()
        .map(|value| matches!(value.as_str(), "1" | "true" | "TRUE" | "yes" | "YES"))
        .unwrap_or(default)
}

fn read_env_file(path: &Path) -> HashMap<String, String> {
    let mut values = HashMap::new();
    let content = fs::read_to_string(path).unwrap_or_default();
    for line in content.lines() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') {
            continue;
        }
        let Some((key, value)) = trimmed.split_once('=') else {
            continue;
        };
        let key = key.trim();
        if key
            .chars()
            .all(|ch| ch.is_ascii_uppercase() || ch.is_ascii_digit() || ch == '_')
        {
            values.insert(key.to_string(), unquote_env_value(value.trim()));
        }
    }
    values
}

fn unquote_env_value(value: &str) -> String {
    if value.len() >= 2 && value.starts_with('\'') && value.ends_with('\'') {
        return value[1..value.len() - 1].replace("'\\''", "'");
    }
    if value.len() >= 2 && value.starts_with('"') && value.ends_with('"') {
        return value[1..value.len() - 1]
            .replace("\\\"", "\"")
            .replace("\\\\", "\\");
    }
    value.to_string()
}

fn env_string(values: &HashMap<String, String>, key: &str, default: &str) -> String {
    values
        .get(key)
        .cloned()
        .unwrap_or_else(|| default.to_string())
}

fn env_bool_value(values: &HashMap<String, String>, key: &str, default: bool) -> bool {
    values
        .get(key)
        .map(|value| matches!(value.as_str(), "1" | "true" | "TRUE" | "yes" | "YES"))
        .unwrap_or(default)
}

fn json_string(value: &Value, key: &str, default: String) -> String {
    value
        .get(key)
        .and_then(Value::as_str)
        .map(str::to_string)
        .unwrap_or(default)
}

fn json_bool(value: &Value, key: &str, default: bool) -> bool {
    value.get(key).and_then(Value::as_bool).unwrap_or(default)
}

fn validate_choice(value: &str, allowed: &[&str], label: &str) -> Result<(), String> {
    if allowed.contains(&value) {
        Ok(())
    } else {
        Err(format!(
            "unsupported {label}: {value}; allowed values: {}",
            allowed.join(", ")
        ))
    }
}

fn validate_iface(value: &str) -> Result<(), String> {
    if value.is_empty()
        || value.len() > 32
        || !value
            .chars()
            .all(|ch| ch.is_ascii_alphanumeric() || matches!(ch, '_' | '-' | '.' | ':'))
    {
        Err(format!("invalid network interface name: {value}"))
    } else {
        Ok(())
    }
}

fn clean_single_line(value: &str) -> Result<String, String> {
    if value.contains('\n') || value.contains('\r') || value.contains('\0') {
        Err("network setting values must be single-line strings".to_string())
    } else {
        Ok(value.trim().to_string())
    }
}

fn bool_env(value: bool) -> String {
    if value {
        "1".to_string()
    } else {
        "0".to_string()
    }
}

fn render_network_env(values: &HashMap<String, String>) -> String {
    let keys = [
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
    ];
    let mut output = String::from(
        "# Persistent Dionysus network settings.\n# Generated by dionysusd; keep mode 0600 when a Wi-Fi PSK is stored here.\n\n",
    );
    for key in keys {
        let value = values.get(key).cloned().unwrap_or_default();
        output.push_str(key);
        output.push('=');
        output.push_str(&shell_quote(&value));
        output.push('\n');
    }
    output
}

fn shell_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\\''"))
}

#[cfg(unix)]
fn set_private_mode(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    if let Ok(metadata) = fs::metadata(path) {
        let mut permissions = metadata.permissions();
        permissions.set_mode(0o600);
        let _ = fs::set_permissions(path, permissions);
    }
}

#[cfg(not(unix))]
fn set_private_mode(_path: &Path) {}

fn command_stdout(command: &str, args: &[&str]) -> Result<String, String> {
    let output = Command::new(command)
        .args(args)
        .output()
        .map_err(|error| error.to_string())?;
    if output.status.success() {
        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    } else {
        Err(String::from_utf8_lossy(&output.stderr).trim().to_string())
    }
}

fn read_arg(args: &[String], index: &mut usize) -> String {
    *index += 1;
    args.get(*index).cloned().unwrap_or_default()
}

fn trim_slash(value: &str) -> String {
    value.trim_end_matches('/').to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_ollama_ps_payload() {
        let tags = OllamaGet {
            reachable: true,
            status: 200,
            data: json!({ "models": [{ "name": "gemma3", "size": 10, "digest": "abc", "modified_at": "now" }] }),
            error: String::new(),
        };
        let version = OllamaGet {
            reachable: true,
            status: 200,
            data: json!({ "version": "0.23.3" }),
            error: String::new(),
        };
        let ps = OllamaGet {
            reachable: true,
            status: 200,
            data: json!({ "models": [{ "name": "gemma3", "model": "gemma3", "size": 100, "size_vram": 80, "context_length": 4096 }] }),
            error: String::new(),
        };
        let parsed = parse_ollama_api(&tags, &version, &ps);
        assert_eq!(parsed["version"], "0.23.3");
        assert_eq!(parsed["models"][0]["name"], "gemma3");
        assert_eq!(parsed["loadedModels"][0]["sizeVram"], 80);
        assert_eq!(parsed["loadedModels"][0]["contextLength"], 4096);
    }

    #[test]
    fn auth_requires_existing_token_file() {
        let root = unique_temp_dir("auth");
        let token_file = root.join("pve.token");
        let config = Config {
            listen: DEFAULT_PROXY_LISTEN.to_string(),
            proc_root: root.clone(),
            sys_root: root.clone(),
            etc_root: root.clone(),
            ollama_api: DEFAULT_OLLAMA_API.to_string(),
            metrics_db: root.join("metrics.sqlite3"),
            token_file: token_file.clone(),
            www_root: root.clone(),
            network_config: root.join("network.env"),
            interval_seconds: 30,
            retention_days: 7,
            dev_allow_host: false,
        };
        let request = HttpRequest {
            method: "GET".to_string(),
            path: "/api2/json/version".to_string(),
            query: HashMap::new(),
            headers: HashMap::new(),
            body: String::new(),
        };
        assert!(authorize(&config, &request).is_ok());
        fs::write(&token_file, "secret\n").unwrap();
        assert!(authorize(&config, &request).is_err());
        let mut wrong_headers = HashMap::new();
        wrong_headers.insert("authorization".to_string(), "Bearer wrong".to_string());
        let wrong_request = HttpRequest {
            method: "GET".to_string(),
            path: "/api2/json/version".to_string(),
            query: HashMap::new(),
            headers: wrong_headers,
            body: String::new(),
        };
        assert!(authorize(&config, &wrong_request).is_err());
        let mut headers = HashMap::new();
        headers.insert("authorization".to_string(), "Bearer secret".to_string());
        let request = HttpRequest { headers, ..request };
        assert!(authorize(&config, &request).is_ok());
    }

    #[test]
    fn parses_proc_memory_and_ollama_processes() {
        let root = unique_temp_dir("proc");
        fs::create_dir_all(root.join("proc/123")).unwrap();
        fs::create_dir_all(root.join("proc/sys/kernel")).unwrap();
        fs::create_dir_all(root.join("etc")).unwrap();
        fs::write(
            root.join("proc/meminfo"),
            "MemTotal: 1024 kB\nMemAvailable: 256 kB\nMemFree: 128 kB\nCached: 64 kB\nSwapTotal: 512 kB\nSwapFree: 128 kB\nSwapCached: 16 kB\n",
        )
        .unwrap();
        fs::write(root.join("proc/uptime"), "10.50 20.00\n").unwrap();
        fs::write(root.join("proc/loadavg"), "0.01 0.02 0.03 1/2 3\n").unwrap();
        fs::write(root.join("proc/sys/kernel/hostname"), "dionysus-test\n").unwrap();
        fs::write(root.join("proc/sys/kernel/ostype"), "Linux\n").unwrap();
        fs::write(root.join("proc/sys/kernel/osrelease"), "6.12.0-test\n").unwrap();
        fs::write(root.join("proc/sys/kernel/version"), "#1 SMP PREEMPT\n").unwrap();
        fs::write(
            root.join("etc/os-release"),
            "NAME=\"Dionysus Test OS\"\nPRETTY_NAME=\"Dionysus Test OS 0.1\"\nID=dionysus\nVERSION_ID=0.1\n",
        )
        .unwrap();
        fs::write(root.join("proc/123/comm"), "ollama\n").unwrap();
        fs::write(root.join("proc/123/cmdline"), b"ollama\0serve\0").unwrap();
        fs::write(
            root.join("proc/123/status"),
            "Name:\tollama\nVmRSS:\t100 kB\nVmHWM:\t120 kB\nVmSwap:\t30 kB\n",
        )
        .unwrap();

        let config = test_config(&root);
        let status = node_status(&config);
        assert_eq!(status["memory"]["total"], 1024 * 1024);
        assert_eq!(status["memory"]["used"], 768 * 1024);
        assert_eq!(status["swap"]["used"], 384 * 1024);
        assert_eq!(status["loadavg"][0], "0.01");
        assert_eq!(status["os"]["hostname"], "dionysus-test");
        assert_eq!(status["os"]["prettyName"], "Dionysus Test OS 0.1");
        assert_eq!(status["os"]["kernel"]["release"], "6.12.0-test");

        let processes = ollama_processes(&config.proc_root);
        assert_eq!(processes.len(), 1);
        assert_eq!(processes[0]["pid"], 123);
        assert_eq!(processes[0]["rss"], 100 * 1024);
        assert_eq!(processes[0]["swap"], 30 * 1024);
    }

    #[test]
    fn target_os_guard_requires_runtime_unless_dev_override_is_explicit() {
        let root = unique_temp_dir("runtime-guard");
        let mut config = test_config(&root);
        assert!(ensure_target_os_runtime(&config).is_err());
        config.dev_allow_host = true;
        assert!(ensure_target_os_runtime(&config).is_ok());
    }

    #[test]
    fn network_config_masks_wifi_secret_and_parses_lan() {
        let root = unique_temp_dir("network-read");
        let config = test_config(&root);
        fs::write(
            &config.network_config,
            "DIONYSUS_NETWORK_ENABLED='1'\nDIONYSUS_NETWORK_MODE='wifi'\nDIONYSUS_LAN_ENABLED='1'\nDIONYSUS_LAN_IFACE='enp1s0'\nDIONYSUS_WIFI_ENABLED='1'\nDIONYSUS_WIFI_IFACE='wlan0'\nDIONYSUS_WIFI_SSID='office'\nDIONYSUS_WIFI_PSK='secret value'\n",
        )
        .unwrap();
        let payload = network_config_payload(&config.network_config);
        assert_eq!(payload["mode"], "wifi");
        assert_eq!(payload["lan"]["iface"], "enp1s0");
        assert_eq!(payload["wifi"]["ssid"], "office");
        assert_eq!(payload["wifi"]["pskStored"], true);
        assert!(payload["wifi"].get("psk").is_none());
        assert!(!payload.to_string().contains("secret value"));
    }

    #[test]
    fn network_config_dry_run_preserves_secret_without_returning_it() {
        let root = unique_temp_dir("network-write");
        let config = test_config(&root);
        fs::write(
            &config.network_config,
            "DIONYSUS_WIFI_PSK='secret value'\nDIONYSUS_WIFI_PSK_HASH=''\n",
        )
        .unwrap();
        let body = json!({
            "dryRun": true,
            "apply": true,
            "networkEnabled": true,
            "mode": "lan",
            "lan": {
                "enabled": true,
                "iface": "eth0",
                "ipv4": {
                    "method": "static",
                    "address": "192.168.10.20/24",
                    "gateway": "192.168.10.1",
                    "dns": "1.1.1.1 8.8.8.8"
                }
            },
            "wifi": {
                "enabled": false,
                "iface": "wlan0",
                "country": "kr",
                "ssid": "office",
                "scanSsid": false,
                "pskAction": "preserve",
                "ipv4": { "method": "dhcp" }
            },
            "dhcp": { "client": "auto", "tries": "6", "timeout": "5" },
            "resolvConf": "/etc/resolv.conf"
        });
        let result = update_network_config(&config, &body).unwrap();
        assert_eq!(result["dryRun"], true);
        assert_eq!(result["written"], false);
        assert_eq!(result["config"]["mode"], "lan");
        assert_eq!(
            result["config"]["lan"]["ipv4"]["address"],
            "192.168.10.20/24"
        );
        assert_eq!(result["config"]["wifi"]["pskStored"], true);
        assert!(!result.to_string().contains("secret value"));
        assert_eq!(
            fs::read_to_string(&config.network_config).unwrap(),
            "DIONYSUS_WIFI_PSK='secret value'\nDIONYSUS_WIFI_PSK_HASH=''\n"
        );
    }

    #[test]
    fn dry_run_does_not_write_sysctl() {
        let root = unique_temp_dir("dry-run");
        fs::create_dir_all(root.join("proc/sys/vm")).unwrap();
        fs::create_dir_all(root.join("sys/kernel/mm/transparent_hugepage")).unwrap();
        fs::write(root.join("proc/sys/vm/swappiness"), "30\n").unwrap();
        fs::write(root.join("proc/sys/vm/page-cluster"), "1\n").unwrap();
        fs::write(root.join("proc/sys/vm/vfs_cache_pressure"), "100\n").unwrap();
        fs::write(root.join("proc/sys/vm/watermark_scale_factor"), "10\n").unwrap();
        fs::write(
            root.join("sys/kernel/mm/transparent_hugepage/enabled"),
            "always [madvise] never\n",
        )
        .unwrap();
        let config = test_config(&root);
        let profile = kv_cache_profile_status(&config);
        assert_eq!(profile["targets"].as_array().unwrap().len(), 5);
        assert_eq!(profile["missingCount"], 0);
        assert!(profile["pendingCount"].as_i64().unwrap() > 0);
        let result = apply_ollama_profile(&config, true);
        assert_eq!(result["writes"][0]["status"], "dry-run");
        assert_eq!(result["status"], "dry-run");
        assert_eq!(result["summary"]["pendingBefore"], profile["pendingCount"]);
        assert_eq!(
            fs::read_to_string(root.join("proc/sys/vm/swappiness")).unwrap(),
            "30\n"
        );
        let result = apply_ollama_profile(&config, false);
        assert_eq!(result["writes"][0]["status"], "written");
        assert_eq!(result["after"]["pendingCount"], 0);
        assert_eq!(
            fs::read_to_string(root.join("proc/sys/vm/swappiness")).unwrap(),
            "80"
        );
    }

    #[test]
    fn metrics_retention_keeps_recent_rows() {
        if Command::new("sqlite3").arg("--version").output().is_err() {
            return;
        }
        let root = unique_temp_dir("metrics");
        let config = test_config(&root);
        let old = json!({
            "collectedAt": now() - 8 * 24 * 60 * 60,
            "node": { "memory": { "total": 1, "used": 1, "available": 0 }, "swap": { "total": 1, "used": 1 } },
            "ollama": { "apiReachable": true, "runtime": { "processCount": 1, "processRss": 1, "processSwap": 1 }, "loadedModelMemory": { "count": 1, "size": 1, "sizeVram": 0 } }
        });
        let recent = json!({
            "collectedAt": now(),
            "node": { "memory": { "total": 2, "used": 1, "available": 1 }, "swap": { "total": 2, "used": 1 } },
            "ollama": { "apiReachable": true, "runtime": { "processCount": 2, "processRss": 20, "processSwap": 3 }, "loadedModelMemory": { "count": 2, "size": 100, "sizeVram": 80 } }
        });
        init_metrics_schema(&config.metrics_db).unwrap();
        insert_sample_for_test(&config.metrics_db, &old, 7).unwrap();
        insert_sample_for_test(&config.metrics_db, &recent, 7).unwrap();
        let history = metrics_history(&config.metrics_db, 10);
        assert_eq!(history.as_array().unwrap().len(), 1);
        assert_eq!(history[0]["ollama"]["loadedModelCount"], 2);
        assert_eq!(history[0]["ollama"]["processRss"], 20);
    }

    fn insert_sample_for_test(
        db: &Path,
        sample: &Value,
        retention_days: i64,
    ) -> Result<(), String> {
        let config = Config {
            metrics_db: db.to_path_buf(),
            retention_days,
            ..test_config(db.parent().unwrap())
        };
        let summary = sample_summary(sample);
        let cutoff =
            summary["collectedAt"].as_i64().unwrap_or(now()) - retention_days * 24 * 60 * 60;
        let sql = format!(
            "INSERT INTO ollama_samples (collected_at, memory_total, memory_used, memory_available, swap_total, swap_used, ollama_reachable, process_count, process_rss, process_swap, loaded_model_count, loaded_model_size, loaded_model_vram, sample_json) VALUES ({}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}, {}); DELETE FROM ollama_samples WHERE collected_at < {};",
            summary["collectedAt"],
            summary["memoryTotal"],
            summary["memoryUsed"],
            summary["memoryAvailable"],
            summary["swapTotal"],
            summary["swapUsed"],
            summary["ollamaReachable"],
            summary["processCount"],
            summary["processRss"],
            summary["processSwap"],
            summary["loadedModelCount"],
            summary["loadedModelSize"],
            summary["loadedModelVram"],
            sql_quote(&sample.to_string()),
            cutoff
        );
        run_sqlite(&config.metrics_db, &sql)
    }

    fn test_config(root: &Path) -> Config {
        Config {
            listen: DEFAULT_PROXY_LISTEN.to_string(),
            proc_root: root.join("proc"),
            sys_root: root.join("sys"),
            etc_root: root.join("etc"),
            ollama_api: DEFAULT_OLLAMA_API.to_string(),
            metrics_db: root.join("metrics.sqlite3"),
            token_file: root.join("pve.token"),
            www_root: root.to_path_buf(),
            network_config: root.join("network.env"),
            interval_seconds: 30,
            retention_days: 7,
            dev_allow_host: false,
        }
    }

    fn unique_temp_dir(name: &str) -> PathBuf {
        let path =
            env::temp_dir().join(format!("dionysus-{name}-{}-{}", std::process::id(), now()));
        let _ = fs::remove_dir_all(&path);
        fs::create_dir_all(&path).unwrap();
        path
    }
}
