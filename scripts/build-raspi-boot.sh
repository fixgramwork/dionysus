#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
LINUX_BUILD_DIR="${1:-${RASPI_BUILD_DIR:-build/raspi/linux}}"
INITRAMFS_ARCHIVE="${2:-${RASPI_INITRAMFS_ARCHIVE:-build/raspi/initramfs/dionysus-initramfs.cpio.gz}}"
OUTPUT_DIR="${3:-${RASPI_BOOT_DIR:-build/raspi/boot}}"
CONFIG_TEMPLATE="${4:-${RASPI_CONFIG_TEMPLATE:-config/raspberry-pi/config.txt}}"
CMDLINE_TEMPLATE="${5:-${RASPI_CMDLINE_TEMPLATE:-config/raspberry-pi/cmdline.txt}}"
KERNEL_NAME="${RASPI_KERNEL_NAME:-kernel8.img}"
KERNEL_IMAGE="${RASPI_KERNEL_IMAGE:-$LINUX_BUILD_DIR/arch/arm64/boot/Image}"
DTB_DIR="${RASPI_DTB_DIR:-$LINUX_BUILD_DIR/arch/arm64/boot/dts/broadcom}"
OVERLAY_DIR="${RASPI_OVERLAY_DIR:-$LINUX_BUILD_DIR/arch/arm64/boot/dts/overlays}"
FIRMWARE_DIR="${RASPI_FIRMWARE_DIR:-}"

if [ ! -f "$KERNEL_IMAGE" ]; then
    $STATUS error "Raspberry Pi kernel image not found: $KERNEL_IMAGE"
    exit 1
fi

if [ ! -f "$INITRAMFS_ARCHIVE" ]; then
    $STATUS error "Raspberry Pi initramfs not found: $INITRAMFS_ARCHIVE"
    exit 1
fi

if [ ! -f "$CONFIG_TEMPLATE" ]; then
    $STATUS error "Raspberry Pi config template not found: $CONFIG_TEMPLATE"
    exit 1
fi

if [ ! -f "$CMDLINE_TEMPLATE" ]; then
    $STATUS error "Raspberry Pi cmdline template not found: $CMDLINE_TEMPLATE"
    exit 1
fi

rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR" "$OUTPUT_DIR/overlays"

$STATUS info "Installing kernel as $KERNEL_NAME"
cp "$KERNEL_IMAGE" "$OUTPUT_DIR/$KERNEL_NAME"
cp "$INITRAMFS_ARCHIVE" "$OUTPUT_DIR/dionysus-initramfs.cpio.gz"
cp "$CONFIG_TEMPLATE" "$OUTPUT_DIR/config.txt"
cp "$CMDLINE_TEMPLATE" "$OUTPUT_DIR/cmdline.txt"

if [ -d "$DTB_DIR" ]; then
    $STATUS info "Copying Raspberry Pi device trees from $DTB_DIR"
    find "$DTB_DIR" -maxdepth 1 -type f -name '*rpi*.dtb' -exec cp {} "$OUTPUT_DIR/" \;
else
    $STATUS warn "Device tree directory not found: $DTB_DIR"
fi

if [ -d "$OVERLAY_DIR" ]; then
    $STATUS info "Copying Raspberry Pi overlays from $OVERLAY_DIR"
    find "$OVERLAY_DIR" -maxdepth 1 -type f \( -name '*.dtbo' -o -name '*.dtb' -o -name '*.dtb*' -o -name 'README' \) -exec cp {} "$OUTPUT_DIR/overlays/" \;
else
    $STATUS warn "Overlay directory not found: $OVERLAY_DIR"
fi

if [ -n "$FIRMWARE_DIR" ]; then
    if [ ! -d "$FIRMWARE_DIR" ]; then
        $STATUS error "Raspberry Pi firmware directory not found: $FIRMWARE_DIR"
        exit 1
    fi

    $STATUS info "Copying Raspberry Pi firmware files from $FIRMWARE_DIR"
    find "$FIRMWARE_DIR" -maxdepth 1 -type f \( -name 'bootcode.bin' -o -name 'start*.elf' -o -name 'fixup*.dat' \) -exec cp {} "$OUTPUT_DIR/" \;
fi

$STATUS success "Prepared Raspberry Pi boot overlay in $OUTPUT_DIR"
$STATUS info "Copy these files to a Raspberry Pi FAT boot partition that already contains firmware files, or rerun with RASPI_FIRMWARE_DIR=/path/to/firmware/boot."
