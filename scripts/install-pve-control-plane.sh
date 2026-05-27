#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
HOST_INSTALL=0
INSTALL_DEPS="${DIONYSUS_INSTALL_DEPS:-0}"
APT_GET="${APT_GET:-apt-get}"

usage() {
    cat <<'USAGE'
Usage:
  sh scripts/install-pve-control-plane.sh [DESTDIR]
  sudo sh scripts/install-pve-control-plane.sh --host [--install-deps]

Options:
  --host          Install directly into the running Ubuntu/Debian system.
  --install-deps  Run apt-get update/install for missing runtime packages.
  --help          Show this help text.
USAGE
}

DESTDIR_ARG=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        --host)
            HOST_INSTALL=1
            ;;
        --install-deps)
            INSTALL_DEPS=1
            ;;
        --no-install-deps)
            INSTALL_DEPS=0
            ;;
        --help)
            usage
            exit 0
            ;;
        --*)
            $STATUS error "Unsupported option: $1"
            usage
            exit 2
            ;;
        *)
            if [ -n "$DESTDIR_ARG" ]; then
                $STATUS error "Only one DESTDIR argument is supported"
                usage
                exit 2
            fi
            DESTDIR_ARG="$1"
            ;;
    esac
    shift
done

if [ "$HOST_INSTALL" -eq 1 ]; then
    DESTDIR="${DESTDIR:-}"
else
    DESTDIR="${DESTDIR:-${DESTDIR_ARG:-build/pve-control-plane-rootfs}}"
fi

PVE_DIR="${PVE_DIR:-pve}"
SYSTEMD_DIR="${SYSTEMD_DIR:-packaging/systemd}"
DIONYSUS_PROFILES_DIR="${DIONYSUS_PROFILES_DIR:-profiles}"
DIONYSUSD_BIN="${DIONYSUSD_BIN:-}"

if [ "$HOST_INSTALL" -eq 1 ] && [ "$(id -u)" -ne 0 ]; then
    $STATUS error "Host install must be run as root, for example: sudo sh scripts/install-pve-control-plane.sh --host"
    exit 1
fi

if [ "$INSTALL_DEPS" = "1" ] && [ "$HOST_INSTALL" -ne 1 ]; then
    $STATUS error "--install-deps is only supported with --host"
    exit 2
fi

require_file() {
    path="$1"
    if [ ! -f "$path" ]; then
        $STATUS error "Missing required file: $path"
        exit 1
    fi
}

add_package() {
    package="$1"
    case " $MISSING_PACKAGES " in
        *" $package "*) ;;
        *) MISSING_PACKAGES="$MISSING_PACKAGES $package" ;;
    esac
}

check_dependencies() {
    MISSING_PACKAGES=""

    if [ -z "$DIONYSUSD_BIN" ] && [ ! -x target/release/dionysusd ]; then
        command -v cargo >/dev/null 2>&1 || add_package cargo
    fi
    command -v systemctl >/dev/null 2>&1 || add_package systemd
    command -v sqlite3 >/dev/null 2>&1 || add_package sqlite3
    command -v swapon >/dev/null 2>&1 || add_package util-linux
    command -v mkswap >/dev/null 2>&1 || add_package util-linux
    command -v ip >/dev/null 2>&1 || add_package iproute2
    command -v wpa_supplicant >/dev/null 2>&1 || add_package wpasupplicant
    if ! command -v udhcpc >/dev/null 2>&1 && ! command -v dhclient >/dev/null 2>&1; then
        add_package isc-dhcp-client
    fi

    if [ -n "$MISSING_PACKAGES" ]; then
        $STATUS warn "Missing runtime dependencies detected:$MISSING_PACKAGES"
        $STATUS warn "Install them with: sudo apt-get update"
        $STATUS warn "Then run: sudo apt-get install -y --no-install-recommends$MISSING_PACKAGES"
    else
        $STATUS success "Runtime dependencies look available"
    fi
}

