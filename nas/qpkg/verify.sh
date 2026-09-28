#!/bin/sh
set -eu

# Best-effort pre-install sanity checks for the produced .qpkg.
#
# Unlike nas/spk/verify.sh, this cannot fully introspect the archive: a QPKG
# built by qbuild is a self-extracting shell script whose container format
# was not independently verified here (no QNAP hardware or qbuild available
# to this build). Treat a PASS as "looks plausible", not "guaranteed to
# install cleanly" -- the real gate is a manual install on the target NAS.

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 path/to/PKGSenderNAS_*.qpkg" >&2
    exit 2
fi

QPKG="$1"
[ -f "${QPKG}" ] || {
    echo "QPKG not found: ${QPKG}" >&2
    exit 2
}
[ -x "${QPKG}" ] || {
    echo "QPKG is not marked executable (self-extracting installers must be): ${QPKG}" >&2
    exit 1
}

FIRST_LINE="$(head -n 1 "${QPKG}")"
case "${FIRST_LINE}" in
    '#!'*) ;;
    *)
        echo "QPKG does not start with a shell shebang; unexpected qbuild output format: ${FIRST_LINE}" >&2
        exit 1
        ;;
esac

for needle in "PKGSenderNAS"; do
    if ! grep -aq "${needle}" "${QPKG}"; then
        echo "QPKG does not appear to embed expected string: ${needle}" >&2
        exit 1
    fi
done

echo "Basic QPKG structure checks: PASS (best-effort only)."
echo "Next: enable unsigned-package installs in App Center and install on real hardware -- see README.md."
