#!/bin/sh

set -eu

STATUS="sh scripts/status.sh"
LINUX_REMOTE="${LINUX_REMOTE:-https://github.com/torvalds/linux.git}"
LINUX_REF="${1:-${LINUX_REF:-master}}"
LINUX_DIR="${2:-${LINUX_DIR:-upstream/linux}}"
LINUX_DEPTH="${LINUX_DEPTH:-1}"

if ! command -v git >/dev/null 2>&1; then
    $STATUS error "Missing required tool: git"
    exit 1
fi

if [ ! -d "$LINUX_DIR/.git" ]; then
    $STATUS info "Initializing Linux checkout in $LINUX_DIR"
    mkdir -p "$LINUX_DIR"
    git init "$LINUX_DIR"
    git -C "$LINUX_DIR" remote add origin "$LINUX_REMOTE"
else
    $STATUS info "Reusing existing Linux checkout at $LINUX_DIR"
fi

git -C "$LINUX_DIR" remote set-url origin "$LINUX_REMOTE"
$STATUS info "Fetching Linux ref $LINUX_REF from $LINUX_REMOTE"
if [ "$LINUX_DEPTH" = "0" ]; then
    git -C "$LINUX_DIR" fetch --tags origin "$LINUX_REF"
else
    git -C "$LINUX_DIR" fetch --depth="$LINUX_DEPTH" origin "$LINUX_REF"
fi

$STATUS info "Checking out Linux ref $LINUX_REF"
git -C "$LINUX_DIR" checkout --detach FETCH_HEAD

current_commit="$(git -C "$LINUX_DIR" rev-parse HEAD)"
$STATUS success "Linux checkout is ready at $LINUX_DIR"
$STATUS info "Checked out commit $current_commit"
