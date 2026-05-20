.DEFAULT_GOAL := run

OS_NAME := dionysus
STATUS := sh scripts/status.sh
LINUX_FETCH := sh scripts/fetch-linux.sh
INITRAMFS_BUILD := sh scripts/build-initramfs.sh
LINUX_BUILD := sh scripts/build-linux-kernel.sh
RASPI_BOOT_BUILD := sh scripts/build-raspi-boot.sh
PVE_CONTROL_PLANE_INSTALL := sh scripts/install-pve-control-plane.sh

GIT := git
GRUB_MKRESCUE ?= $(shell if command -v grub-mkrescue >/dev/null 2>&1; then printf '%s' grub-mkrescue; elif command -v i686-elf-grub-mkrescue >/dev/null 2>&1; then printf '%s' i686-elf-grub-mkrescue; else printf '%s' grub-mkrescue; fi)
QEMU := qemu-system-x86_64
QEMU_AARCH64 := qemu-system-aarch64
XORRISO := xorriso

BUILD_DIR := build
LINUX_DIR ?= upstream/linux
LINUX_REF ?= master
LINUX_FRAGMENT := config/linux/x86_64-dionysus.fragment
LINUX_GRUB_CFG := config/grub-linux.cfg
LINUX_BUILD_DIR ?= $(BUILD_DIR)/linux
LINUX_BZIMAGE := $(LINUX_BUILD_DIR)/arch/x86/boot/bzImage
LINUX_ISO_ROOT := $(BUILD_DIR)/linux-iso
LINUX_ISO_FILE := $(BUILD_DIR)/$(OS_NAME)-linux.iso
INITRAMFS_DIR ?= initramfs/overlay
INITRAMFS_STAGING ?= $(BUILD_DIR)/initramfs/rootfs
INITRAMFS_ARCHIVE ?= $(BUILD_DIR)/initramfs/dionysus-initramfs.cpio.gz
PVE_CONTROL_PLANE_ROOTFS ?= $(BUILD_DIR)/pve-control-plane-rootfs

LINUX_QEMU_FLAGS := -cdrom $(LINUX_ISO_FILE) -m 1024M -serial stdio

ARM64_QEMU_FRAGMENT := config/linux/arm64-qemu-dionysus.fragment
ARM64_QEMU_BUILD_DIR ?= $(BUILD_DIR)/linux-arm64-qemu
ARM64_QEMU_IMAGE := $(ARM64_QEMU_BUILD_DIR)/arch/arm64/boot/Image
ARM64_QEMU_INITRAMFS_STAGING ?= $(BUILD_DIR)/qemu-arm64/initramfs/rootfs
ARM64_QEMU_INITRAMFS_ARCHIVE ?= $(BUILD_DIR)/qemu-arm64/initramfs/dionysus-initramfs.cpio.gz
ARM64_QEMU_BUILDER_IMAGE ?= dionysus/linux-builder:bookworm-arm64
ARM64_QEMU_BUILDER_PLATFORM ?= linux/arm64
ARM64_QEMU_FLAGS := -M virt -cpu cortex-a57 -m 1024M -nographic -kernel $(ARM64_QEMU_IMAGE) -initrd $(ARM64_QEMU_INITRAMFS_ARCHIVE) -append "console=ttyAMA0 rdinit=/init"

RASPI_LINUX_DIR ?= upstream/raspberrypi-linux
RASPI_LINUX_REMOTE ?= https://github.com/raspberrypi/linux.git
RASPI_LINUX_REF ?= rpi-6.12.y
RASPI_FRAGMENT := config/linux/arm64-raspi-dionysus.fragment
RASPI_BUILD_DIR ?= $(BUILD_DIR)/raspi/linux
RASPI_KERNEL_IMAGE := $(RASPI_BUILD_DIR)/arch/arm64/boot/Image
RASPI_BOOT_DIR ?= $(BUILD_DIR)/raspi/boot
RASPI_INITRAMFS_STAGING ?= $(BUILD_DIR)/raspi/initramfs/rootfs
RASPI_INITRAMFS_ARCHIVE ?= $(BUILD_DIR)/raspi/initramfs/dionysus-initramfs.cpio.gz
RASPI_CONFIG_TEMPLATE := config/raspberry-pi/config.txt
RASPI_CMDLINE_TEMPLATE := config/raspberry-pi/cmdline.txt
RASPI_CONFIG_TARGET ?= bcm2711_defconfig
RASPI_KERNEL_TARGETS ?= Image dtbs
RASPI_KERNEL_NAME ?= kernel8.img
RASPI_CROSS_COMPILE ?= aarch64-linux-gnu-

