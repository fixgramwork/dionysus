#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
OLLAMA_ARCH="${OLLAMA_ARCH:-arm64}"
OLLAMA_URL="${OLLAMA_URL:-https://ollama.com/download/ollama-linux-$OLLAMA_ARCH.tar.zst}"
OLLAMA_ARCHIVE="${OLLAMA_ARCHIVE:-build/ollama/ollama-linux-$OLLAMA_ARCH.tar.zst}"
OLLAMA_ROOT="${OLLAMA_ROOT:-build/ollama/linux-$OLLAMA_ARCH/rootfs}"
OLLAMA_DEBIAN_RUNTIME="${OLLAMA_DEBIAN_RUNTIME:-1}"
OLLAMA_DEBIAN_MIRROR="${OLLAMA_DEBIAN_MIRROR:-https://deb.debian.org/debian}"
OLLAMA_DEBIAN_SUITE="${OLLAMA_DEBIAN_SUITE:-bookworm}"
OLLAMA_DEBIAN_ARCH="${OLLAMA_DEBIAN_ARCH:-$OLLAMA_ARCH}"
OLLAMA_CACHE_DIR="${OLLAMA_CACHE_DIR:-build/ollama/debian-$OLLAMA_DEBIAN_SUITE-$OLLAMA_DEBIAN_ARCH}"
CURL_BIN="${CURL_BIN:-curl}"
ZSTD_BIN="${ZSTD_BIN:-zstd}"
TAR_BIN="${TAR_BIN:-tar}"
AR_BIN="${AR_BIN:-ar}"
XZ_BIN="${XZ_BIN:-xz}"
GZIP_BIN="${GZIP_BIN:-gzip}"

require_tool() {
    tool="$1"
    if ! command -v "$tool" >/dev/null 2>&1; then
        $STATUS error "Missing required tool: $tool"
        exit 1
    fi
}

case "$OLLAMA_ARCH" in
    amd64|arm64) ;;
    *)
        $STATUS error "Unsupported Ollama Linux architecture: $OLLAMA_ARCH"
        $STATUS error "Expected one of: amd64, arm64"
        exit 2
        ;;
esac

case "$OLLAMA_DEBIAN_ARCH" in
    amd64)
        DEBIAN_MULTIARCH="x86_64-linux-gnu"
        DEBIAN_LOADER="$OLLAMA_ROOT/lib64/ld-linux-x86-64.so.2"
        ;;
    arm64)
        DEBIAN_MULTIARCH="aarch64-linux-gnu"
        DEBIAN_LOADER="$OLLAMA_ROOT/lib/ld-linux-aarch64.so.1"
        ;;
    *)
        $STATUS error "Unsupported Debian runtime architecture: $OLLAMA_DEBIAN_ARCH"
        $STATUS error "Expected one of: amd64, arm64"
        exit 2
        ;;
esac

require_tool "$CURL_BIN"
require_tool "$ZSTD_BIN"
require_tool "$TAR_BIN"
require_tool "$AR_BIN"
require_tool "$XZ_BIN"
require_tool "$GZIP_BIN"

download_file() {
    url="$1"
    output="$2"

    if [ -f "$output" ]; then
        $STATUS info "Reusing cached download $output"
        return
    fi

    $STATUS info "Downloading $url"
    $CURL_BIN -fL "$url" -o "$output"
}

find_debian_package() {
    package="$1"
    index="$2"

    $XZ_BIN -dc "$index" | awk -v package="$package" '
        BEGIN { RS = ""; FS = "\n" }
        {
            found = 0
            filename = ""
            for (i = 1; i <= NF; i++) {
                if ($i == "Package: " package) {
                    found = 1
                } else if ($i ~ /^Filename: /) {
                    filename = $i
                    sub(/^Filename: /, "", filename)
                }
            }
            if (found && filename != "") {
                print filename
                exit
            }
        }
    '
}

