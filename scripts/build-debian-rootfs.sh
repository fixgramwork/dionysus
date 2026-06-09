#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
DEBIAN_ARCH="${DEBIAN_ARCH:-arm64}"
DEBIAN_SUITE="${DEBIAN_SUITE:-bookworm}"
DEBIAN_MIRROR="${DEBIAN_MIRROR:-https://deb.debian.org/debian}"
DEBIAN_SECURITY_MIRROR="${DEBIAN_SECURITY_MIRROR:-https://security.debian.org/debian-security}"
DEBIAN_ROOTFS_IMAGE="${1:-${DEBIAN_ROOTFS_IMAGE:-build/debian-$DEBIAN_ARCH/dionysus-debian-$DEBIAN_ARCH.ext4}}"
DEBIAN_KERNEL_OUT="${DEBIAN_KERNEL_OUT:-build/debian-$DEBIAN_ARCH/vmlinuz}"
DEBIAN_INITRD_OUT="${DEBIAN_INITRD_OUT:-build/debian-$DEBIAN_ARCH/initrd.img}"
DEBIAN_ROOTFS_SIZE="${DEBIAN_ROOTFS_SIZE:-8G}"
DEBIAN_ROOT_PASSWORD="${DEBIAN_ROOT_PASSWORD:-dionysus}"
DEBIAN_HOSTNAME="${DEBIAN_HOSTNAME:-dionysus}"
DEBIAN_ROOTFS_WORK_DIR="${DEBIAN_ROOTFS_WORK_DIR:-}"
DIONYSUSD_BIN="${DIONYSUSD_BIN:-build/dionysusd-linux-arm64}"
DIONYSUS_DEBIAN_INCLUDE_OLLAMA="${DIONYSUS_DEBIAN_INCLUDE_OLLAMA:-1}"
DIONYSUS_DEBIAN_LLM_SWAP_SIZE_MB="${DIONYSUS_DEBIAN_LLM_SWAP_SIZE_MB:-1024}"
LINUX_BUILDER_RUNNER="${LINUX_BUILDER_RUNNER:-sh scripts/run-linux-builder.sh}"

DEBIAN_PACKAGES="${DEBIAN_PACKAGES:-systemd systemd-sysv dbus ca-certificates curl iproute2 isc-dhcp-client iputils-ping procps sqlite3 util-linux kmod wpasupplicant openssh-server passwd sudo linux-image-arm64 initramfs-tools}"

