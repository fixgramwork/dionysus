#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
ROOTFS_OVERLAY="${1:-${ROOTFS_OVERLAY:-initramfs/overlay}}"
OUTPUT_FILE="${2:-${OUTPUT_FILE:-build/initramfs/dionysus-initramfs.cpio.gz}}"
STAGING_DIR="${3:-${STAGING_DIR:-build/initramfs/rootfs}}"
LAYOUT_ONLY="${LAYOUT_ONLY:-0}"
REQUIRE_BUSYBOX="${REQUIRE_BUSYBOX:-1}"
BUSYBOX_BIN="${BUSYBOX_BIN:-}"
DIONYSUS_PROFILES_DIR="${DIONYSUS_PROFILES_DIR:-profiles}"
HOST_OS="$(uname -s)"
LINUX_BUILDER_RUNNER="${LINUX_BUILDER_RUNNER:-sh scripts/run-linux-builder.sh}"
TEMP_BUSYBOX_DIR=""

cleanup() {
    if [ -n "$TEMP_BUSYBOX_DIR" ] && [ -d "$TEMP_BUSYBOX_DIR" ]; then
        rm -rf "$TEMP_BUSYBOX_DIR"
    fi
}

trap cleanup EXIT

if ! command -v cpio >/dev/null 2>&1; then
    $STATUS error "Missing required tool: cpio"
    exit 1
fi

if ! command -v gzip >/dev/null 2>&1; then
    $STATUS error "Missing required tool: gzip"
    exit 1
fi

if [ -z "$BUSYBOX_BIN" ] && command -v busybox >/dev/null 2>&1; then
    BUSYBOX_BIN="$(command -v busybox)"
fi

if [ -z "$BUSYBOX_BIN" ] && [ "$HOST_OS" != "Linux" ]; then
    TEMP_BUSYBOX_DIR="$(mktemp -d)"
    BUSYBOX_BIN="$TEMP_BUSYBOX_DIR/busybox"
    if $LINUX_BUILDER_RUNNER fetch-busybox "$BUSYBOX_BIN"; then
        $STATUS info "Using busybox-static from the Linux builder image"
    else
        BUSYBOX_BIN=""
        cleanup
        TEMP_BUSYBOX_DIR=""
    fi
fi

if [ ! -d "$ROOTFS_OVERLAY" ]; then
    $STATUS error "Initramfs overlay not found: $ROOTFS_OVERLAY"
    exit 1
fi

$STATUS info "Preparing initramfs staging tree in $STAGING_DIR"
rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"
cp -R "$ROOTFS_OVERLAY"/. "$STAGING_DIR"
mkdir -p "$STAGING_DIR/bin" "$STAGING_DIR/dev" "$STAGING_DIR/etc" \
    "$STAGING_DIR/proc" "$STAGING_DIR/run" "$STAGING_DIR/sys" \
    "$STAGING_DIR/tmp" "$STAGING_DIR/usr/bin" "$STAGING_DIR/usr/sbin" \
    "$STAGING_DIR/usr/share/dionysus/profiles" "$STAGING_DIR/usr/share/udhcpc" \
    "$STAGING_DIR/var"

if [ -f "$STAGING_DIR/init" ]; then
    chmod 755 "$STAGING_DIR/init"
fi

if [ -f "$STAGING_DIR/usr/bin/dionysus-agent" ]; then
    chmod 755 "$STAGING_DIR/usr/bin/dionysus-agent"
fi

if [ -f "$STAGING_DIR/usr/bin/dionysus-services" ]; then
    chmod 755 "$STAGING_DIR/usr/bin/dionysus-services"
fi

if [ -f "$STAGING_DIR/usr/bin/dionysus-network" ]; then
    chmod 755 "$STAGING_DIR/usr/bin/dionysus-network"
fi

if [ -f "$STAGING_DIR/usr/share/udhcpc/default.script" ]; then
    chmod 755 "$STAGING_DIR/usr/share/udhcpc/default.script"
fi

if [ -d "$DIONYSUS_PROFILES_DIR" ]; then
    $STATUS info "Copying workload profiles from $DIONYSUS_PROFILES_DIR"
    find "$DIONYSUS_PROFILES_DIR" -maxdepth 1 -type f \( -name '*.yaml' -o -name '*.yml' \) -exec cp {} "$STAGING_DIR/usr/share/dionysus/profiles/" \;
else
    $STATUS warn "Profiles directory not found at $DIONYSUS_PROFILES_DIR"
fi

if [ -n "$BUSYBOX_BIN" ] && [ ! -f "$BUSYBOX_BIN" ]; then
    if [ "$REQUIRE_BUSYBOX" = "1" ]; then
        $STATUS error "BusyBox binary not found: $BUSYBOX_BIN"
        exit 1
    fi

    $STATUS warn "BusyBox binary not found at $BUSYBOX_BIN; prepared layout will not include a bootable shell"
    BUSYBOX_BIN=""
fi

if [ -n "$BUSYBOX_BIN" ]; then
    $STATUS info "Installing BusyBox from $BUSYBOX_BIN"
    cp "$BUSYBOX_BIN" "$STAGING_DIR/bin/busybox"
    chmod 755 "$STAGING_DIR/bin/busybox"

    for applet in sh mount umount mkdir cat uname echo sleep poweroff reboot ip ifconfig route udhcpc hostname dmesg ps ls; do
        ln -sf busybox "$STAGING_DIR/bin/$applet"
    done
elif [ "$REQUIRE_BUSYBOX" = "1" ]; then
    $STATUS error "BusyBox binary is required to build a bootable initramfs"
    $STATUS error "Set BUSYBOX_BIN=/path/to/busybox or install busybox on the host"
    exit 1
else
    $STATUS warn "BusyBox not found; prepared layout without a bootable shell"
fi

if [ "$LAYOUT_ONLY" = "1" ]; then
    $STATUS success "Prepared initramfs layout in $STAGING_DIR"
    exit 0
fi

mkdir -p "$(dirname "$OUTPUT_FILE")"
OUTPUT_DIR="$(cd "$(dirname "$OUTPUT_FILE")" && pwd)"
OUTPUT_PATH="$OUTPUT_DIR/$(basename "$OUTPUT_FILE")"

$STATUS info "Packing initramfs archive -> $OUTPUT_PATH"
(
    cd "$STAGING_DIR"
    find . -print | LC_ALL=C sort | cpio -o -H newc | gzip -9 > "$OUTPUT_PATH"
)

$STATUS success "Created initramfs archive $OUTPUT_PATH"
