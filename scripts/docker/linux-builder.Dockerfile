FROM debian:bookworm-slim

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        bc \
        bison \
        build-essential \
        busybox-static \
        ca-certificates \
        cpio \
        dwarves \
        file \
        flex \
        binutils-aarch64-linux-gnu \
        gcc-aarch64-linux-gnu \
        git \
        debian-archive-keyring \
        debootstrap \
        e2fsprogs \
        kmod \
        libelf-dev \
        libssl-dev \
        python3 \
        qemu-user-static \
        rsync \
        xz-utils \
        zstd \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
