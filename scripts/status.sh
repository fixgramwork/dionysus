#!/bin/sh

level="${1:-info}"
shift 2>/dev/null || true
message="$*"

case "$level" in
    info)
        label="INFO"
        color="34"
        stream="stdout"
        ;;
    success)
        label=" OK "
        color="32"
        stream="stdout"
        ;;
    warn)
        label="WARN"
        color="33"
        stream="stdout"
        ;;
    error)
        label="FAIL"
        color="31"
        stream="stderr"
        ;;
    *)
        label="LOG "
        color="36"
        stream="stdout"
        ;;
esac

use_color=0
if [ -z "${NO_COLOR:-}" ]; then
    if [ "${FORCE_COLOR:-0}" = "1" ]; then
        use_color=1
    elif [ "$stream" = "stderr" ] && [ -t 2 ]; then
        use_color=1
    elif [ "$stream" = "stdout" ] && [ -t 1 ]; then
        use_color=1
    fi
fi

if [ "$use_color" -eq 1 ]; then
    prefix="$(printf '\033[%sm[%s]\033[0m' "$color" "$label")"
else
    prefix="[$label]"
fi

if [ "$stream" = "stderr" ]; then
    printf '%s %s\n' "$prefix" "$message" >&2
else
    printf '%s %s\n' "$prefix" "$message"
fi