.PHONY: all build iso run clean help pve-check pve-control-plane-install linux-fetch linux-status linux-initramfs linux-initramfs-layout linux-kernel linux-iso linux-run linux-qemu-arm64-kernel linux-qemu-arm64-initramfs linux-qemu-arm64-run raspi-fetch raspi-status raspi-initramfs raspi-kernel raspi-boot check-iso-tools check-run-tools check-arm64-run-tools check-linux-tools check-linux-build-tools check-initramfs-tools check-pve-tools

all: run

help:
	@$(STATUS) info "make           Build the Linux-based Dionysus OS path and boot it in QEMU."
	@$(STATUS) info "make build     Build the upstream Linux kernel image used by the OS."
	@$(STATUS) info "make iso       Package Linux + initramfs into a bootable GRUB ISO."
	@$(STATUS) info "make run       Boot the Linux-based Dionysus OS in QEMU."
	@$(STATUS) info "make linux-fetch  Clone or update upstream Linux in $(LINUX_DIR)."
	@$(STATUS) info "make linux-status Show the current upstream Linux checkout status."
	@$(STATUS) info "make linux-initramfs Build a Dionysus initramfs archive."
	@$(STATUS) info "make linux-initramfs-layout Prepare the initramfs layout without packing it."
	@$(STATUS) info "make linux-kernel Build upstream Linux into $(LINUX_BZIMAGE)."
	@$(STATUS) info "make linux-iso    Package bzImage + initramfs into a GRUB ISO."
	@$(STATUS) info "make linux-run    Boot the Linux-based ISO with QEMU."
	@$(STATUS) info "make linux-qemu-arm64-run Boot a fast ARM64 QEMU initramfs smoke VM."
	@$(STATUS) info "make pve-control-plane-install Stage the Proxmox-style Perl/API2/ExtJS control plane."
	@$(STATUS) info "make raspi-fetch  Clone or update Raspberry Pi Linux in $(RASPI_LINUX_DIR)."
	@$(STATUS) info "make raspi-boot   Build a Raspberry Pi ARM64 boot partition overlay."
	@$(STATUS) info "make clean     Remove generated build artifacts."

check-linux-tools:
	@$(STATUS) info "Checking Linux source management tools"
	@if ! command -v "$(GIT)" >/dev/null 2>&1; then \
		$(STATUS) error "Missing required tool: $(GIT)"; \
		$(STATUS) error "Install git before fetching upstream Linux."; \
		exit 1; \
	fi
	@$(STATUS) success "Linux source management tools are available"

check-linux-build-tools:
	@$(STATUS) info "Checking Linux kernel build tools"
	@if ! command -v "$(MAKE)" >/dev/null 2>&1; then \
		$(STATUS) error "Missing required tool: $(MAKE)"; \
		$(STATUS) error "Install make before building upstream Linux."; \
		exit 1; \
	fi
	@if [ ! -d "$(LINUX_DIR)/.git" ]; then \
		$(STATUS) error "Linux checkout not found at $(LINUX_DIR)"; \
		$(STATUS) error "Run 'make linux-fetch LINUX_REF=<ref>' first."; \
		exit 1; \
	fi
	@$(STATUS) success "Linux kernel build prerequisites look available"

check-initramfs-tools:
	@$(STATUS) info "Checking initramfs packaging tools"
	@for tool in cpio gzip; do \
		if ! command -v "$$tool" >/dev/null 2>&1; then \
			$(STATUS) error "Missing required tool: $$tool"; \
			$(STATUS) error "Install initramfs packaging tools before building the Linux boot payload."; \
			exit 1; \
		fi; \
	done
	@$(STATUS) success "Initramfs packaging tools are available"

check-pve-tools:
	@$(STATUS) info "Checking Proxmox-style Perl control-plane tools"
	@if ! command -v perl >/dev/null 2>&1; then \
		$(STATUS) error "Missing required tool: perl"; \
		$(STATUS) error "Install Perl before staging the PVE control plane."; \
		exit 1; \
	fi
	@perl -MJSON::PP -MHTTP::Daemon -MHTTP::Status -MHTTP::Tiny -MGetopt::Long -e '1'
	@$(STATUS) success "Perl control-plane tools are available"

