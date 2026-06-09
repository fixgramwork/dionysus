#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
BASE_ROOTFS="${BASE_ROOTFS:-build/qemu-arm64/initramfs/rootfs}"
STAGING_DIR="${1:-${STAGING_DIR:-build/qemu-web-initramfs/rootfs}}"
OUTPUT_FILE="${2:-${OUTPUT_FILE:-build/qemu-web-initramfs/dionysus-web-initramfs.cpio.gz}}"
DIONYSUSD_BIN="${DIONYSUSD_BIN:-build/dionysusd-linux-arm64}"
WWW_ROOT="${WWW_ROOT:-pve/www}"
DIONYSUS_INCLUDE_OLLAMA="${DIONYSUS_INCLUDE_OLLAMA:-1}"
OLLAMA_ARCH="${OLLAMA_ARCH:-arm64}"
OLLAMA_ROOT="${OLLAMA_ROOT:-build/ollama/linux-$OLLAMA_ARCH/rootfs}"
OLLAMA_FETCH="${OLLAMA_FETCH:-sh scripts/fetch-ollama-linux.sh}"
DIONYSUS_PVE_USERNAME="${DIONYSUS_PVE_USERNAME:-root}"
DIONYSUS_PVE_PASSWORD="${DIONYSUS_PVE_PASSWORD:-dionysus}"
case "$OLLAMA_ARCH" in
    amd64) OLLAMA_LOADER="$OLLAMA_ROOT/lib64/ld-linux-x86-64.so.2" ;;
    arm64) OLLAMA_LOADER="$OLLAMA_ROOT/lib/ld-linux-aarch64.so.1" ;;
    *) OLLAMA_LOADER="$OLLAMA_ROOT/lib/ld-linux-aarch64.so.1" ;;
esac

require_file() {
    path="$1"
    if [ ! -f "$path" ]; then
        $STATUS error "Missing required file: $path"
        exit 1
    fi
}