install_missing_packages() {
    if [ "$INSTALL_DEPS" != "1" ] || [ -z "$MISSING_PACKAGES" ]; then
        return
    fi

    command -v "$APT_GET" >/dev/null 2>&1 || {
        $STATUS error "--install-deps requires apt-get on the target host"
        exit 1
    }

    $STATUS info "Installing missing runtime dependencies with $APT_GET:$MISSING_PACKAGES"
    "$APT_GET" update
    DEBIAN_FRONTEND=noninteractive "$APT_GET" install -y --no-install-recommends $MISSING_PACKAGES
}

build_dionysusd() {
    if [ -n "$DIONYSUSD_BIN" ]; then
        require_file "$DIONYSUSD_BIN"
        DIONYSUSD_BUILD_BIN="$DIONYSUSD_BIN"
        return
    fi

    if [ ! -x target/release/dionysusd ]; then
        $STATUS info "Building Rust control-plane daemon with cargo"
        cargo build --release --bin dionysusd
    fi

    DIONYSUSD_BUILD_BIN="target/release/dionysusd"
    require_file "$DIONYSUSD_BUILD_BIN"
}

generate_token() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 32
        return
    fi

    if command -v od >/dev/null 2>&1 && [ -r /dev/urandom ]; then
        od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
        printf '\n'
        return
    fi

    printf '%s-%s\n' "$(date +%s)" "$$"
}

install_token() {
    token_file="$DESTDIR/etc/dionysus/pve.token"

    if [ -f "$token_file" ]; then
        chmod 600 "$token_file"
        $STATUS info "Reusing existing API token at $token_file"
        return
    fi

    token="$(generate_token)"
    printf '%s\n' "$token" > "$token_file"
    chmod 600 "$token_file"

    $STATUS success "Created API token at $token_file"
    if [ "$HOST_INSTALL" -eq 1 ]; then
        $STATUS info "Dionysus API token: $token"
    else
        $STATUS info "Staged API token: $token"
    fi
}

enable_host_services() {
    if [ "$HOST_INSTALL" -ne 1 ]; then
        return
    fi

    $STATUS info "Reloading systemd and enabling Dionysus services"
    systemctl daemon-reload
    systemctl enable --now dionysus-network.service
    systemctl enable --now dionysus-llm-swap.service
    systemctl enable --now dionysus-metricsd.service
    systemctl enable --now dionysus-pvedaemon.service
    systemctl enable --now dionysus-pveproxy.service
}

require_file "Cargo.toml"
require_file "src/bin/dionysusd.rs"
require_file "$PVE_DIR/bin/dionysus-metricsd"
require_file "$PVE_DIR/bin/dionysus-pvedaemon"
require_file "$PVE_DIR/bin/dionysus-pveproxy"
require_file "$PVE_DIR/www/index.html"
require_file "$SYSTEMD_DIR/dionysus-pve.env"
require_file "$SYSTEMD_DIR/dionysus-network"
require_file "$SYSTEMD_DIR/dionysus-network.env"
require_file "$SYSTEMD_DIR/dionysus-network.service"
require_file "$SYSTEMD_DIR/dionysus-metricsd.service"
require_file "$SYSTEMD_DIR/dionysus-pvedaemon.service"
require_file "$SYSTEMD_DIR/dionysus-pveproxy.service"
require_file "$SYSTEMD_DIR/dionysus-llm-swap"
require_file "$SYSTEMD_DIR/dionysus-llm-swap.service"
require_file "$SYSTEMD_DIR/dionysus-llm-swap.env"

check_dependencies
install_missing_packages
if [ "$INSTALL_DEPS" = "1" ]; then
    check_dependencies
fi
build_dionysusd

target_label="$DESTDIR"
if [ -z "$target_label" ]; then
    target_label="/"
fi

$STATUS info "Installing Proxmox-style Dionysus control plane into $target_label"
mkdir -p \
    "$DESTDIR/etc/dionysus" \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants" \
    "$DESTDIR/etc/systemd/system/network-online.target.wants" \
    "$DESTDIR/usr/lib/dionysus" \
    "$DESTDIR/usr/sbin" \
    "$DESTDIR/usr/share/dionysus/profiles" \
    "$DESTDIR/usr/share/dionysus-pve-manager/www" \
    "$DESTDIR/var/lib/dionysus/metrics"

