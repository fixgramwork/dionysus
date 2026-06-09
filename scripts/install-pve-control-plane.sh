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
DIONYSUS_PVE_USERNAME="${DIONYSUS_PVE_USERNAME:-root}"
DIONYSUS_PVE_PASSWORD="${DIONYSUS_PVE_PASSWORD:-dionysus}"

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

target_has_command() {
    command_name="$1"

    if [ "$HOST_INSTALL" -eq 1 ] || [ -z "$DESTDIR" ]; then
        command -v "$command_name" >/dev/null 2>&1
        return
    fi

    for path_dir in bin sbin usr/bin usr/sbin; do
        if [ -x "$DESTDIR/$path_dir/$command_name" ]; then
            return 0
        fi
    done

    return 1
}

check_dependencies() {
    MISSING_PACKAGES=""

    if [ -z "$DIONYSUSD_BIN" ] && [ ! -x build/dionysusd ]; then
        command -v go >/dev/null 2>&1 || add_package golang-go
    fi
    target_has_command systemctl || add_package systemd
    target_has_command sqlite3 || add_package sqlite3
    target_has_command swapon || add_package util-linux
    target_has_command mkswap || add_package util-linux
    target_has_command ip || add_package iproute2
    target_has_command wpa_supplicant || add_package wpasupplicant
    if ! target_has_command udhcpc && ! target_has_command dhclient; then
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

    if [ ! -x build/dionysusd ]; then
        command -v go >/dev/null 2>&1 || {
            $STATUS error "Missing required tool: go"
            exit 1
        }
        $STATUS info "Building Go control-plane daemon"
        mkdir -p build/gocache build/gomodcache
        GOCACHE="${GOCACHE:-$(pwd)/build/gocache}" GOMODCACHE="${GOMODCACHE:-$(pwd)/build/gomodcache}" go build -o build/dionysusd ./cmd/dionysusd
    fi

    DIONYSUSD_BUILD_BIN="build/dionysusd"
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

hash_password() {
    password="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        printf '%s' "$password" | sha256sum | awk '{print "sha256:" $1}'
        return
    fi
    if command -v shasum >/dev/null 2>&1; then
        printf '%s' "$password" | shasum -a 256 | awk '{print "sha256:" $1}'
        return
    fi
    printf '%s\n' "$password"
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

install_auth_credentials() {
    users_file="$DESTDIR/etc/dionysus/pve.users.json"
    user_file="$DESTDIR/etc/dionysus/pve.user"
    password_file="$DESTDIR/etc/dionysus/pve.password"
    created_at="$(date +%s)"

    if [ ! -f "$user_file" ]; then
        printf '%s\n' "$DIONYSUS_PVE_USERNAME" > "$user_file"
        $STATUS success "Created web console username file at $user_file"
    else
        $STATUS info "Reusing existing web console username file at $user_file"
    fi
    chmod 600 "$user_file"

    if [ ! -f "$password_file" ]; then
        printf '%s\n' "$DIONYSUS_PVE_PASSWORD" > "$password_file"
        $STATUS success "Created web console password file at $password_file"
    else
        $STATUS info "Reusing existing web console password file at $password_file"
    fi
    chmod 600 "$password_file"

    if [ ! -f "$users_file" ]; then
        password_hash="$(hash_password "$DIONYSUS_PVE_PASSWORD")"
        cat > "$users_file" <<EOF
{
  "users": [
    {
      "username": "$DIONYSUS_PVE_USERNAME",
      "passwordHash": "$password_hash",
      "permissions": [
        "node.read",
        "network.manage",
        "packages.manage",
        "llm.manage",
        "services.manage",
        "console.run"
      ],
      "createdAt": $created_at,
      "updatedAt": $created_at
    }
  ]
}
EOF
        $STATUS success "Created web console users file at $users_file"
    else
        $STATUS info "Reusing existing web console users file at $users_file"
    fi
    chmod 600 "$users_file"

    if [ "$HOST_INSTALL" -eq 1 ]; then
        $STATUS info "Dionysus web console username: $DIONYSUS_PVE_USERNAME"
        $STATUS info "Dionysus web console password: stored in $password_file"
    else
        $STATUS info "Staged web console login: $DIONYSUS_PVE_USERNAME / $DIONYSUS_PVE_PASSWORD"
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

require_file "go.mod"
require_file "cmd/dionysusd/main.go"
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

cp -R "$PVE_DIR/www/." "$DESTDIR/usr/share/dionysus-pve-manager/www/"
find "$DESTDIR/usr/share/dionysus-pve-manager/www" -type d -exec chmod 755 {} \;
find "$DESTDIR/usr/share/dionysus-pve-manager/www" -type f -exec chmod 644 {} \;

cp "$SYSTEMD_DIR/dionysus-pve.env" "$DESTDIR/etc/dionysus/pve.env"
chmod 644 "$DESTDIR/etc/dionysus/pve.env"

install_token
install_auth_credentials

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
$STATUS info "pveproxy listens on port 8006 and exposes JWT-protected /api2/json endpoints."