check-iso-tools:
	@$(STATUS) info "Checking ISO packaging tools"
	@for tool in "$(GRUB_MKRESCUE)" "$(XORRISO)"; do \
		if ! command -v "$$tool" >/dev/null 2>&1; then \
			$(STATUS) error "Missing required tool: $$tool"; \
			$(STATUS) error "Install the GRUB tools and xorriso before packaging the ISO."; \
			exit 1; \
		fi; \
	done
	@$(STATUS) success "ISO packaging tools are available"

check-run-tools:
	@$(STATUS) info "Checking QEMU runtime"
	@if ! command -v "$(QEMU)" >/dev/null 2>&1; then \
		$(STATUS) error "Missing required tool: $(QEMU)"; \
		$(STATUS) error "Install QEMU to boot the generated ISO."; \
		exit 1; \
	fi
	@$(STATUS) success "QEMU is available"

check-arm64-run-tools:
	@$(STATUS) info "Checking ARM64 QEMU runtime"
	@if ! command -v "$(QEMU_AARCH64)" >/dev/null 2>&1; then \
		$(STATUS) error "Missing required tool: $(QEMU_AARCH64)"; \
		$(STATUS) error "Install QEMU to boot the ARM64 smoke VM."; \
		exit 1; \
	fi
	@$(STATUS) success "ARM64 QEMU is available"

build: linux-kernel

iso: linux-iso

run: linux-run

linux-fetch: check-linux-tools $(LINUX_FRAGMENT) scripts/fetch-linux.sh
	@$(STATUS) info "Fetching upstream Linux ref $(LINUX_REF)"
	@$(LINUX_FETCH) "$(LINUX_REF)" "$(LINUX_DIR)"

linux-status:
	@$(STATUS) info "Linux base configuration fragment: $(LINUX_FRAGMENT)"
	@if [ -d "$(LINUX_DIR)/.git" ]; then \
		remote_url="$$(git -C "$(LINUX_DIR)" remote get-url origin)"; \
		current_commit="$$(git -C "$(LINUX_DIR)" rev-parse HEAD)"; \
		$(STATUS) info "Linux checkout: $(LINUX_DIR)"; \
		$(STATUS) info "Remote: $$remote_url"; \
		$(STATUS) info "Commit: $$current_commit"; \
	else \
		$(STATUS) warn "Linux checkout not found at $(LINUX_DIR)"; \
		$(STATUS) warn "Run 'make linux-fetch LINUX_REF=<ref>' to create it."; \
	fi

linux-initramfs-layout: check-initramfs-tools scripts/build-initramfs.sh $(INITRAMFS_DIR)/init $(INITRAMFS_DIR)/usr/bin/dionysus-agent $(INITRAMFS_DIR)/usr/bin/dionysus-services
	@$(STATUS) info "Preparing Linux initramfs layout from $(INITRAMFS_DIR)"
	@LAYOUT_ONLY=1 REQUIRE_BUSYBOX=0 DIONYSUS_PROFILES_DIR="profiles" $(INITRAMFS_BUILD) "$(INITRAMFS_DIR)" "$(INITRAMFS_ARCHIVE)" "$(INITRAMFS_STAGING)"

$(INITRAMFS_ARCHIVE): check-initramfs-tools scripts/build-initramfs.sh $(INITRAMFS_DIR)/init $(INITRAMFS_DIR)/usr/bin/dionysus-agent $(INITRAMFS_DIR)/usr/bin/dionysus-services
	@$(STATUS) info "Building Linux initramfs archive from $(INITRAMFS_DIR)"
	@DIONYSUS_PROFILES_DIR="profiles" $(INITRAMFS_BUILD) "$(INITRAMFS_DIR)" "$(INITRAMFS_ARCHIVE)" "$(INITRAMFS_STAGING)"

linux-initramfs: $(INITRAMFS_ARCHIVE)

$(LINUX_BZIMAGE): check-linux-build-tools scripts/build-linux-kernel.sh $(LINUX_FRAGMENT)
	@$(STATUS) info "Building upstream Linux bzImage from $(LINUX_DIR)"
	@$(LINUX_BUILD) "$(LINUX_DIR)" "$(LINUX_BUILD_DIR)" "$(LINUX_FRAGMENT)"

linux-kernel: $(LINUX_BZIMAGE)

