#!/bin/sh
set -eu

REPO="GongJS/agym"
INSTALL_DIR="${AGYM_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${AGYM_VERSION:-}"

os="$(uname -s)"
if [ "$os" != "Darwin" ]; then
  echo "error: agym currently supports macOS only (detected: $os)" >&2
  exit 1
fi

arch_raw="$(uname -m)"
case "$arch_raw" in
  arm64|aarch64) arch="arm64" ;;
  x86_64)        arch="amd64" ;;
  *)
    echo "error: unsupported macOS architecture: $arch_raw" >&2
    exit 1
    ;;
esac

if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$VERSION" ]; then
    echo "error: failed to resolve latest release version from GitHub" >&2
    exit 1
  fi
fi

ver_no_v="${VERSION#v}"
archive="agym_${ver_no_v}_darwin_${arch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$VERSION"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "Downloading agym $VERSION (darwin/$arch)..."
curl -fsSL "$base_url/$archive" -o "$tmpdir/$archive"
curl -fsSL "$base_url/checksums.txt" -o "$tmpdir/checksums.txt"

(
  cd "$tmpdir"
  grep "  $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null
)

tar -C "$tmpdir" -xzf "$tmpdir/$archive" agym
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/agym" "$INSTALL_DIR/agym"

echo "Installed agym to $INSTALL_DIR/agym"
case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo "Note: add $INSTALL_DIR to your PATH, e.g.:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac
echo "Run 'agym' (or 'agym tray') to launch the menu bar app, or 'agym --help' for CLI usage."
echo "Optional: run 'agym install-hook' only if you want automatic account handoff during 'agym supervise' sessions."
