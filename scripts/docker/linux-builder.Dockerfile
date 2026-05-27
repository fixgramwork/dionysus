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
        kmod \
        libelf-dev \
        libssl-dev \
        python3 \
        rsync \
        xz-utils \
        zstd \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
