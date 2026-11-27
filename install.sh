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

in_path=0
case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) in_path=1 ;;
esac

linked_dir=""
if [ "$in_path" -eq 0 ]; then
  for candidate in /opt/homebrew/bin /usr/local/bin; do
    case ":${PATH:-}:" in
      *":$candidate:"*)
        if [ -d "$candidate" ] && [ -w "$candidate" ]; then
          if ln -sf "$INSTALL_DIR/agym" "$candidate/agym" 2>/dev/null; then
            linked_dir="$candidate"
            break
          fi
        fi
        ;;
    esac
  done

  shell_name="$(basename "${SHELL:-}")"
  rc_file=""
  path_line="export PATH=\"$INSTALL_DIR:\$PATH\""
  case "$shell_name" in
    zsh)  rc_file="$HOME/.zshrc" ;;
    bash)
      if [ -f "$HOME/.bash_profile" ]; then
        rc_file="$HOME/.bash_profile"
      else
        rc_file="$HOME/.bashrc"
      fi
      ;;
    fish)
      rc_file="$HOME/.config/fish/config.fish"
      path_line="fish_add_path \"$INSTALL_DIR\""
      ;;
    *)    rc_file="$HOME/.profile" ;;
  esac

  if [ -n "$rc_file" ]; then
    mkdir -p "$(dirname "$rc_file")"
    if [ ! -f "$rc_file" ] || ! grep -qsF "$INSTALL_DIR" "$rc_file"; then
      printf '\n# Added by agym installer\n%s\n' "$path_line" >> "$rc_file"
      echo "Added $INSTALL_DIR to PATH in $rc_file"
    fi
  fi

  if [ -n "$linked_dir" ]; then
    echo "Linked $linked_dir/agym -> $INSTALL_DIR/agym (ready in current shell)."
  elif [ -n "$rc_file" ]; then
    echo "Restart your terminal or run 'source $rc_file' to use 'agym' in this shell."
  fi
fi

echo "Run 'agym' (or 'agym ui') to launch the menu bar UI, or 'agym --help' for CLI usage."
echo "Optional: run 'agym install-hook' only if you want automatic account handoff during 'agym supervise' sessions."
