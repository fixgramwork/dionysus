#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
BASE_ROOTFS="${BASE_ROOTFS:-build/qemu-arm64/initramfs/rootfs}"
STAGING_DIR="${1:-${STAGING_DIR:-build/qemu-web-initramfs/rootfs}}"
OUTPUT_FILE="${2:-${OUTPUT_FILE:-build/qemu-web-initramfs/dionysus-web-initramfs.cpio.gz}}"
DIONYSUSD_BIN="${DIONYSUSD_BIN:-build/dionysusd-linux-arm64}"
WWW_ROOT="${WWW_ROOT:-pve/www}"

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

require_dir "$BASE_ROOTFS"
require_file "$DIONYSUSD_BIN"
require_file "$WWW_ROOT/index.html"

$STATUS info "Preparing QEMU web-console initramfs in $STAGING_DIR"
rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"
cp -R "$BASE_ROOTFS"/. "$STAGING_DIR"

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
DIONYSUS_NETWORK_CONFIG=/etc/dionysus/network.env
DIONYSUS_METRICS_DB=/var/lib/dionysus/metrics/ollama.sqlite3
ENV

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