extract_deb() {
    deb="$1"
    extract_dir="$OLLAMA_CACHE_DIR/extract-$(basename "$deb" .deb)"
    deb_abs="$(cd "$(dirname "$deb")" && pwd)/$(basename "$deb")"

    rm -rf "$extract_dir"
    mkdir -p "$extract_dir"
    (
        cd "$extract_dir"
        $AR_BIN -x "$deb_abs"
    )

    data_archive="$(find "$extract_dir" -name 'data.tar.*' -type f | head -n 1)"
    if [ -z "$data_archive" ]; then
        $STATUS error "No data archive found in $deb"
        exit 1
    fi

    case "$data_archive" in
        *.tar.xz) $XZ_BIN -dc "$data_archive" | $TAR_BIN -C "$OLLAMA_ROOT" -xf - ;;
        *.tar.zst) $ZSTD_BIN -dc "$data_archive" | $TAR_BIN -C "$OLLAMA_ROOT" -xf - ;;
        *.tar.gz) $GZIP_BIN -dc "$data_archive" | $TAR_BIN -C "$OLLAMA_ROOT" -xf - ;;
        *)
            $STATUS error "Unsupported Debian data archive format: $data_archive"
            exit 1
            ;;
    esac
}

install_debian_runtime() {
    index="$OLLAMA_CACHE_DIR/Packages.xz"
    mkdir -p "$OLLAMA_CACHE_DIR"

    download_file \
        "$OLLAMA_DEBIAN_MIRROR/dists/$OLLAMA_DEBIAN_SUITE/main/binary-$OLLAMA_DEBIAN_ARCH/Packages.xz" \
        "$index"

    for package in libc6 libgcc-s1 libstdc++6; do
        filename="$(find_debian_package "$package" "$index")"
        if [ -z "$filename" ]; then
            $STATUS error "Could not find Debian package in index: $package"
            exit 1
        fi

        deb="$OLLAMA_CACHE_DIR/$(basename "$filename")"
        download_file "$OLLAMA_DEBIAN_MIRROR/$filename" "$deb"
        $STATUS info "Extracting Debian runtime package $package"
        extract_deb "$deb"
    done
}

mkdir -p "$(dirname "$OLLAMA_ARCHIVE")" "$OLLAMA_ROOT/usr"

download_file "$OLLAMA_URL" "$OLLAMA_ARCHIVE"

if [ ! -x "$OLLAMA_ROOT/usr/bin/ollama" ]; then
    $STATUS info "Extracting Ollama into $OLLAMA_ROOT/usr"
    rm -rf "$OLLAMA_ROOT/usr/bin/ollama" "$OLLAMA_ROOT/usr/lib/ollama"
    $ZSTD_BIN -dc "$OLLAMA_ARCHIVE" | $TAR_BIN -C "$OLLAMA_ROOT/usr" -xf -
fi

if [ "$OLLAMA_DEBIAN_RUNTIME" = "1" ]; then
    install_debian_runtime
fi

if [ ! -x "$OLLAMA_ROOT/usr/bin/ollama" ]; then
    $STATUS error "Ollama binary was not found after extraction: $OLLAMA_ROOT/usr/bin/ollama"
    exit 1
fi

if [ "$OLLAMA_DEBIAN_RUNTIME" = "1" ]; then
    for path in \
        "$DEBIAN_LOADER" \
        "$OLLAMA_ROOT/lib/$DEBIAN_MULTIARCH/libc.so.6" \
        "$OLLAMA_ROOT/lib/$DEBIAN_MULTIARCH/libgcc_s.so.1" \
        "$OLLAMA_ROOT/usr/lib/$DEBIAN_MULTIARCH/libstdc++.so.6"; do
        if [ ! -e "$path" ]; then
            $STATUS error "Missing Ollama runtime dependency after Debian extraction: $path"
            exit 1
        fi
    done
fi

$STATUS success "Prepared Ollama rootfs overlay at $OLLAMA_ROOT"
