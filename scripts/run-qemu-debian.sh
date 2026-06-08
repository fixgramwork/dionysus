#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
QEMU_BIN="${QEMU_BIN:-qemu-system-aarch64}"
DEBIAN_ARCH="${DEBIAN_ARCH:-arm64}"
DEBIAN_ROOTFS_IMAGE="${1:-${DEBIAN_ROOTFS_IMAGE:-build/debian-$DEBIAN_ARCH/dionysus-debian-$DEBIAN_ARCH.ext4}}"
DEBIAN_KERNEL_IMAGE="${DEBIAN_KERNEL_IMAGE:-build/debian-$DEBIAN_ARCH/vmlinuz}"
DEBIAN_INITRD_IMAGE="${DEBIAN_INITRD_IMAGE:-build/debian-$DEBIAN_ARCH/initrd.img}"
QEMU_MEMORY="${QEMU_MEMORY:-2048M}"
QEMU_CPUS="${QEMU_CPUS:-2}"
QEMU_WEB_PORT="${QEMU_WEB_PORT:-18106}"
QEMU_SSH_PORT="${QEMU_SSH_PORT:-10022}"
QEMU_PID_FILE="${QEMU_PID_FILE:-build/qemu-debian-$DEBIAN_ARCH.pid}"
QEMU_LOG_FILE="${QEMU_LOG_FILE:-build/qemu-debian-$DEBIAN_ARCH-console.log}"
QEMU_MONITOR="${QEMU_MONITOR:-build/qemu-debian-$DEBIAN_ARCH.monitor}"
QEMU_APPEND="${QEMU_APPEND:-root=/dev/vda rw console=ttyAMA0 net.ifnames=0 biosdevname=0}"

require_file() {
    path="$1"
    if [ ! -f "$path" ]; then
        $STATUS error "Missing required file: $path"
        exit 1
    fi
}

require_tool() {
    tool="$1"
    if ! command -v "$tool" >/dev/null 2>&1; then
        $STATUS error "Missing required tool: $tool"
        exit 1
    fi
}

if [ -f "$QEMU_PID_FILE" ]; then
    old_pid="$(cat "$QEMU_PID_FILE" 2>/dev/null || true)"
    if [ -n "$old_pid" ] && kill -0 "$old_pid" 2>/dev/null; then
        $STATUS error "QEMU already appears to be running with pid $old_pid"
        $STATUS error "Stop it first: kill $old_pid"
        exit 1
    fi
fi

require_tool "$QEMU_BIN"
require_file "$DEBIAN_ROOTFS_IMAGE"
require_file "$DEBIAN_KERNEL_IMAGE"
require_file "$DEBIAN_INITRD_IMAGE"

mkdir -p "$(dirname "$QEMU_PID_FILE")" "$(dirname "$QEMU_LOG_FILE")" "$(dirname "$QEMU_MONITOR")"
rm -f "$QEMU_MONITOR" "$QEMU_PID_FILE"

$STATUS info "Starting Debian-based Dionysus OS at http://127.0.0.1:$QEMU_WEB_PORT"
"$QEMU_BIN" \
    -M virt \
    -cpu cortex-a57 \
    -smp "$QEMU_CPUS" \
    -m "$QEMU_MEMORY" \
    -kernel "$DEBIAN_KERNEL_IMAGE" \
    -initrd "$DEBIAN_INITRD_IMAGE" \
    -append "$QEMU_APPEND" \
    -drive if=none,file="$DEBIAN_ROOTFS_IMAGE",format=raw,id=rootfs \
    -device virtio-blk-device,drive=rootfs \
    -netdev user,id=net0,hostfwd=tcp:127.0.0.1:"$QEMU_WEB_PORT"-:8006,hostfwd=tcp:127.0.0.1:"$QEMU_SSH_PORT"-:22 \
    -device virtio-net-device,netdev=net0 \
    -device virtio-rng-device \
    -display none \
    -serial file:"$QEMU_LOG_FILE" \
    -monitor unix:"$QEMU_MONITOR",server,nowait \
    -pidfile "$QEMU_PID_FILE" \
    -daemonize \
    -no-reboot

$STATUS success "Started QEMU pid $(cat "$QEMU_PID_FILE")"
$STATUS info "Web console: http://127.0.0.1:$QEMU_WEB_PORT"
$STATUS info "SSH forward: ssh root@127.0.0.1 -p $QEMU_SSH_PORT"
$STATUS info "Serial log: $QEMU_LOG_FILE"