$(LINUX_ISO_FILE): check-iso-tools $(LINUX_BZIMAGE) $(INITRAMFS_ARCHIVE) $(LINUX_GRUB_CFG)
	@mkdir -p $(LINUX_ISO_ROOT)/boot/grub
	@$(STATUS) info "Copying Linux kernel, initramfs, and GRUB config into ISO root"
	@cp $(LINUX_BZIMAGE) $(LINUX_ISO_ROOT)/boot/vmlinuz
	@cp $(INITRAMFS_ARCHIVE) $(LINUX_ISO_ROOT)/boot/initramfs.cpio.gz
	@cp $(LINUX_GRUB_CFG) $(LINUX_ISO_ROOT)/boot/grub/grub.cfg
	@$(STATUS) info "Packaging Linux boot ISO -> $@"
	@if $(GRUB_MKRESCUE) -o $@ $(LINUX_ISO_ROOT); then \
		$(STATUS) success "Created $@"; \
	else \
		status=$$?; \
		$(STATUS) error "Linux ISO packaging failed"; \
		exit $$status; \
	fi

linux-iso: $(LINUX_ISO_FILE)

linux-run: check-run-tools $(LINUX_ISO_FILE)
	@$(STATUS) info "Booting $(LINUX_ISO_FILE) with QEMU"
	@if $(QEMU) $(LINUX_QEMU_FLAGS); then \
		$(STATUS) success "Linux QEMU session finished"; \
	else \
		status=$$?; \
		$(STATUS) error "QEMU failed while booting $(LINUX_ISO_FILE)"; \
		exit $$status; \
	fi

$(ARM64_QEMU_IMAGE): check-linux-build-tools scripts/build-linux-kernel.sh $(ARM64_QEMU_FRAGMENT)
	@$(STATUS) info "Building ARM64 QEMU Linux Image from $(LINUX_DIR)"
	@DIONYSUS_LINUX_BUILDER_IMAGE="$(ARM64_QEMU_BUILDER_IMAGE)" DIONYSUS_LINUX_BUILDER_PLATFORM="$(ARM64_QEMU_BUILDER_PLATFORM)" ARCH=arm64 CONFIG_TARGET=allnoconfig KERNEL_TARGETS=Image KERNEL_IMAGE_PATH="$(ARM64_QEMU_IMAGE)" $(LINUX_BUILD) "$(LINUX_DIR)" "$(ARM64_QEMU_BUILD_DIR)" "$(ARM64_QEMU_FRAGMENT)"

linux-qemu-arm64-kernel: $(ARM64_QEMU_IMAGE)

$(ARM64_QEMU_INITRAMFS_ARCHIVE): check-initramfs-tools scripts/build-initramfs.sh $(INITRAMFS_DIR)/init $(INITRAMFS_DIR)/usr/bin/dionysus-agent $(INITRAMFS_DIR)/usr/bin/dionysus-services
	@$(STATUS) info "Building ARM64 QEMU initramfs archive from $(INITRAMFS_DIR)"
	@DIONYSUS_LINUX_BUILDER_IMAGE="$(ARM64_QEMU_BUILDER_IMAGE)" DIONYSUS_LINUX_BUILDER_PLATFORM="$(ARM64_QEMU_BUILDER_PLATFORM)" DIONYSUS_PROFILES_DIR="profiles" $(INITRAMFS_BUILD) "$(INITRAMFS_DIR)" "$(ARM64_QEMU_INITRAMFS_ARCHIVE)" "$(ARM64_QEMU_INITRAMFS_STAGING)"

linux-qemu-arm64-initramfs: $(ARM64_QEMU_INITRAMFS_ARCHIVE)

linux-qemu-arm64-run: check-arm64-run-tools $(ARM64_QEMU_IMAGE) $(ARM64_QEMU_INITRAMFS_ARCHIVE)
	@$(STATUS) info "Booting ARM64 QEMU smoke VM"
	@if $(QEMU_AARCH64) $(ARM64_QEMU_FLAGS); then \
		$(STATUS) success "ARM64 QEMU session finished"; \
	else \
		status=$$?; \
		$(STATUS) error "ARM64 QEMU failed"; \
		exit $$status; \
	fi

pve-check: check-pve-tools
	@$(STATUS) info "Checking Proxmox-style Perl modules and daemons"
	@perl -I pve/lib -c pve/lib/Dionysus/PVE/API.pm
	@perl -I pve/lib -c pve/bin/dionysus-pvedaemon
	@perl -I pve/lib -c pve/bin/dionysus-pveproxy

pve-control-plane-install: pve-check scripts/install-pve-control-plane.sh pve/lib/Dionysus/PVE/API.pm pve/bin/dionysus-pvedaemon pve/bin/dionysus-pveproxy pve/www/index.html packaging/systemd/dionysus-pve.env packaging/systemd/dionysus-pvedaemon.service packaging/systemd/dionysus-pveproxy.service packaging/systemd/dionysus-llm-swap.service packaging/systemd/dionysus-llm-swap.env packaging/systemd/dionysus-llm-swap
	@$(STATUS) info "Staging Proxmox-style Dionysus control plane in $(PVE_CONTROL_PLANE_ROOTFS)"
	@DESTDIR="$(PVE_CONTROL_PLANE_ROOTFS)" DIONYSUS_PROFILES_DIR="profiles" $(PVE_CONTROL_PLANE_INSTALL)

