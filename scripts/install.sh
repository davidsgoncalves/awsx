#!/bin/sh
# Install script for awsx. Detects OS/arch, downloads the matching release
# archive from GitHub, verifies its checksum, and installs the binary.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/davidsgoncalves/awsx/main/scripts/install.sh | sh

set -eu

REPO="davidsgoncalves/awsx"
BINARY="awsx"

info() { printf '%s\n' "$*"; }
err() { printf 'error: %s\n' "$*" >&2; }

require() {
	if ! command -v "$1" >/dev/null 2>&1; then
		err "'$1' is required but was not found on your PATH."
		exit 1
	fi
}

require curl
require tar

# Detect OS (matches the release artifact naming: Darwin / Linux).
os="$(uname -s)"
case "$os" in
	Darwin) os="Darwin" ;;
	Linux) os="Linux" ;;
	*)
		err "unsupported OS: $os (awsx supports macOS and Linux)"
		exit 1
		;;
esac

# Detect architecture (matches the release artifact naming: x86_64 / arm64).
arch="$(uname -m)"
case "$arch" in
	x86_64 | amd64) arch="x86_64" ;;
	arm64 | aarch64) arch="arm64" ;;
	*)
		err "unsupported architecture: $arch"
		exit 1
		;;
esac

# Resolve the latest release tag via the GitHub API.
info "Resolving latest awsx release..."
tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
	| grep '"tag_name":' \
	| head -n 1 \
	| sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"

if [ -z "$tag" ]; then
	err "could not determine the latest release tag."
	exit 1
fi

archive="${BINARY}_${os}_${arch}.tar.gz"
base_url="https://github.com/${REPO}/releases/download/${tag}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

info "Downloading ${archive} (${tag})..."
curl -fsSL "${base_url}/${archive}" -o "${tmp}/${archive}"
curl -fsSL "${base_url}/checksums.txt" -o "${tmp}/checksums.txt"

# Verify the checksum.
info "Verifying checksum..."
expected="$(grep " ${archive}\$" "${tmp}/checksums.txt" | awk '{print $1}')"
if [ -z "$expected" ]; then
	err "checksum for ${archive} not found in checksums.txt"
	exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
	actual="$(sha256sum "${tmp}/${archive}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
	actual="$(shasum -a 256 "${tmp}/${archive}" | awk '{print $1}')"
else
	err "neither sha256sum nor shasum is available to verify the download."
	exit 1
fi

if [ "$expected" != "$actual" ]; then
	err "checksum mismatch for ${archive}"
	err "  expected: ${expected}"
	err "  actual:   ${actual}"
	exit 1
fi

# Extract the binary.
tar -xzf "${tmp}/${archive}" -C "$tmp"
if [ ! -f "${tmp}/${BINARY}" ]; then
	err "binary '${BINARY}' not found in the archive."
	exit 1
fi
chmod +x "${tmp}/${BINARY}"

# Choose an install directory: /usr/local/bin if writable, else ~/.local/bin.
if [ -w /usr/local/bin ] 2>/dev/null; then
	dest="/usr/local/bin"
else
	dest="${HOME}/.local/bin"
	mkdir -p "$dest"
fi

mv "${tmp}/${BINARY}" "${dest}/${BINARY}"
info "Installed ${BINARY} ${tag} to ${dest}/${BINARY}"

case ":${PATH}:" in
	*":${dest}:"*) ;;
	*) info "Note: ${dest} is not on your PATH. Add it to use '${BINARY}' directly." ;;
esac
