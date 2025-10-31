#!/usr/bin/env bash
set -euo pipefail

# Prepare runtime/state dirs (avoid writes to /usr)
mkdir -p /run/lldpd /var/lib/lldpd || true

# Start lldpd if not running. Try init script first; fallback to direct start
if ! pgrep -x lldpd >/dev/null 2>&1; then
  if [ -x /etc/init.d/lldpd ]; then
    /etc/init.d/lldpd start
  else
    lldpd -d -I /run/lldpd -s /var/lib/lldpd &
  fi
fi

# Wait for lldpd to start
sleep 1

exec /metalprobe "$@"