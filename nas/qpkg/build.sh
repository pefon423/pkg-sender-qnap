#!/bin/sh
set -eu

# Builds a QNAP QPKG for one CPU architecture using the official qbuild tool
# (github.com/qnap-dev/QDK). This script must run on a Linux build host or
# WSL: it needs `sh`, `go`, and `qbuild`, none of which are available on a
# plain Windows shell.
#
# ARCH selection matters and is intentionally NOT defaulted: pick the wrong
# one and the binary will not run on the target NAS (illegal instruction on a
# float-ABI mismatch, or App Center will not even offer the package). Confirm
# your NAS's exact architecture before a real build -- see README.md.

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
NAS_DIR="$(CDPATH= cd -- "${SCRIPT_DIR}/.." && pwd)"
OUT_DIR="${OUT_DIR:-${SCRIPT_DIR}/dist}"
VERSION="${VERSION:-0.1.0-0001}"
GO_BIN="${GO_BIN:-go}"
QBUILD_BIN="${QBUILD_BIN:-qbuild}"

ARCH="${ARCH:-}"
if [ -z "${ARCH}" ]; then
    echo "Set ARCH to one of: x86 arm-x19 arm-x31 arm-x41 arm_64" >&2
    echo "Unsure which one your NAS needs? See the 'Determine your ARCH' section in README.md." >&2
    exit 2
fi

case "${ARCH}" in
    x86)
        # Intel Atom (e.g. TS-459 Pro / D510) running 32-bit QTS 4.2.x.
        # GOARCH=386 even on CPUs that support 64-bit: QTS 4.2.x itself is a
        # 32-bit userspace on these models, confirmed for TS-459 Pro.
        GOARCH="386"
        GOARM=""
        ;;
    arm-x19)
        # Marvell Kirkwood, ARMv5TE, softfloat.
        GOARCH="arm"
        GOARM="5"
        ;;
    arm-x31|arm-x41)
        # Marvell Armada (x31) / Annapurna Alpine dual-core (x41), ARMv7, hardfloat.
        GOARCH="arm"
        GOARM="7"
        ;;
    arm_64)
        # Annapurna Alpine 64-bit, aarch64.
        GOARCH="arm64"
        GOARM=""
        ;;
    *)
        echo "Unsupported ARCH: ${ARCH}. Use x86, arm-x19, arm-x31, arm-x41, or arm_64." >&2
        exit 2
        ;;
esac

command -v "${GO_BIN}" >/dev/null 2>&1 || {
    echo "Go tool not found: ${GO_BIN}" >&2
    exit 2
}
command -v "${QBUILD_BIN}" >/dev/null 2>&1 || {
    echo "qbuild not found: ${QBUILD_BIN}" >&2
    echo "Install QNAP's QDK first: https://github.com/qnap-dev/QDK" >&2
    exit 2
}

case "${VERSION}" in
    *[!0-9A-Za-z._-]*|"")
        echo "Invalid VERSION: ${VERSION}" >&2
        exit 2
        ;;
esac

echo "Running native Go tests..."
(
    cd "${NAS_DIR}"
    "${GO_BIN}" test ./...
)

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/pkgsender-qpkg.XXXXXX")"
trap 'rm -rf "${TMP_ROOT}"' EXIT HUP INT TERM

SRC="${TMP_ROOT}/src"
mkdir -p "${SRC}"
cp -R "${SCRIPT_DIR}/." "${SRC}/"
rm -rf "${SRC}/dist" "${SRC}/build"

# Only the requested arch directory should exist so qbuild produces exactly
# one .qpkg instead of trying to reason about the other empty placeholders.
for other in x86 arm-x19 arm-x31 arm-x41 arm_64; do
    if [ "${other}" != "${ARCH}" ]; then
        rm -rf "${SRC:?}/${other}"
    fi
done
# Present even if the repo tree this was copied from never committed a
# .gitkeep for this arch (git does not track empty directories).
mkdir -p "${SRC}/${ARCH}"
rm -f "${SRC}/${ARCH}/.gitkeep"

echo "Building linux/${GOARCH}${GOARM:+ (GOARM=${GOARM})} static binary for ${ARCH}..."
(
    cd "${NAS_DIR}"
    if [ -n "${GOARM}" ]; then
        CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" GOARM="${GOARM}" "${GO_BIN}" build \
            -trimpath -ldflags="-s -w" \
            -o "${SRC}/${ARCH}/pkg-sender-nas" ./cmd/pkg-sender-nas
    else
        CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" "${GO_BIN}" build \
            -trimpath -ldflags="-s -w" \
            -o "${SRC}/${ARCH}/pkg-sender-nas" ./cmd/pkg-sender-nas
    fi
)
chmod 755 "${SRC}/${ARCH}/pkg-sender-nas"

sed -e "s/@VERSION@/${VERSION}/g" "${SCRIPT_DIR}/qpkg.cfg" >"${SRC}/qpkg.cfg.tmp"
mv "${SRC}/qpkg.cfg.tmp" "${SRC}/qpkg.cfg"

mkdir -p "${OUT_DIR}"
(
    cd "${SRC}"
    "${QBUILD_BIN}"
)

if [ ! -d "${SRC}/build" ]; then
    echo "qbuild did not produce a build/ directory; check its output above." >&2
    exit 1
fi

COPIED=0
for f in "${SRC}"/build/*.qpkg "${SRC}"/build/*.qpkg.*; do
    [ -e "${f}" ] || continue
    cp "${f}" "${OUT_DIR}/"
    COPIED=1
done

if [ "${COPIED}" -eq 0 ]; then
    echo "No .qpkg output found under ${SRC}/build. qbuild's output layout may differ from what this script expects -- inspect it manually:" >&2
    ls -la "${SRC}/build" >&2 || true
    exit 1
fi

echo "Output written to ${OUT_DIR}"
ls "${OUT_DIR}"
