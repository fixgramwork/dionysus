[BITS 32]

MULTIBOOT_ALIGN  equ 1 << 0
MULTIBOOT_MEMMAP equ 1 << 1
MULTIBOOT_FLAGS  equ MULTIBOOT_ALIGN | MULTIBOOT_MEMMAP
MULTIBOOT_MAGIC  equ 0x1BADB002
MULTIBOOT_CHECK  equ -(MULTIBOOT_MAGIC + MULTIBOOT_FLAGS)

section .multiboot
align 4
    dd MULTIBOOT_MAGIC
    dd MULTIBOOT_FLAGS
    dd MULTIBOOT_CHECK

section .bss
align 16
stack_bottom:
    resb 16384
stack_top:

section .text
global start
extern kernel_main

start:
    cli
    mov esp, stack_top
    push ebx
    push eax
    call kernel_main

.halt:
    cli
    hlt
    jmp .halt
