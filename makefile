.DEFAULT_GOAL := run

STATUS := sh scripts/status.sh
LINUX_FETCH := sh scripts/fetch-linux.sh
INITRAMFS_BUILD := sh scripts/build-initramfs.sh
RASPI_BOOT_BUILD := sh scripts/build-raspi-boot.sh
PVE_CONTROL_PLANE_INSTALL := sh scripts/install-pve-control-plane.sh
DEBIAN_ROOTFS_BUILD := sh scripts/build-debian-rootfs.sh
QEMU_DEBIAN_RUN := sh scripts/run-qemu-debian.sh

GIT := git

BUILD_DIR := build
INITRAMFS_DIR ?= initramfs/overlay
PVE_CONTROL_PLANE_ROOTFS ?= $(BUILD_DIR)/pve-control-plane-rootfs
PVE_CHECK_ROOTFS ?= $(BUILD_DIR)/pve-check-rootfs
DIONYSUSD_BIN ?= $(BUILD_DIR)/dionysusd
DIONYSUSD_LINUX_ARM64 ?= $(BUILD_DIR)/dionysusd-linux-arm64
DIONYSUS_GO_CACHE ?= $(BUILD_DIR)/gocache
DIONYSUS_GO_MOD_CACHE ?= $(BUILD_DIR)/gomodcache
DIONYSUS_DEV_LISTEN ?= 127.0.0.1:18008
DIONYSUS_DEV_DIR ?= $(BUILD_DIR)/dev
DIONYSUS_DEV_METRICS_DIR ?= $(DIONYSUS_DEV_DIR)/metrics
DIONYSUS_DEV_METRICS_DB ?= $(DIONYSUS_DEV_METRICS_DIR)/ollama.sqlite3
DIONYSUS_DEV_TOKEN_FILE ?= $(DIONYSUS_DEV_DIR)/pve.token
DIONYSUS_DEV_USERS_FILE ?= $(DIONYSUS_DEV_DIR)/pve.users.json
DIONYSUS_DEV_USER_FILE ?= $(DIONYSUS_DEV_DIR)/pve.user
DIONYSUS_DEV_PASSWORD_FILE ?= $(DIONYSUS_DEV_DIR)/pve.password
DIONYSUS_DEV_USERNAME ?= root
DIONYSUS_DEV_PASSWORD ?= dionysus
DIONYSUS_DEV_NETWORK_CONFIG ?= $(DIONYSUS_DEV_DIR)/network.env
DIONYSUS_DEV_WWW ?= pve/www
PVE_UI_DIR ?= frontend/svelte
PVE_UI_INDEX := pve/www/index.html
GO_SOURCES := $(wildcard cmd/dionysusd/*.go)
PVE_UI_SOURCES := $(PVE_UI_DIR)/package.json $(PVE_UI_DIR)/index.html $(PVE_UI_DIR)/vite.config.js $(wildcard $(PVE_UI_DIR)/src/*)
DEBIAN_ARCH ?= arm64
DEBIAN_ROOTFS_IMAGE ?= $(BUILD_DIR)/debian-$(DEBIAN_ARCH)/dionysus-debian-$(DEBIAN_ARCH).ext4
DEBIAN_KERNEL_IMAGE ?= $(BUILD_DIR)/debian-$(DEBIAN_ARCH)/vmlinuz
DEBIAN_INITRD_IMAGE ?= $(BUILD_DIR)/debian-$(DEBIAN_ARCH)/initrd.img

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
RASPI_KERNEL_NAME ?= kernel8.img

.PHONY: all build run clean help dionysusd dionysusd-linux-arm64 pve-ui dev-data pve-check pve-control-plane-install pve-control-plane-host-install debian-rootfs debian-qemu-run raspi-fetch raspi-status raspi-initramfs raspi-boot check-raspi-fetch-tools check-initramfs-tools check-pve-tools

all: build

help:
	@$(STATUS) info "make           Run the Go/Svelte control-plane web UI in local development mode."
	@$(STATUS) info "make build     Build the Go daemon and Svelte operator UI."
	@$(STATUS) info "make run       Serve the Go/Svelte control-plane web UI on $(DIONYSUS_DEV_LISTEN)."
	@$(STATUS) info "make pve-control-plane-install Stage the Go/Svelte API2 web control plane."
	@$(STATUS) info "make pve-control-plane-host-install Install the Ollama RAM control plane on this systemd host."
	@$(STATUS) info "make debian-rootfs Build a Debian ARM64 rootfs image with apt and Dionysus services."
	@$(STATUS) info "make debian-qemu-run Boot the Debian ARM64 rootfs in QEMU."
	@$(STATUS) info "make raspi-fetch  Clone or update Raspberry Pi Linux in $(RASPI_LINUX_DIR)."
	@$(STATUS) info "make raspi-initramfs Build a Raspberry Pi ARM64 initramfs archive."
	@$(STATUS) info "make raspi-boot   Package a Raspberry Pi boot overlay using an existing kernel image."
	@$(STATUS) info "make clean     Remove generated build artifacts."

check-raspi-fetch-tools:
	@$(STATUS) info "Checking Raspberry Pi Linux source fetch tools"
	@if ! command -v "$(GIT)" >/dev/null 2>&1; then \
		$(STATUS) error "Missing required tool: $(GIT)"; \
		$(STATUS) error "Install git before fetching upstream Linux."; \
		exit 1; \
	fi
	@$(STATUS) success "Raspberry Pi Linux source fetch tools are available"

check-initramfs-tools:
	@$(STATUS) info "Checking initramfs packaging tools"
	@for tool in cpio gzip; do \
		if ! command -v "$$tool" >/dev/null 2>&1; then \
			$(STATUS) error "Missing required tool: $$tool"; \
			$(STATUS) error "Install initramfs packaging tools before building an initramfs archive."; \
			exit 1; \
		fi; \
	done
	@$(STATUS) success "Initramfs packaging tools are available"

check-pve-tools:
	@$(STATUS) info "Checking Go/Svelte control-plane tools"
	@for tool in go npm sqlite3; do \
		if ! command -v "$$tool" >/dev/null 2>&1; then \
			$(STATUS) error "Missing required tool: $$tool"; \
			$(STATUS) error "Install go, npm, and sqlite3 before staging the control plane."; \
			exit 1; \
		fi; \
	done
	@$(STATUS) success "Go/Svelte control-plane tools are available"

build: dionysusd pve-ui

dev-data:
	@mkdir -p "$(DIONYSUS_DEV_METRICS_DIR)"
	@if [ ! -f "$(DIONYSUS_DEV_TOKEN_FILE)" ]; then \
		if command -v openssl >/dev/null 2>&1; then openssl rand -hex 32 > "$(DIONYSUS_DEV_TOKEN_FILE)"; \
		else printf 'dionysus-dev-jwt-secret\n' > "$(DIONYSUS_DEV_TOKEN_FILE)"; fi; \
	fi
	@printf '%s\n' "$(DIONYSUS_DEV_USERNAME)" > "$(DIONYSUS_DEV_USER_FILE)"
	@printf '%s\n' "$(DIONYSUS_DEV_PASSWORD)" > "$(DIONYSUS_DEV_PASSWORD_FILE)"
	@if [ ! -f "$(DIONYSUS_DEV_USERS_FILE)" ]; then \
		now=$$(date +%s); \
		printf '{\n  "users": [\n    {\n      "username": "%s",\n      "passwordHash": "%s",\n      "permissions": [\n        "node.read",\n        "network.manage",\n        "llm.manage",\n        "services.manage",\n        "console.run"\n      ],\n      "createdAt": %s,\n      "updatedAt": %s\n    }\n  ]\n}\n' "$(DIONYSUS_DEV_USERNAME)" "$(DIONYSUS_DEV_PASSWORD)" "$$now" "$$now" > "$(DIONYSUS_DEV_USERS_FILE)"; \
	fi
	@chmod 600 "$(DIONYSUS_DEV_TOKEN_FILE)" "$(DIONYSUS_DEV_USERS_FILE)" "$(DIONYSUS_DEV_USER_FILE)" "$(DIONYSUS_DEV_PASSWORD_FILE)"

run: dev-data dionysusd pve-ui
	@$(STATUS) info "Starting Dionysus control-plane UI at http://$(DIONYSUS_DEV_LISTEN)"
	@$(DIONYSUSD_BIN) proxy --dev-allow-host --listen "$(DIONYSUS_DEV_LISTEN)" --www-root "$(DIONYSUS_DEV_WWW)" --metrics-db "$(DIONYSUS_DEV_METRICS_DB)" --token-file "$(DIONYSUS_DEV_TOKEN_FILE)" --auth-users-file "$(DIONYSUS_DEV_USERS_FILE)" --auth-user-file "$(DIONYSUS_DEV_USER_FILE)" --auth-password-file "$(DIONYSUS_DEV_PASSWORD_FILE)" --network-config "$(DIONYSUS_DEV_NETWORK_CONFIG)"

dionysusd: $(DIONYSUSD_BIN)

dionysusd-linux-arm64: $(DIONYSUSD_LINUX_ARM64)

$(DIONYSUSD_BIN): go.mod $(GO_SOURCES)
	@$(STATUS) info "Building Go control-plane daemon"
	@mkdir -p "$(dir $(DIONYSUSD_BIN))" "$(DIONYSUS_GO_CACHE)" "$(DIONYSUS_GO_MOD_CACHE)"
	@GOCACHE="$(abspath $(DIONYSUS_GO_CACHE))" GOMODCACHE="$(abspath $(DIONYSUS_GO_MOD_CACHE))" go build -o "$(DIONYSUSD_BIN)" ./cmd/dionysusd

$(DIONYSUSD_LINUX_ARM64): go.mod $(GO_SOURCES)
	@$(STATUS) info "Building Linux ARM64 Go control-plane daemon"
	@mkdir -p "$(dir $(DIONYSUSD_LINUX_ARM64))" "$(DIONYSUS_GO_CACHE)" "$(DIONYSUS_GO_MOD_CACHE)"
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 GOCACHE="$(abspath $(DIONYSUS_GO_CACHE))" GOMODCACHE="$(abspath $(DIONYSUS_GO_MOD_CACHE))" go build -o "$(DIONYSUSD_LINUX_ARM64)" ./cmd/dionysusd

pve-ui: $(PVE_UI_INDEX)

$(PVE_UI_INDEX): $(PVE_UI_SOURCES)
	@$(STATUS) info "Building Svelte operator UI"
	@cd "$(PVE_UI_DIR)" && npm install
	@cd "$(PVE_UI_DIR)" && npm run build

pve-check: check-pve-tools $(DIONYSUSD_BIN) pve-ui
	@$(STATUS) info "Running Go control-plane unit tests"
	@GOCACHE="$(abspath $(DIONYSUS_GO_CACHE))" GOMODCACHE="$(abspath $(DIONYSUS_GO_MOD_CACHE))" go test ./cmd/dionysusd
	@$(STATUS) info "Checking control-plane shell wrappers"
	@sh -n pve/bin/dionysus-metricsd
	@sh -n pve/bin/dionysus-pvedaemon
	@sh -n pve/bin/dionysus-pveproxy
	@sh -n packaging/systemd/dionysus-network
	@sh -n scripts/install-pve-control-plane.sh
	@$(STATUS) info "Verifying staged PVE control-plane install in $(PVE_CHECK_ROOTFS)"
	@DESTDIR="$(PVE_CHECK_ROOTFS)" DIONYSUSD_BIN="$(DIONYSUSD_BIN)" DIONYSUS_PROFILES_DIR="profiles" $(PVE_CONTROL_PLANE_INSTALL)

pve-control-plane-install: pve-check scripts/install-pve-control-plane.sh $(DIONYSUSD_BIN) pve/bin/dionysus-metricsd pve/bin/dionysus-pvedaemon pve/bin/dionysus-pveproxy $(PVE_UI_INDEX) packaging/systemd/dionysus-pve.env packaging/systemd/dionysus-network packaging/systemd/dionysus-network.env packaging/systemd/dionysus-network.service packaging/systemd/dionysus-metricsd.service packaging/systemd/dionysus-pvedaemon.service packaging/systemd/dionysus-pveproxy.service packaging/systemd/dionysus-llm-swap.service packaging/systemd/dionysus-llm-swap.env packaging/systemd/dionysus-llm-swap
	@$(STATUS) info "Staging Proxmox-style Dionysus control plane in $(PVE_CONTROL_PLANE_ROOTFS)"
	@DESTDIR="$(PVE_CONTROL_PLANE_ROOTFS)" DIONYSUSD_BIN="$(DIONYSUSD_BIN)" DIONYSUS_PROFILES_DIR="profiles" $(PVE_CONTROL_PLANE_INSTALL)

pve-control-plane-host-install: pve-check scripts/install-pve-control-plane.sh
	@$(STATUS) info "Installing Dionysus Ollama RAM control plane on this host"
	@DIONYSUSD_BIN="$(DIONYSUSD_BIN)" $(PVE_CONTROL_PLANE_INSTALL) --host

debian-rootfs: $(DIONYSUSD_LINUX_ARM64) pve-ui scripts/build-debian-rootfs.sh scripts/install-pve-control-plane.sh scripts/fetch-ollama-linux.sh
	@$(STATUS) info "Building Debian $(DEBIAN_ARCH) rootfs image at $(DEBIAN_ROOTFS_IMAGE)"
	@DEBIAN_ARCH="$(DEBIAN_ARCH)" DIONYSUSD_BIN="$(DIONYSUSD_LINUX_ARM64)" DEBIAN_KERNEL_OUT="$(DEBIAN_KERNEL_IMAGE)" DEBIAN_INITRD_OUT="$(DEBIAN_INITRD_IMAGE)" $(DEBIAN_ROOTFS_BUILD) "$(DEBIAN_ROOTFS_IMAGE)"

debian-qemu-run: debian-rootfs scripts/run-qemu-debian.sh
	@$(STATUS) info "Booting Debian $(DEBIAN_ARCH) rootfs image in QEMU"
	@DEBIAN_ARCH="$(DEBIAN_ARCH)" DEBIAN_KERNEL_IMAGE="$(DEBIAN_KERNEL_IMAGE)" DEBIAN_INITRD_IMAGE="$(DEBIAN_INITRD_IMAGE)" $(QEMU_DEBIAN_RUN) "$(DEBIAN_ROOTFS_IMAGE)"

raspi-fetch: check-raspi-fetch-tools $(RASPI_FRAGMENT) scripts/fetch-linux.sh
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

raspi-boot: $(RASPI_INITRAMFS_ARCHIVE) $(RASPI_CONFIG_TEMPLATE) $(RASPI_CMDLINE_TEMPLATE) scripts/build-raspi-boot.sh
	@if [ ! -f "$(RASPI_KERNEL_IMAGE)" ]; then \
		$(STATUS) error "Raspberry Pi kernel image not found: $(RASPI_KERNEL_IMAGE)"; \
		$(STATUS) error "Kernel builds are intentionally not handled by this Makefile anymore."; \
		exit 1; \
	fi
	@$(STATUS) info "Preparing Raspberry Pi boot partition overlay in $(RASPI_BOOT_DIR)"
	@RASPI_KERNEL_NAME="$(RASPI_KERNEL_NAME)" $(RASPI_BOOT_BUILD) "$(RASPI_BUILD_DIR)" "$(RASPI_INITRAMFS_ARCHIVE)" "$(RASPI_BOOT_DIR)" "$(RASPI_CONFIG_TEMPLATE)" "$(RASPI_CMDLINE_TEMPLATE)"

clean:
	@$(STATUS) warn "Removing build artifacts from $(BUILD_DIR)"
	@rm -rf $(BUILD_DIR)
	@$(STATUS) success "Cleaned build directory"
