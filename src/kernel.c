#include <stddef.h>
#include <stdint.h>

#define MULTIBOOT_BOOTLOADER_MAGIC 0x2BADB002u
#define VGA_WIDTH 80
#define VGA_HEIGHT 25

enum vga_color {
    VGA_COLOR_BLACK = 0,
    VGA_COLOR_BLUE = 1,
    VGA_COLOR_GREEN = 2,
    VGA_COLOR_CYAN = 3,
    VGA_COLOR_RED = 4,
    VGA_COLOR_MAGENTA = 5,
    VGA_COLOR_BROWN = 6,
    VGA_COLOR_LIGHT_GREY = 7,
    VGA_COLOR_DARK_GREY = 8,
    VGA_COLOR_LIGHT_BLUE = 9,
    VGA_COLOR_LIGHT_GREEN = 10,
    VGA_COLOR_LIGHT_CYAN = 11,
    VGA_COLOR_LIGHT_RED = 12,
    VGA_COLOR_LIGHT_MAGENTA = 13,
    VGA_COLOR_LIGHT_BROWN = 14,
    VGA_COLOR_WHITE = 15,
};

static volatile uint16_t *const vga_buffer = (uint16_t *)0xB8000;
static size_t terminal_row = 0;
static size_t terminal_column = 0;
static uint8_t terminal_color = 0;
static int serial_enabled = 0;

static inline uint8_t vga_entry_color(enum vga_color fg, enum vga_color bg) {
    return (uint8_t)(fg | bg << 4);
}

static inline uint16_t vga_entry(unsigned char character, uint8_t color) {
    return (uint16_t)character | (uint16_t)color << 8;
}

static inline void outb(uint16_t port, uint8_t value) {
    __asm__ volatile("outb %0, %1" : : "a"(value), "Nd"(port));
}

static inline uint8_t inb(uint16_t port) {
    uint8_t value;
    __asm__ volatile("inb %1, %0" : "=a"(value) : "Nd"(port));
    return value;
}

static void serial_write_char(char character) {
    if (!serial_enabled) {
        return;
    }

    while ((inb(0x3F8 + 5) & 0x20) == 0) {
    }

    outb(0x3F8, (uint8_t)character);
}

static void terminal_clear(void) {
    for (size_t y = 0; y < VGA_HEIGHT; ++y) {
        for (size_t x = 0; x < VGA_WIDTH; ++x) {
            const size_t index = y * VGA_WIDTH + x;
            vga_buffer[index] = vga_entry(' ', terminal_color);
        }
    }

    terminal_row = 0;
    terminal_column = 0;
}

static void terminal_scroll(void) {
    for (size_t y = 1; y < VGA_HEIGHT; ++y) {
        for (size_t x = 0; x < VGA_WIDTH; ++x) {
            vga_buffer[(y - 1) * VGA_WIDTH + x] = vga_buffer[y * VGA_WIDTH + x];
        }
    }

    for (size_t x = 0; x < VGA_WIDTH; ++x) {
        vga_buffer[(VGA_HEIGHT - 1) * VGA_WIDTH + x] = vga_entry(' ', terminal_color);
    }

    terminal_row = VGA_HEIGHT - 1;
    terminal_column = 0;
}

static void terminal_initialize(void) {
    terminal_color = vga_entry_color(VGA_COLOR_LIGHT_GREY, VGA_COLOR_BLACK);
    terminal_clear();
}

static void terminal_putentryat(char character, uint8_t color, size_t x, size_t y) {
    const size_t index = y * VGA_WIDTH + x;
    vga_buffer[index] = vga_entry((unsigned char)character, color);
}

static void terminal_putchar(char character) {
    if (character == '\n') {
        serial_write_char('\r');
        serial_write_char('\n');
        terminal_column = 0;
        ++terminal_row;
        if (terminal_row >= VGA_HEIGHT) {
            terminal_scroll();
        }
        return;
    }

    serial_write_char(character);
    terminal_putentryat(character, terminal_color, terminal_column, terminal_row);
    ++terminal_column;

    if (terminal_column >= VGA_WIDTH) {
        terminal_column = 0;
        ++terminal_row;
    }

    if (terminal_row >= VGA_HEIGHT) {
        terminal_scroll();
    }
}

static void terminal_write(const char *data, size_t size) {
    for (size_t index = 0; index < size; ++index) {
        terminal_putchar(data[index]);
    }
}

static void terminal_writestring(const char *data) {
    size_t length = 0;
    while (data[length] != '\0') {
        ++length;
    }

    terminal_write(data, length);
}

static void terminal_write_hex(uint32_t value) {
    static const char digits[] = "0123456789ABCDEF";
    char buffer[10];

    buffer[0] = '0';
    buffer[1] = 'x';
    for (size_t index = 0; index < 8; ++index) {
        const uint32_t shift = (uint32_t)(28 - index * 4);
        buffer[index + 2] = digits[(value >> shift) & 0xF];
    }

    terminal_write(buffer, sizeof(buffer));
}

static void serial_initialize(void) {
    outb(0x3F8 + 1, 0x00);
    outb(0x3F8 + 3, 0x80);
    outb(0x3F8 + 0, 0x03);
    outb(0x3F8 + 1, 0x00);
    outb(0x3F8 + 3, 0x03);
    outb(0x3F8 + 2, 0xC7);
    outb(0x3F8 + 4, 0x0B);
    serial_enabled = 1;
}

void kernel_main(uint32_t multiboot_magic, uint32_t multiboot_info) {
    (void)multiboot_info;

    terminal_initialize();
    serial_initialize();

    terminal_writestring("Dionysus kernel booted.\n");
    terminal_writestring("Build pipeline: ELF -> ISO -> QEMU\n");

    if (multiboot_magic != MULTIBOOT_BOOTLOADER_MAGIC) {
        terminal_writestring("Unexpected multiboot magic: ");
        terminal_write_hex(multiboot_magic);
        terminal_writestring("\nSystem halted.\n");
        for (;;) {
            __asm__ volatile("hlt");
        }
    }

    terminal_writestring("GRUB loaded the kernel successfully.\n");
    terminal_writestring("Next step: replace kernel_main() with your scheduler, memory manager, and drivers.\n");

    for (;;) {
        __asm__ volatile("hlt");
    }
}
