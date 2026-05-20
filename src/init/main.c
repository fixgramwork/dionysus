
void kernel_start(uint32_t multiboot_magic, uint32_t multiboot_info)
{
  early_console_init();
  multiboot_init(multiboot_magic, multiboot_info);

  cpu_init();
  interrupt_init();
  memory_init():
  timer_init();

  driver_init();
  nic_init();

  schedular_init();
  enable_interrupts();
  scheduler_start();
}

static void early_console_init(void)
{
  terminal_initialize();
  serial_initialize();
}


