.DEFAULT_GOAL := run

OS_NAME := dionysus
CROSS ?= i686-elf-
STATUS := sh scripts/status.sh

CC := $(CROSS)gcc
LD := $(CROSS)ld
AS := nasm
GRUB_MKRESCUE ?= $(shell if command -v grub-mkrescue >/dev/null 2>&1; then printf '%s' grub-mkrescue; elif command -v i686-elf-grub-mkrescue >/dev/null 2>&1; then printf '%s' i686-elf-grub-mkrescue; else printf '%s' grub-mkrescue; fi)
QEMU := qemu-system-x86_64
XORRISO := xorriso

BUILD_DIR := build
OBJ_DIR := $(BUILD_DIR)/obj
ISO_ROOT := $(BUILD_DIR)/iso
KERNEL_ELF := $(BUILD_DIR)/kernel.elf
ISO_FILE := $(BUILD_DIR)/$(OS_NAME).iso

C_SOURCES := src/kernel.c
ASM_SOURCES := src/boot.asm
OBJECTS := $(patsubst src/%.c,$(OBJ_DIR)/%.o,$(C_SOURCES)) \
	$(patsubst src/%.asm,$(OBJ_DIR)/%.o,$(ASM_SOURCES))

CFLAGS := -std=gnu11 -O2 -Wall -Wextra -ffreestanding -fno-stack-protector -fno-pie -m32
LDFLAGS := -m elf_i386 -T src/linker.ld -nostdlib
QEMU_FLAGS := -cdrom $(ISO_FILE) -m 256M -serial stdio

.PHONY: all build iso run clean help check-kernel-tools check-iso-tools check-run-tools

all: run

help:
	@$(STATUS) info "make           Build the kernel, create the ISO, and boot it in QEMU."
	@$(STATUS) info "make build     Compile and link the freestanding kernel ELF."
	@$(STATUS) info "make iso       Package the kernel into a bootable GRUB ISO."
	@$(STATUS) info "make run       Boot the generated ISO with QEMU."
	@$(STATUS) info "make clean     Remove generated build artifacts."

check-kernel-tools:
	@$(STATUS) info "Checking kernel build tools"
	@for tool in "$(CC)" "$(LD)" "$(AS)"; do \
		if ! command -v "$$tool" >/dev/null 2>&1; then \
			$(STATUS) error "Missing required tool: $$tool"; \
			$(STATUS) error "Install the cross-compiler and assembler before building the kernel."; \
			exit 1; \
		fi; \
	done
	@$(STATUS) success "Kernel build tools are available"

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

build: $(KERNEL_ELF)

iso: $(ISO_FILE)

run: check-run-tools $(ISO_FILE)
	@$(STATUS) info "Booting $(ISO_FILE) with QEMU"
	@if $(QEMU) $(QEMU_FLAGS); then \
		$(STATUS) success "QEMU session finished"; \
	else \
		status=$$?; \
		$(STATUS) error "QEMU failed while booting $(ISO_FILE)"; \
		exit $$status; \
	fi

$(KERNEL_ELF): check-kernel-tools $(OBJECTS) src/linker.ld
	@mkdir -p $(dir $@)
	@$(STATUS) info "Linking kernel ELF -> $@"
	@if $(LD) $(LDFLAGS) -o $@ $(OBJECTS); then \
		$(STATUS) success "Linked $@"; \
	else \
		status=$$?; \
		$(STATUS) error "Kernel link failed"; \
		exit $$status; \
	fi

$(ISO_FILE): check-iso-tools $(KERNEL_ELF) config/grub.cfg
	@mkdir -p $(ISO_ROOT)/boot/grub
	@$(STATUS) info "Copying kernel and GRUB config into ISO root"
	@cp $(KERNEL_ELF) $(ISO_ROOT)/boot/$(OS_NAME).elf
	@cp config/grub.cfg $(ISO_ROOT)/boot/grub/grub.cfg
	@$(STATUS) info "Packaging bootable ISO -> $@"
	@if $(GRUB_MKRESCUE) -o $@ $(ISO_ROOT); then \
		$(STATUS) success "Created $@"; \
	else \
		status=$$?; \
		$(STATUS) error "ISO packaging failed"; \
		exit $$status; \
	fi

$(OBJ_DIR)/%.o: src/%.c
	@mkdir -p $(dir $@)
	@$(STATUS) info "Compiling C source $<"
	@if $(CC) $(CFLAGS) -c $< -o $@; then \
		$(STATUS) success "Built $@"; \
	else \
		status=$$?; \
		$(STATUS) error "Compilation failed for $<"; \
		exit $$status; \
	fi

$(OBJ_DIR)/%.o: src/%.asm
	@mkdir -p $(dir $@)
	@$(STATUS) info "Assembling $<"
	@if $(AS) -f elf32 $< -o $@; then \
		$(STATUS) success "Built $@"; \
	else \
		status=$$?; \
		$(STATUS) error "Assembly failed for $<"; \
		exit $$status; \
	fi

clean:
	@$(STATUS) warn "Removing build artifacts from $(BUILD_DIR)"
	@rm -rf $(BUILD_DIR)
	@$(STATUS) success "Cleaned build directory"
