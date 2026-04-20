#!/usr/bin/env sh
# scaffor install script — userspace, no root needed.
#
# Downloads the right release archive for your OS/arch from GitHub and drops
# the binary into BINDIR (default: ~/.local/bin).
#
# Quick install (latest release):
#   curl -sSL https://raw.githubusercontent.com/JLugagne/scaffor/main/install.sh | sh
#
# Pick a specific version:
#   curl -sSL https://raw.githubusercontent.com/JLugagne/scaffor/main/install.sh | VERSION=v0.5.0 sh
#
# Install somewhere else:
#   curl -sSL https://raw.githubusercontent.com/JLugagne/scaffor/main/install.sh | BINDIR=$HOME/bin sh

set -eu

OWNER="JLugagne"
REPO="scaffor"
BIN_NAME="scaffor"

# User-facing knobs.
VERSION="${VERSION:-latest}"
BINDIR="${BINDIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}"

# --- helpers ---------------------------------------------------------------

log()  { printf "\033[1;94m==>\033[0m %s\n" "$*"; }
warn() { printf "\033[1;93m==>\033[0m %s\n" "$*" >&2; }
err()  { printf "\033[1;91mError:\033[0m %s\n" "$*" >&2; exit 1; }

have() { command -v "$1" >/dev/null 2>&1; }

# Refuse to run as root: this script is for userspace installs only.
if [ "$(id -u 2>/dev/null || echo 0)" = "0" ]; then
  err "do not run as root; this installer targets your user account only.
    Re-run as a regular user. Installs to \$HOME/.local/bin (or \$BINDIR)."
fi

# detect_os prints one of: linux, darwin, windows.
detect_os() {
  uname_s=$(uname -s 2>/dev/null || echo unknown)
  case "$uname_s" in
    Linux)              echo linux ;;
    Darwin)             echo darwin ;;
    MINGW*|MSYS*|CYGWIN*) echo windows ;;
    *) err "unsupported OS: $uname_s" ;;
  esac
}

# detect_arch prints one of: amd64, arm64.
detect_arch() {
  uname_m=$(uname -m 2>/dev/null || echo unknown)
  case "$uname_m" in
    x86_64|amd64)   echo amd64 ;;
    aarch64|arm64)  echo arm64 ;;
    *) err "unsupported architecture: $uname_m" ;;
  esac
}

# resolve_version turns "latest" into the actual tag via the GitHub API.
# Falls back to parsing the redirect when curl/wget can hit the releases URL.
resolve_version() {
  if [ "$VERSION" != "latest" ]; then
    echo "$VERSION"
    return
  fi
  api_url="https://api.github.com/repos/$OWNER/$REPO/releases/latest"
  if have curl; then
    tag=$(curl -fsSL "$api_url" 2>/dev/null | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  elif have wget; then
    tag=$(wget -qO- "$api_url" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
  else
    err "neither curl nor wget found; install one and retry"
  fi
  if [ -z "$tag" ]; then
    err "could not resolve latest version from $api_url
    Tip: set VERSION=vX.Y.Z explicitly"
  fi
  echo "$tag"
}

# download fetches URL to FILE.
download() {
  _url="$1"
  _out="$2"
  if have curl; then
    curl -fsSL -o "$_out" "$_url" || err "failed to download $_url"
  elif have wget; then
    wget -qO "$_out" "$_url" || err "failed to download $_url"
  else
    err "neither curl nor wget found; install one and retry"
  fi
}

# verify_checksum compares <file> against <checksums.txt> (sha256).
# Skipped silently when no sha256 tool is available — the HTTPS transport
# still protects against MITM.
verify_checksum() {
  _archive="$1"
  _sums="$2"
  _name=$(basename "$_archive")
  _expected=$(grep "  $_name$" "$_sums" | awk '{print $1}' | head -n1)
  if [ -z "$_expected" ]; then
    warn "no checksum entry for $_name in checksums.txt; skipping verification"
    return
  fi
  if have sha256sum; then
    _got=$(sha256sum "$_archive" | awk '{print $1}')
  elif have shasum; then
    _got=$(shasum -a 256 "$_archive" | awk '{print $1}')
  else
    warn "no sha256 tool available; skipping checksum verification"
    return
  fi
  if [ "$_got" != "$_expected" ]; then
    err "checksum mismatch for $_name
    expected: $_expected
    got:      $_got"
  fi
  log "checksum OK"
}

# extract unpacks <archive> into <destdir>.
extract() {
  _archive="$1"
  _dest="$2"
  case "$_archive" in
    *.tar.gz|*.tgz)
      have tar || err "tar not found"
      tar -xzf "$_archive" -C "$_dest"
      ;;
    *.zip)
      have unzip || err "unzip not found (required to install Windows builds)"
      unzip -q "$_archive" -d "$_dest"
      ;;
    *) err "unknown archive format: $_archive" ;;
  esac
}

# --- main ------------------------------------------------------------------

OS=$(detect_os)
ARCH=$(detect_arch)
TAG=$(resolve_version)
# Strip leading "v" for the archive name; goreleaser uses the bare version.
VER_BARE=${TAG#v}

case "$OS" in
  windows) EXT=zip;    BIN_FILE="$BIN_NAME.exe" ;;
  *)       EXT=tar.gz; BIN_FILE="$BIN_NAME"     ;;
esac

ARCHIVE="${REPO}_${VER_BARE}_${OS}_${ARCH}.${EXT}"
ARCHIVE_URL="https://github.com/$OWNER/$REPO/releases/download/$TAG/$ARCHIVE"
SUMS_URL="https://github.com/$OWNER/$REPO/releases/download/$TAG/checksums.txt"

log "Installing $BIN_NAME $TAG for $OS/$ARCH"
log "Target: $BINDIR/$BIN_FILE"

mkdir -p "$BINDIR" || err "cannot create $BINDIR"

TMPDIR=$(mktemp -d 2>/dev/null || mktemp -d -t "${BIN_NAME}-install") \
  || err "cannot create temp directory"
trap 'rm -rf "$TMPDIR"' EXIT INT TERM

log "Downloading $ARCHIVE"
download "$ARCHIVE_URL" "$TMPDIR/$ARCHIVE"

log "Downloading checksums.txt"
download "$SUMS_URL" "$TMPDIR/checksums.txt"

verify_checksum "$TMPDIR/$ARCHIVE" "$TMPDIR/checksums.txt"

log "Extracting"
extract "$TMPDIR/$ARCHIVE" "$TMPDIR"

if [ ! -f "$TMPDIR/$BIN_FILE" ]; then
  err "binary $BIN_FILE not found in archive — report this at https://github.com/$OWNER/$REPO/issues"
fi

install -m 0755 "$TMPDIR/$BIN_FILE" "$BINDIR/$BIN_FILE" 2>/dev/null \
  || { cp "$TMPDIR/$BIN_FILE" "$BINDIR/$BIN_FILE" && chmod 0755 "$BINDIR/$BIN_FILE"; }

log "Installed $BIN_FILE → $BINDIR/$BIN_FILE"

# PATH hint — only if the chosen BINDIR isn't already on PATH.
case ":$PATH:" in
  *":$BINDIR:"*) : ;;
  *)
    warn "$BINDIR is not on your \$PATH."
    printf "    Add this line to your shell rc (~/.bashrc, ~/.zshrc, ~/.config/fish/config.fish, …):\n\n"
    printf "        export PATH=\"%s:\$PATH\"\n\n" "$BINDIR"
    printf "    Then reload your shell (or open a new terminal).\n"
    ;;
esac

log "Done. Run: $BIN_NAME --help"