cp "$DIONYSUSD_BUILD_BIN" "$DESTDIR/usr/sbin/dionysusd"
cp "$PVE_DIR/bin/dionysus-metricsd" "$DESTDIR/usr/sbin/dionysus-metricsd"
cp "$PVE_DIR/bin/dionysus-pvedaemon" "$DESTDIR/usr/sbin/dionysus-pvedaemon"
cp "$PVE_DIR/bin/dionysus-pveproxy" "$DESTDIR/usr/sbin/dionysus-pveproxy"
chmod 755 "$DESTDIR/usr/sbin/dionysusd" \
    "$DESTDIR/usr/sbin/dionysus-metricsd" \
    "$DESTDIR/usr/sbin/dionysus-pvedaemon" \
    "$DESTDIR/usr/sbin/dionysus-pveproxy"

cp "$PVE_DIR/www/index.html" "$DESTDIR/usr/share/dionysus-pve-manager/www/index.html"
chmod 644 "$DESTDIR/usr/share/dionysus-pve-manager/www/index.html"

cp "$SYSTEMD_DIR/dionysus-pve.env" "$DESTDIR/etc/dionysus/pve.env"
chmod 644 "$DESTDIR/etc/dionysus/pve.env"

install_token

cp "$SYSTEMD_DIR/dionysus-network" "$DESTDIR/usr/lib/dionysus/dionysus-network"
cp "$SYSTEMD_DIR/dionysus-network.env" "$DESTDIR/etc/dionysus/network.env"
cp "$SYSTEMD_DIR/dionysus-network.service" "$DESTDIR/etc/systemd/system/dionysus-network.service"
chmod 755 "$DESTDIR/usr/lib/dionysus/dionysus-network"
chmod 600 "$DESTDIR/etc/dionysus/network.env"
chmod 644 "$DESTDIR/etc/systemd/system/dionysus-network.service"

cp "$SYSTEMD_DIR/dionysus-metricsd.service" "$DESTDIR/etc/systemd/system/dionysus-metricsd.service"
cp "$SYSTEMD_DIR/dionysus-pvedaemon.service" "$DESTDIR/etc/systemd/system/dionysus-pvedaemon.service"
cp "$SYSTEMD_DIR/dionysus-pveproxy.service" "$DESTDIR/etc/systemd/system/dionysus-pveproxy.service"
chmod 644 "$DESTDIR/etc/systemd/system/dionysus-metricsd.service" \
    "$DESTDIR/etc/systemd/system/dionysus-pvedaemon.service" \
    "$DESTDIR/etc/systemd/system/dionysus-pveproxy.service"

cp "$SYSTEMD_DIR/dionysus-llm-swap" "$DESTDIR/usr/lib/dionysus/dionysus-llm-swap"
cp "$SYSTEMD_DIR/dionysus-llm-swap.service" "$DESTDIR/etc/systemd/system/dionysus-llm-swap.service"
cp "$SYSTEMD_DIR/dionysus-llm-swap.env" "$DESTDIR/etc/dionysus/llm-swap.env"
chmod 755 "$DESTDIR/usr/lib/dionysus/dionysus-llm-swap"
chmod 644 "$DESTDIR/etc/systemd/system/dionysus-llm-swap.service" "$DESTDIR/etc/dionysus/llm-swap.env"

if [ -d "$DIONYSUS_PROFILES_DIR" ]; then
    find "$DIONYSUS_PROFILES_DIR" -maxdepth 1 -type f \( -name '*.yaml' -o -name '*.yml' \) \
        -exec cp {} "$DESTDIR/usr/share/dionysus/profiles/" \;
else
    $STATUS warn "Profiles directory not found: $DIONYSUS_PROFILES_DIR"
fi

ln -sf ../dionysus-llm-swap.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-llm-swap.service"
ln -sf ../dionysus-network.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-network.service"
ln -sf ../dionysus-network.service \
    "$DESTDIR/etc/systemd/system/network-online.target.wants/dionysus-network.service"
ln -sf ../dionysus-metricsd.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-metricsd.service"
ln -sf ../dionysus-pvedaemon.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-pvedaemon.service"
ln -sf ../dionysus-pveproxy.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-pveproxy.service"

enable_host_services

$STATUS success "Installed Dionysus PVE control plane into $target_label"
$STATUS info "pveproxy listens on port 8006 and exposes token-protected /api2/json endpoints."