abspath() {
    case "$1" in
        /*) printf '%s\n' "$1" ;;
        *) printf '%s/%s\n' "$(pwd)" "$1" ;;
    esac
}

require_tool() {
    tool="$1"
    if ! command -v "$tool" >/dev/null 2>&1; then
        $STATUS error "Missing required tool: $tool"
        exit 1
    fi
}

set_env_value() {
    path="$1"
    key="$2"
    value="$3"

    if grep -q "^$key=" "$path"; then
        sed -i "s|^$key=.*|$key=$value|" "$path"
    else
        printf '%s=%s\n' "$key" "$value" >> "$path"
    fi
}

debian_multiarch_dir() {
    case "$1" in
        amd64) printf 'x86_64-linux-gnu\n' ;;
        arm64) printf 'aarch64-linux-gnu\n' ;;
        *)
            $STATUS error "Unsupported Debian architecture for Ollama service: $1"
            exit 2
            ;;
    esac
}

fallback_to_builder() {
    if [ "${DIONYSUS_IN_LINUX_BUILDER:-0}" = "1" ]; then
        return
    fi

    needs_builder=0
    [ "$(uname -s)" = "Linux" ] || needs_builder=1
    command -v debootstrap >/dev/null 2>&1 || needs_builder=1
    command -v mkfs.ext4 >/dev/null 2>&1 || needs_builder=1

    if [ "$needs_builder" = "1" ]; then
        $STATUS warn "Debian rootfs build needs Linux debootstrap/e2fsprogs; using container builder"
        exec $LINUX_BUILDER_RUNNER build-debian-rootfs "$@"
    fi
}

chroot_run() {
    chroot "$ROOTFS_DIR" /usr/bin/env DEBIAN_FRONTEND=noninteractive "$@"
}

normalize_rootfs_permissions() {
    chmod 755 "$ROOTFS_DIR"
    for path in \
        "$ROOTFS_DIR/var" \
        "$ROOTFS_DIR/var/lib" \
        "$ROOTFS_DIR/var/lib/ollama" \
        "$ROOTFS_DIR/var/lib/ollama/models"
    do
        if [ -d "$path" ]; then
            chmod 755 "$path"
        fi
    done
}

write_base_config() {
    printf '%s\n' "$DEBIAN_HOSTNAME" > "$ROOTFS_DIR/etc/hostname"
    cat > "$ROOTFS_DIR/etc/hosts" <<EOF
127.0.0.1 localhost
127.0.1.1 $DEBIAN_HOSTNAME

::1 localhost ip6-localhost ip6-loopback
ff02::1 ip6-allnodes
ff02::2 ip6-allrouters
EOF

    cat > "$ROOTFS_DIR/etc/fstab" <<'EOF'
/dev/vda / ext4 defaults 0 1
tmpfs /tmp tmpfs mode=1777,nosuid,nodev 0 0
EOF

    cat > "$ROOTFS_DIR/etc/apt/sources.list" <<EOF
deb $DEBIAN_MIRROR $DEBIAN_SUITE main
deb $DEBIAN_MIRROR $DEBIAN_SUITE-updates main
deb $DEBIAN_SECURITY_MIRROR $DEBIAN_SUITE-security main
EOF

    mkdir -p "$ROOTFS_DIR/etc/apt/apt.conf.d"
    cat > "$ROOTFS_DIR/etc/apt/apt.conf.d/99dionysus-qemu" <<'EOF'
Acquire::ForceIPv4 "true";
Acquire::Languages "none";
EOF

    if [ -r /etc/resolv.conf ]; then
        cp /etc/resolv.conf "$ROOTFS_DIR/etc/resolv.conf"
    else
        printf 'nameserver 1.1.1.1\n' > "$ROOTFS_DIR/etc/resolv.conf"
    fi

    cat > "$ROOTFS_DIR/usr/sbin/policy-rc.d" <<'EOF'
#!/bin/sh
exit 101
EOF
    chmod 755 "$ROOTFS_DIR/usr/sbin/policy-rc.d"
}

install_packages() {
    $STATUS info "Installing Debian packages with apt-get"
    chroot_run apt-get update
    # shellcheck disable=SC2086
    chroot_run apt-get install -y --no-install-recommends $DEBIAN_PACKAGES
    chroot_run apt-get clean
    rm -rf "$ROOTFS_DIR/var/lib/apt/lists/"*
}

configure_login_and_console() {
    chroot "$ROOTFS_DIR" /bin/sh -c "printf 'root:%s\n' '$DEBIAN_ROOT_PASSWORD' | chpasswd"
    mkdir -p "$ROOTFS_DIR/etc/systemd/system/getty.target.wants"
    ln -sf /lib/systemd/system/serial-getty@.service \
        "$ROOTFS_DIR/etc/systemd/system/getty.target.wants/serial-getty@ttyAMA0.service"
    mkdir -p "$ROOTFS_DIR/etc/ssh/sshd_config.d"
    cat > "$ROOTFS_DIR/etc/ssh/sshd_config.d/90-dionysus-qemu.conf" <<'EOF'
PermitRootLogin yes
PasswordAuthentication yes
EOF
    chmod 644 "$ROOTFS_DIR/etc/ssh/sshd_config.d/90-dionysus-qemu.conf"
    : > "$ROOTFS_DIR/etc/machine-id"
}

install_dionysus_control_plane() {
    dionysusd_abs="$(abspath "$DIONYSUSD_BIN")"
    if [ ! -f "$dionysusd_abs" ]; then
        $STATUS error "Missing ARM64 dionysusd binary: $dionysusd_abs"
        $STATUS error "Run 'make dionysusd-linux-arm64' before building the Debian rootfs."
        exit 1
    fi

    $STATUS info "Installing Dionysus control plane into Debian rootfs"
    DESTDIR="$ROOTFS_DIR" \
        DIONYSUSD_BIN="$dionysusd_abs" \
        DIONYSUS_PROFILES_DIR="profiles" \
        sh scripts/install-pve-control-plane.sh

    network_env="$ROOTFS_DIR/etc/dionysus/network.env"
    set_env_value "$network_env" DIONYSUS_NETWORK_MODE lan
    set_env_value "$network_env" DIONYSUS_LAN_ENABLED 1
    set_env_value "$network_env" DIONYSUS_LAN_IFACE eth0
    set_env_value "$network_env" DIONYSUS_LAN_IPV4_METHOD dhcp
    set_env_value "$network_env" DIONYSUS_DHCP_CLIENT dhclient

    swap_env="$ROOTFS_DIR/etc/dionysus/llm-swap.env"
    set_env_value "$swap_env" DIONYSUS_LLM_SWAP_SIZE_MB "$DIONYSUS_DEBIAN_LLM_SWAP_SIZE_MB"
}

install_ollama() {
    if [ "$DIONYSUS_DEBIAN_INCLUDE_OLLAMA" != "1" ]; then
        $STATUS info "Skipping Ollama inclusion for Debian rootfs"
        return
    fi

    $STATUS info "Preparing Ollama for Debian rootfs"
    OLLAMA_ARCH="$DEBIAN_ARCH" OLLAMA_DEBIAN_RUNTIME=0 sh scripts/fetch-ollama-linux.sh
    ollama_root="${OLLAMA_ROOT:-build/ollama/linux-$DEBIAN_ARCH/rootfs}"
    ollama_multiarch="$(debian_multiarch_dir "$DEBIAN_ARCH")"

    if [ ! -x "$ollama_root/usr/bin/ollama" ]; then
        $STATUS error "Ollama binary missing after fetch: $ollama_root/usr/bin/ollama"
        exit 1
    fi

    mkdir -p "$ROOTFS_DIR/usr/bin" "$ROOTFS_DIR/usr/lib" "$ROOTFS_DIR/var/lib/ollama/models"
    cp "$ollama_root/usr/bin/ollama" "$ROOTFS_DIR/usr/bin/ollama"
    chmod 755 "$ROOTFS_DIR/usr/bin/ollama"
    if [ -d "$ollama_root/usr/lib/ollama" ]; then
        rm -rf "$ROOTFS_DIR/usr/lib/ollama"
        cp -R "$ollama_root/usr/lib/ollama" "$ROOTFS_DIR/usr/lib/ollama"
    fi
    chown -R 0:0 "$ROOTFS_DIR/usr/bin/ollama" "$ROOTFS_DIR/usr/lib/ollama" 2>/dev/null || true

    chroot "$ROOTFS_DIR" /bin/sh -c 'getent group ollama >/dev/null || groupadd --system ollama'
    chroot "$ROOTFS_DIR" /bin/sh -c 'id -u ollama >/dev/null 2>&1 || useradd --system --home /var/lib/ollama --shell /usr/sbin/nologin --gid ollama ollama'
    chroot "$ROOTFS_DIR" chown -R ollama:ollama /var/lib/ollama

    cat > "$ROOTFS_DIR/etc/systemd/system/ollama.service" <<EOF
[Unit]
Description=Ollama local model server
After=network-online.target dionysus-llm-swap.service
Wants=network-online.target dionysus-llm-swap.service

[Service]
Type=simple
User=ollama
Group=ollama
Environment=HOME=/var/lib/ollama
Environment=OLLAMA_HOST=127.0.0.1:11434
Environment=OLLAMA_MODELS=/var/lib/ollama/models
Environment=LD_LIBRARY_PATH=/lib/$ollama_multiarch:/usr/lib/$ollama_multiarch:/usr/lib/ollama
ExecStart=/usr/bin/ollama serve
Restart=on-failure
RestartSec=2s

[Install]
WantedBy=multi-user.target
EOF
    chmod 644 "$ROOTFS_DIR/etc/systemd/system/ollama.service"
    ln -sf ../ollama.service "$ROOTFS_DIR/etc/systemd/system/multi-user.target.wants/ollama.service"
}

copy_boot_artifacts() {
    kernel_image="$(find "$ROOTFS_DIR/boot" -maxdepth 1 -type f -name 'vmlinuz-*' | sort | tail -n 1)"
    initrd_image="$(find "$ROOTFS_DIR/boot" -maxdepth 1 -type f -name 'initrd.img-*' | sort | tail -n 1)"

    if [ -z "$kernel_image" ] || [ -z "$initrd_image" ]; then
        $STATUS error "Debian kernel or initrd was not created in $ROOTFS_DIR/boot"
        exit 1
    fi

    mkdir -p "$(dirname "$DEBIAN_KERNEL_OUT")" "$(dirname "$DEBIAN_INITRD_OUT")"
    cp "$kernel_image" "$DEBIAN_KERNEL_OUT"
    cp "$initrd_image" "$DEBIAN_INITRD_OUT"
}

pack_rootfs_image() {
    normalize_rootfs_permissions
    mkdir -p "$(dirname "$DEBIAN_ROOTFS_IMAGE")"
    rm -f "$DEBIAN_ROOTFS_IMAGE"
    truncate -s "$DEBIAN_ROOTFS_SIZE" "$DEBIAN_ROOTFS_IMAGE"
    $STATUS info "Packing Debian rootfs into $DEBIAN_ROOTFS_IMAGE"
    mkfs.ext4 -F -L dionysus-root -d "$ROOTFS_DIR" "$DEBIAN_ROOTFS_IMAGE" >/dev/null
}

fallback_to_builder "$@"

require_tool debootstrap
require_tool mkfs.ext4
require_tool truncate
require_tool chroot

if [ -z "$DEBIAN_ROOTFS_WORK_DIR" ]; then
    if [ "${DIONYSUS_IN_LINUX_BUILDER:-0}" = "1" ]; then
        DEBIAN_ROOTFS_WORK_DIR="/tmp/dionysus-debian-rootfs"
    else
        DEBIAN_ROOTFS_WORK_DIR="build/debian-$DEBIAN_ARCH/work"
    fi
fi

DEBIAN_ROOTFS_IMAGE="$(abspath "$DEBIAN_ROOTFS_IMAGE")"
DEBIAN_KERNEL_OUT="$(abspath "$DEBIAN_KERNEL_OUT")"
DEBIAN_INITRD_OUT="$(abspath "$DEBIAN_INITRD_OUT")"
ROOTFS_DIR="$DEBIAN_ROOTFS_WORK_DIR/rootfs"

rm -rf "$DEBIAN_ROOTFS_WORK_DIR"
mkdir -p "$ROOTFS_DIR"
chmod 755 "$ROOTFS_DIR"

$STATUS info "Bootstrapping Debian $DEBIAN_SUITE $DEBIAN_ARCH rootfs"
debootstrap --arch="$DEBIAN_ARCH" --variant=minbase "$DEBIAN_SUITE" "$ROOTFS_DIR" "$DEBIAN_MIRROR"

write_base_config
install_packages
configure_login_and_console
install_dionysus_control_plane
install_ollama
copy_boot_artifacts
pack_rootfs_image

$STATUS success "Created Debian rootfs image $DEBIAN_ROOTFS_IMAGE"
$STATUS success "Copied Debian kernel $DEBIAN_KERNEL_OUT"
$STATUS success "Copied Debian initrd $DEBIAN_INITRD_OUT"
