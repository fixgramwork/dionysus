#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
DESTDIR="${DESTDIR:-${1:-build/pve-control-plane-rootfs}}"
PVE_DIR="${PVE_DIR:-pve}"
SYSTEMD_DIR="${SYSTEMD_DIR:-packaging/systemd}"
DIONYSUS_PROFILES_DIR="${DIONYSUS_PROFILES_DIR:-profiles}"

require_file() {
    path="$1"
    if [ ! -f "$path" ]; then
        $STATUS error "Missing required file: $path"
        exit 1
    fi
}

require_file "$PVE_DIR/lib/Dionysus/PVE/API.pm"
require_file "$PVE_DIR/bin/dionysus-pvedaemon"
require_file "$PVE_DIR/bin/dionysus-pveproxy"
require_file "$PVE_DIR/www/index.html"
require_file "$SYSTEMD_DIR/dionysus-pve.env"
require_file "$SYSTEMD_DIR/dionysus-pvedaemon.service"
require_file "$SYSTEMD_DIR/dionysus-pveproxy.service"
require_file "$SYSTEMD_DIR/dionysus-llm-swap"
require_file "$SYSTEMD_DIR/dionysus-llm-swap.service"
require_file "$SYSTEMD_DIR/dionysus-llm-swap.env"

$STATUS info "Installing Proxmox-style Dionysus control plane into $DESTDIR"
mkdir -p \
    "$DESTDIR/etc/dionysus" \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants" \
    "$DESTDIR/usr/lib/dionysus" \
    "$DESTDIR/usr/sbin" \
    "$DESTDIR/usr/share/dionysus/profiles" \
    "$DESTDIR/usr/share/dionysus-pve-manager/www" \
    "$DESTDIR/usr/share/perl5/Dionysus/PVE"

cp "$PVE_DIR/lib/Dionysus/PVE/API.pm" "$DESTDIR/usr/share/perl5/Dionysus/PVE/API.pm"
chmod 644 "$DESTDIR/usr/share/perl5/Dionysus/PVE/API.pm"

cp "$PVE_DIR/bin/dionysus-pvedaemon" "$DESTDIR/usr/sbin/dionysus-pvedaemon"
cp "$PVE_DIR/bin/dionysus-pveproxy" "$DESTDIR/usr/sbin/dionysus-pveproxy"
chmod 755 "$DESTDIR/usr/sbin/dionysus-pvedaemon" "$DESTDIR/usr/sbin/dionysus-pveproxy"

cp "$PVE_DIR/www/index.html" "$DESTDIR/usr/share/dionysus-pve-manager/www/index.html"
chmod 644 "$DESTDIR/usr/share/dionysus-pve-manager/www/index.html"

cp "$SYSTEMD_DIR/dionysus-pve.env" "$DESTDIR/etc/dionysus/pve.env"
chmod 644 "$DESTDIR/etc/dionysus/pve.env"

cp "$SYSTEMD_DIR/dionysus-pvedaemon.service" "$DESTDIR/etc/systemd/system/dionysus-pvedaemon.service"
cp "$SYSTEMD_DIR/dionysus-pveproxy.service" "$DESTDIR/etc/systemd/system/dionysus-pveproxy.service"
chmod 644 "$DESTDIR/etc/systemd/system/dionysus-pvedaemon.service" \
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
ln -sf ../dionysus-pvedaemon.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-pvedaemon.service"
ln -sf ../dionysus-pveproxy.service \
    "$DESTDIR/etc/systemd/system/multi-user.target.wants/dionysus-pveproxy.service"

$STATUS success "Installed Dionysus PVE control plane into $DESTDIR"
$STATUS info "pveproxy listens on port 8006 and exposes Proxmox-style /api2/json endpoints."
