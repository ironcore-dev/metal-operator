#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

# install-promtool.sh downloads a pre-built promtool binary from the
# Prometheus GitHub release artifacts with checksum verification.
#
# Usage: install-promtool.sh <version> <output-dir>
#   version    - Prometheus release version (e.g. 2.54.0, without leading 'v')
#   output-dir - Directory to place the promtool binary in

set -euo pipefail

VERSION="${1:?Usage: install-promtool.sh <version> <output-dir>}"
OUTPUT_DIR="${2:?Usage: install-promtool.sh <version> <output-dir>}"

# Strip leading 'v' if present
VERSION="${VERSION#v}"

BINARY="${OUTPUT_DIR}/promtool"

if [[ -f "${BINARY}" ]] && "${BINARY}" --version 2>&1 | grep -q "${VERSION}"; then
    echo "promtool ${VERSION} already installed at ${BINARY}"
    exit 0
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "${ARCH}" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
esac

TARBALL="prometheus-${VERSION}.${OS}-${ARCH}.tar.gz"
RELEASE_URL="https://github.com/prometheus/prometheus/releases/download/v${VERSION}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

echo "Downloading promtool ${VERSION} for ${OS}/${ARCH}..."
curl -sSfL "${RELEASE_URL}/${TARBALL}" -o "${TMP_DIR}/${TARBALL}"
curl -sSfL "${RELEASE_URL}/sha256sums.txt" -o "${TMP_DIR}/sha256sums.txt"

cd "${TMP_DIR}"
if ! grep -F "${TARBALL}" sha256sums.txt | sha256sum -c -; then
    echo "ERROR: SHA256 checksum verification failed for ${TARBALL}" >&2
    exit 1
fi

tar -xzf "${TARBALL}" -C "${TMP_DIR}"
mv "prometheus-${VERSION}.${OS}-${ARCH}/promtool" "${BINARY}"
chmod +x "${BINARY}"
echo "Installed promtool ${VERSION} at ${BINARY}"
