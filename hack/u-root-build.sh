#!/usr/bin/env bash

set -euo pipefail
shopt -s extglob

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )

U_ROOT="${U_ROOT:-u-root}"
KBAKE="${KBAKE:-kbake}"
IRONCORE_IMAGE="${IRONCORE_IMAGE:-ironcore-image}"

U_ROOT_PKG="$(go list -m -f "{{ .Dir }}" github.com/u-root/u-root)"

KERNEL=""
EFI_STUB=""
U_ROOT_ARGS=()
IRONCORE_IMAGE_ARGS=()

while [[ $# -gt 0 ]]; do
  case $1 in
  --kernel|-k)
    KERNEL="$2"
    shift
    shift
    ;;
  builtin:*)
    U_ROOT_ARGS+=("$U_ROOT_PKG"/"${1#builtin:}")
    shift
    ;;
  -o)
    echo "Cannot specify output"
    exit 1
    ;;
  -t)
    IRONCORE_IMAGE_ARGS+=("-t")
    IRONCORE_IMAGE_ARGS+=("$2")
    shift
    shift
    ;;
  *)
    U_ROOT_ARGS+=("$1")
    shift
    ;;
  esac
done

if [[ "$KERNEL" == "" ]]; then
  echo "Must specify --kernel|-k"
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap "rm -rf $TMP_DIR" EXIT

"$KBAKE" get kernel "$KERNEL" > "$TMP_DIR/kernel"

GOOS=linux CGO_ENABLED=0 "$U_ROOT" \
  -o "$TMP_DIR/initrd" \
  ${U_ROOT_ARGS[@]+"${U_ROOT_ARGS[@]}"}

cat > "$TMP_DIR/Directfile" << EOF
kernel: kernel
initrds:
- initrd
EOF

"$IRONCORE_IMAGE" build -f "$TMP_DIR/Directfile" "$TMP_DIR" \
  "${IRONCORE_IMAGE_ARGS[@]+"${IRONCORE_IMAGE_ARGS[@]}"}"