require_dir() {
    path="$1"
    if [ ! -d "$path" ]; then
        $STATUS error "Missing required directory: $path"
        exit 1
    fi
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

require_dir "$BASE_ROOTFS"
require_file "$DIONYSUSD_BIN"
require_file "$WWW_ROOT/index.html"

if [ "$DIONYSUS_INCLUDE_OLLAMA" = "1" ]; then
    if [ ! -x "$OLLAMA_ROOT/usr/bin/ollama" ] || [ ! -e "$OLLAMA_LOADER" ]; then
        $STATUS info "Ollama is enabled for this image; preparing $OLLAMA_ROOT"
        OLLAMA_ARCH="$OLLAMA_ARCH" OLLAMA_ROOT="$OLLAMA_ROOT" $OLLAMA_FETCH
    fi
    require_file "$OLLAMA_ROOT/usr/bin/ollama"
    require_file "$OLLAMA_LOADER"
fi

$STATUS info "Preparing QEMU web-console initramfs in $STAGING_DIR"
rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"
cp -R "$BASE_ROOTFS"/. "$STAGING_DIR"

if [ "$DIONYSUS_INCLUDE_OLLAMA" = "1" ]; then
    $STATUS info "Including Ollama in QEMU web-console initramfs"
    cp -R "$OLLAMA_ROOT"/. "$STAGING_DIR"/
fi

mkdir -p \
    "$STAGING_DIR/etc/dionysus" \
    "$STAGING_DIR/run/dionysus" \
    "$STAGING_DIR/usr/sbin" \
    "$STAGING_DIR/usr/share/dionysus-pve-manager/www" \
    "$STAGING_DIR/var/lib/dionysus/metrics"

cp "$DIONYSUSD_BIN" "$STAGING_DIR/usr/sbin/dionysusd"
chmod 755 "$STAGING_DIR/usr/sbin/dionysusd"

cp -R "$WWW_ROOT"/. "$STAGING_DIR/usr/share/dionysus-pve-manager/www/"
find "$STAGING_DIR/usr/share/dionysus-pve-manager/www" -type d -exec chmod 755 {} \;
find "$STAGING_DIR/usr/share/dionysus-pve-manager/www" -type f -exec chmod 644 {} \;

cat > "$STAGING_DIR/etc/dionysus/pve.env" <<'ENV'
DIONYSUS_PVE_PROXY_LISTEN=0.0.0.0:8006
DIONYSUS_PROC_ROOT=/proc
DIONYSUS_SYS_ROOT=/sys
DIONYSUS_ETC_ROOT=/etc
DIONYSUS_OLLAMA_API=http://127.0.0.1:11434
DIONYSUS_PVE_WWW=/usr/share/dionysus-pve-manager/www
DIONYSUS_PVE_TOKEN_FILE=/etc/dionysus/pve.token
DIONYSUS_PVE_USERS_FILE=/etc/dionysus/pve.users.json
DIONYSUS_PVE_USER_FILE=/etc/dionysus/pve.user
DIONYSUS_PVE_PASSWORD_FILE=/etc/dionysus/pve.password
DIONYSUS_PVE_USERNAME=root
DIONYSUS_JWT_TTL_SECONDS=43200
DIONYSUS_NETWORK_CONFIG=/etc/dionysus/network.env
DIONYSUS_METRICS_DB=/var/lib/dionysus/metrics/ollama.sqlite3
ENV

generate_token > "$STAGING_DIR/etc/dionysus/pve.token"
printf '%s\n' "$DIONYSUS_PVE_USERNAME" > "$STAGING_DIR/etc/dionysus/pve.user"
printf '%s\n' "$DIONYSUS_PVE_PASSWORD" > "$STAGING_DIR/etc/dionysus/pve.password"
created_at="$(date +%s)"
password_hash="$(hash_password "$DIONYSUS_PVE_PASSWORD")"
cat > "$STAGING_DIR/etc/dionysus/pve.users.json" <<EOF
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
chmod 600 "$STAGING_DIR/etc/dionysus/pve.token" \
    "$STAGING_DIR/etc/dionysus/pve.users.json" \
    "$STAGING_DIR/etc/dionysus/pve.user" \
    "$STAGING_DIR/etc/dionysus/pve.password"

cat > "$STAGING_DIR/etc/dionysus/network.env" <<'ENV'
DIONYSUS_NETWORK_ENABLED='1'
DIONYSUS_NETWORK_MODE='lan'
DIONYSUS_LAN_ENABLED='1'
DIONYSUS_LAN_IFACE='eth0'
DIONYSUS_LAN_IPV4_METHOD='dhcp'
ENV
chmod 600 "$STAGING_DIR/etc/dionysus/network.env"

cat > "$STAGING_DIR/init" <<'INIT'
#!/bin/sh

set -eu

export PATH=/bin:/sbin:/usr/bin:/usr/sbin

mkdir -p /proc /sys /dev /run /tmp /var/lib/dionysus/metrics
mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev 2>/dev/null || true

printf '[dionysus-init] Linux base bootstrapped\n'

if [ -r /etc/dionysus-release ]; then
    printf '[dionysus-init] release: '
    cat /etc/dionysus-release
fi

if [ -x /usr/bin/dionysus-agent ]; then
    /usr/bin/dionysus-agent bootstrap
fi

if [ -x /usr/bin/dionysus-network ]; then
    DIONYSUS_NET_IFACE=eth0 /usr/bin/dionysus-network start
fi

if [ -x /usr/bin/ollama ]; then
    mkdir -p /var/lib/ollama/models /run/dionysus
    export HOME="${HOME:-/var/lib/ollama}"
    export LD_LIBRARY_PATH="${LD_LIBRARY_PATH:-/lib/aarch64-linux-gnu:/usr/lib/aarch64-linux-gnu:/usr/lib/ollama}"
    export OLLAMA_HOST="${OLLAMA_HOST:-127.0.0.1:11434}"
    export OLLAMA_MODELS="${OLLAMA_MODELS:-/var/lib/ollama/models}"
    printf '[dionysus-init] starting Ollama on %s\n' "$OLLAMA_HOST"
    /usr/bin/ollama serve > /run/dionysus/ollama.log 2>&1 &
else
    printf '[dionysus-init] Ollama binary not included\n'
fi

printf '[dionysus-init] starting web console on 0.0.0.0:8006\n'
/usr/sbin/dionysusd proxy \
    --dev-allow-host \
    --listen 0.0.0.0:8006 \
    --proc-root /proc \
    --sys-root /sys \
    --etc-root /etc \
    --www-root /usr/share/dionysus-pve-manager/www \
    --network-config /etc/dionysus/network.env \
    --metrics-db /var/lib/dionysus/metrics/ollama.sqlite3 \
    --token-file /etc/dionysus/pve.token &

printf '[dionysus-init] web console ready; rescue shell remains available\n'
exec /bin/sh
INIT
chmod 755 "$STAGING_DIR/init"

mkdir -p "$(dirname "$OUTPUT_FILE")"
OUTPUT_DIR="$(cd "$(dirname "$OUTPUT_FILE")" && pwd)"
OUTPUT_PATH="$OUTPUT_DIR/$(basename "$OUTPUT_FILE")"

$STATUS info "Packing QEMU web-console initramfs -> $OUTPUT_PATH"
(
    cd "$STAGING_DIR"
    find . -print | LC_ALL=C sort | cpio -o -H newc | gzip -9 > "$OUTPUT_PATH"
)

$STATUS success "Created QEMU web-console initramfs $OUTPUT_PATH"
