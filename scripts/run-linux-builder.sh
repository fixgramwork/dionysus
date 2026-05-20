#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
ACTION="${1:-}"

if [ -z "$ACTION" ]; then
    $STATUS error "Usage: sh scripts/run-linux-builder.sh <build-kernel|fetch-busybox> [...]"
    exit 1
fi

shift

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
DOCKERFILE_PATH="$SCRIPT_DIR/docker/linux-builder.Dockerfile"
IMAGE_NAME="${DIONYSUS_LINUX_BUILDER_IMAGE:-dionysus/linux-builder:bookworm-amd64}"
TARGET_PLATFORM="${DIONYSUS_LINUX_BUILDER_PLATFORM:-linux/amd64}"
CONTAINER_ENGINE="${CONTAINER_ENGINE:-}"

abspath() {
    case "$1" in
        /*) printf '%s\n' "$1" ;;
        *) printf '%s/%s\n' "$(pwd)" "$1" ;;
    esac
}

detect_engine() {
    if [ -n "$CONTAINER_ENGINE" ]; then
        if command -v "$CONTAINER_ENGINE" >/dev/null 2>&1 && "$CONTAINER_ENGINE" info >/dev/null 2>&1; then
            printf '%s\n' "$CONTAINER_ENGINE"
            return 0
        fi
        $STATUS error "Requested container engine is unavailable: $CONTAINER_ENGINE"
        exit 1
    fi

    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        printf '%s\n' docker
        return 0
    fi

    if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
        printf '%s\n' podman
        return 0
    fi

    $STATUS error "No running container engine found for Linux builder fallback"
    $STATUS error "Start Docker Desktop or Podman, or rerun on a Linux host."
    exit 1
}

build_image() {
    engine="$1"

    if "$engine" image inspect "$IMAGE_NAME" >/dev/null 2>&1; then
        return 0
    fi

    $STATUS info "Building Linux builder image $IMAGE_NAME"
    "$engine" build --platform "$TARGET_PLATFORM" -t "$IMAGE_NAME" -f "$DOCKERFILE_PATH" "$SCRIPT_DIR/docker"
}

run_build_kernel() {
    engine="$1"
    shift

    linux_dir="$1"
    build_dir="$2"
    fragment="$3"

    user_flag=""
    if [ "$engine" = "docker" ] && [ "$(uname -s)" = "Linux" ] && command -v id >/dev/null 2>&1; then
        user_flag="--user $(id -u):$(id -g)"
    fi

    $STATUS info "Running Linux kernel build inside $engine on $TARGET_PLATFORM"
    # shellcheck disable=SC2086
    exec "$engine" run --rm --platform "$TARGET_PLATFORM" $user_flag \
        -e ALLOW_UNVERIFIED_HOST=1 \
        -e ARCH="${ARCH:-x86_64}" \
        -e CONFIG_TARGET="${CONFIG_TARGET:-x86_64_defconfig}" \
        -e KERNEL_TARGET="${KERNEL_TARGET:-bzImage}" \
        -e KERNEL_TARGETS="${KERNEL_TARGETS:-${KERNEL_TARGET:-bzImage}}" \
        -e KERNEL_IMAGE_PATH="${KERNEL_IMAGE_PATH:-}" \
        -e LINUX_BUILD_JOBS="${LINUX_BUILD_JOBS:-}" \
        -e LLVM_MODE="${LLVM_MODE:-}" \
        -e MAKE_BIN="${MAKE_BIN:-make}" \
        -v "$REPO_ROOT":/workspace \
        -w /workspace \
        "$IMAGE_NAME" \
        sh scripts/build-linux-kernel.sh "$linux_dir" "$build_dir" "$fragment"
}

run_fetch_busybox() {
    engine="$1"
    shift

    target_path="$(abspath "$1")"
    target_dir="$(dirname "$target_path")"
    target_name="$(basename "$target_path")"

    mkdir -p "$target_dir"

    user_flag=""
    if [ "$engine" = "docker" ] && [ "$(uname -s)" = "Linux" ] && command -v id >/dev/null 2>&1; then
        user_flag="--user $(id -u):$(id -g)"
    fi

    $STATUS info "Extracting busybox-static into $target_path"
    # shellcheck disable=SC2086
    exec "$engine" run --rm --platform "$TARGET_PLATFORM" $user_flag \
        -v "$target_dir":/out \
        "$IMAGE_NAME" \
        sh -c "cp /bin/busybox /out/$target_name"
}

engine="$(detect_engine)"
build_image "$engine"

case "$ACTION" in
    build-kernel)
        if [ "$#" -ne 3 ]; then
            $STATUS error "build-kernel requires: <linux-dir> <build-dir> <fragment>"
            exit 1
        fi
        run_build_kernel "$engine" "$@"
        ;;
    fetch-busybox)
        if [ "$#" -ne 1 ]; then
            $STATUS error "fetch-busybox requires: <target-path>"
            exit 1
        fi
        run_fetch_busybox "$engine" "$1"
        ;;
    *)
        $STATUS error "Unsupported Linux builder action: $ACTION"
        exit 1
        ;;
esac