raspi-fetch: check-linux-tools $(RASPI_FRAGMENT) scripts/fetch-linux.sh
	@$(STATUS) info "Fetching Raspberry Pi Linux ref $(RASPI_LINUX_REF)"
	@LINUX_REMOTE="$(RASPI_LINUX_REMOTE)" LINUX_DEPTH="$${LINUX_DEPTH:-1}" $(LINUX_FETCH) "$(RASPI_LINUX_REF)" "$(RASPI_LINUX_DIR)"

raspi-status:
	@$(STATUS) info "Raspberry Pi Linux configuration fragment: $(RASPI_FRAGMENT)"
	@if [ -d "$(RASPI_LINUX_DIR)/.git" ]; then \
		remote_url="$$(git -C "$(RASPI_LINUX_DIR)" remote get-url origin)"; \
		current_commit="$$(git -C "$(RASPI_LINUX_DIR)" rev-parse HEAD)"; \
		$(STATUS) info "Linux checkout: $(RASPI_LINUX_DIR)"; \
		$(STATUS) info "Remote: $$remote_url"; \
		$(STATUS) info "Commit: $$current_commit"; \
	else \
		$(STATUS) warn "Raspberry Pi Linux checkout not found at $(RASPI_LINUX_DIR)"; \
		$(STATUS) warn "Run 'make raspi-fetch RASPI_LINUX_REF=<ref>' to create it."; \
	fi

$(RASPI_INITRAMFS_ARCHIVE): check-initramfs-tools scripts/build-initramfs.sh $(INITRAMFS_DIR)/init $(INITRAMFS_DIR)/usr/bin/dionysus-agent $(INITRAMFS_DIR)/usr/bin/dionysus-services
	@$(STATUS) info "Building Raspberry Pi ARM64 initramfs archive from $(INITRAMFS_DIR)"
	@DIONYSUS_PROFILES_DIR="profiles" $(INITRAMFS_BUILD) "$(INITRAMFS_DIR)" "$(RASPI_INITRAMFS_ARCHIVE)" "$(RASPI_INITRAMFS_STAGING)"

raspi-initramfs: $(RASPI_INITRAMFS_ARCHIVE)

$(RASPI_KERNEL_IMAGE): LINUX_DIR := $(RASPI_LINUX_DIR)
$(RASPI_KERNEL_IMAGE): check-linux-build-tools scripts/build-linux-kernel.sh $(RASPI_FRAGMENT)
	@$(STATUS) info "Building Raspberry Pi ARM64 Linux Image from $(RASPI_LINUX_DIR)"
	@ARCH=arm64 CROSS_COMPILE="$(RASPI_CROSS_COMPILE)" CONFIG_TARGET="$(RASPI_CONFIG_TARGET)" KERNEL_TARGETS="$(RASPI_KERNEL_TARGETS)" KERNEL_IMAGE_PATH="$(RASPI_KERNEL_IMAGE)" $(LINUX_BUILD) "$(RASPI_LINUX_DIR)" "$(RASPI_BUILD_DIR)" "$(RASPI_FRAGMENT)"

raspi-kernel: $(RASPI_KERNEL_IMAGE)

raspi-boot: $(RASPI_KERNEL_IMAGE) $(RASPI_INITRAMFS_ARCHIVE) $(RASPI_CONFIG_TEMPLATE) $(RASPI_CMDLINE_TEMPLATE) scripts/build-raspi-boot.sh
	@$(STATUS) info "Preparing Raspberry Pi boot partition overlay in $(RASPI_BOOT_DIR)"
	@RASPI_KERNEL_NAME="$(RASPI_KERNEL_NAME)" $(RASPI_BOOT_BUILD) "$(RASPI_BUILD_DIR)" "$(RASPI_INITRAMFS_ARCHIVE)" "$(RASPI_BOOT_DIR)" "$(RASPI_CONFIG_TEMPLATE)" "$(RASPI_CMDLINE_TEMPLATE)"

clean:
	@$(STATUS) warn "Removing build artifacts from $(BUILD_DIR)"
	@rm -rf $(BUILD_DIR)
	@$(STATUS) success "Cleaned build directory"
