#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
LINUX_DIR="${1:-${LINUX_DIR:-upstream/linux}}"
BUILD_DIR="${2:-${LINUX_BUILD_DIR:-build/linux}}"
FRAGMENT="${3:-${LINUX_FRAGMENT:-config/linux/x86_64-dionysus.fragment}}"
ARCH="${ARCH:-x86_64}"
CONFIG_TARGET="${CONFIG_TARGET:-x86_64_defconfig}"
KERNEL_TARGET="${KERNEL_TARGET:-bzImage}"
KERNEL_TARGETS="${KERNEL_TARGETS:-$KERNEL_TARGET}"
KERNEL_IMAGE_PATH="${KERNEL_IMAGE_PATH:-}"
MAKE_BIN="${MAKE_BIN:-make}"
HOST_OS="$(uname -s)"
ALLOW_UNVERIFIED_HOST="${ALLOW_UNVERIFIED_HOST:-0}"
LLVM_MODE="${LLVM_MODE:-}"
LINUX_BUILD_JOBS="${LINUX_BUILD_JOBS:-}"
LINUX_BUILDER_RUNNER="${LINUX_BUILDER_RUNNER:-sh scripts/run-linux-builder.sh}"

abspath() {
    case "$1" in
        /*) printf '%s\n' "$1" ;;
        *) printf '%s/%s\n' "$(pwd)" "$1" ;;
    esac
}

if ! command -v "$MAKE_BIN" >/dev/null 2>&1; then
    $STATUS error "Missing required tool: $MAKE_BIN"
    exit 1
fi

if [ ! -d "$LINUX_DIR/.git" ]; then
    $STATUS error "Linux checkout not found: $LINUX_DIR"
    $STATUS error "Run 'make linux-fetch LINUX_REF=<ref>' first."
    exit 1
fi

if [ ! -f "$FRAGMENT" ]; then
    $STATUS error "Linux config fragment not found: $FRAGMENT"
    exit 1
fi

if [ ! -f "$LINUX_DIR/scripts/kconfig/merge_config.sh" ]; then
    $STATUS error "Linux helper script missing: $LINUX_DIR/scripts/kconfig/merge_config.sh"
    $STATUS error "Ensure the Linux checkout is complete and checked out."
    exit 1
fi

if [ "$HOST_OS" != "Linux" ] && [ "$ALLOW_UNVERIFIED_HOST" != "1" ]; then
    $STATUS warn "Non-Linux host detected: $HOST_OS"
    $STATUS warn "Falling back to the containerized Linux builder."
    exec $LINUX_BUILDER_RUNNER build-kernel "$LINUX_DIR" "$BUILD_DIR" "$FRAGMENT"
fi

LINUX_DIR_ABS="$(abspath "$LINUX_DIR")"
BUILD_DIR_ABS="$(abspath "$BUILD_DIR")"
FRAGMENT_ABS="$(abspath "$FRAGMENT")"
MERGE_CONFIG="$LINUX_DIR_ABS/scripts/kconfig/merge_config.sh"

mkdir -p "$BUILD_DIR_ABS"

if [ -n "$LINUX_BUILD_JOBS" ]; then
    jobs="$LINUX_BUILD_JOBS"
else
    jobs="$(
        getconf _NPROCESSORS_ONLN 2>/dev/null || \
        nproc 2>/dev/null || \
        sysctl -n hw.ncpu 2>/dev/null || \
        printf '%s\n' 4
    )"
fi

if [ -n "$LLVM_MODE" ]; then
    EXTRA_ARGS="LLVM=$LLVM_MODE"
else
    EXTRA_ARGS=""
fi

$STATUS info "Configuring Linux kernel with $CONFIG_TARGET"
if [ -n "$EXTRA_ARGS" ]; then
    $MAKE_BIN -C "$LINUX_DIR_ABS" O="$BUILD_DIR_ABS" ARCH="$ARCH" $EXTRA_ARGS "$CONFIG_TARGET"
else
    $MAKE_BIN -C "$LINUX_DIR_ABS" O="$BUILD_DIR_ABS" ARCH="$ARCH" "$CONFIG_TARGET"
fi

$STATUS info "Merging config fragment $FRAGMENT_ABS"
sh "$MERGE_CONFIG" -m -O "$BUILD_DIR_ABS" "$BUILD_DIR_ABS/.config" "$FRAGMENT_ABS"

$STATUS info "Refreshing merged configuration"
if [ -n "$EXTRA_ARGS" ]; then
    $MAKE_BIN -C "$LINUX_DIR_ABS" O="$BUILD_DIR_ABS" ARCH="$ARCH" $EXTRA_ARGS olddefconfig
else
    $MAKE_BIN -C "$LINUX_DIR_ABS" O="$BUILD_DIR_ABS" ARCH="$ARCH" olddefconfig
fi

$STATUS info "Building Linux targets $KERNEL_TARGETS with $jobs jobs"
if [ -n "$EXTRA_ARGS" ]; then
    # shellcheck disable=SC2086
    $MAKE_BIN -C "$LINUX_DIR_ABS" O="$BUILD_DIR_ABS" ARCH="$ARCH" $EXTRA_ARGS -j"$jobs" $KERNEL_TARGETS
else
    # shellcheck disable=SC2086
    $MAKE_BIN -C "$LINUX_DIR_ABS" O="$BUILD_DIR_ABS" ARCH="$ARCH" -j"$jobs" $KERNEL_TARGETS
fi

if [ -n "$KERNEL_IMAGE_PATH" ]; then
    IMAGE_PATH="$KERNEL_IMAGE_PATH"
else
    case "$ARCH" in
        arm64)
            IMAGE_PATH="$BUILD_DIR_ABS/arch/arm64/boot/Image"
            ;;
        arm)
            IMAGE_PATH="$BUILD_DIR_ABS/arch/arm/boot/zImage"
            ;;
        *)
            IMAGE_PATH="$BUILD_DIR_ABS/arch/x86/boot/bzImage"
            ;;
    esac
fi

if [ ! -f "$IMAGE_PATH" ]; then
    $STATUS error "Expected Linux kernel image not found: $IMAGE_PATH"
    exit 1
fi

$STATUS success "Built Linux kernel image $IMAGE_PATH"
