#!/bin/sh
# Installs task-view for the current user on macOS or Linux, without sudo:
# downloads the release binary for this system and CPU, checks it against
# the release's SHA256SUMS, puts it in ~/.local/bin and adds that folder to
# PATH when needed. Run it again to update.
#
#   curl -fsSL https://raw.githubusercontent.com/gtindo/task-view/main/scripts/install.sh | sh
#
# Environment:
#   TASK_VIEW_VERSION      release tag to install, e.g. v1.0.0 (default: latest)
#   TASK_VIEW_INSTALL_DIR  where to put the binary (default: ~/.local/bin)
#
# Kept to POSIX sh so it runs under dash, busybox, bash and zsh alike.
set -eu

version="${TASK_VIEW_VERSION:-latest}"
dir="${TASK_VIEW_INSTALL_DIR:-$HOME/.local/bin}"
repo="https://github.com/gtindo/task-view/releases"

case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "task-view: unsupported system $(uname -s); see $repo" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "task-view: unsupported CPU $(uname -m); see $repo" >&2; exit 1 ;;
esac

file="task-view-$os-$arch"
base="$repo/latest/download"
[ "$version" = latest ] || base="$repo/download/$version"

tmp="$(mktemp -d)"

# Removes only what this script creates, never a whole tree, on success and
# on failure alike.
cleanup() {
  rm -f "$tmp/$file" "$tmp/SHA256SUMS"
  rmdir "$tmp" 2>/dev/null || true
}
trap cleanup EXIT

echo "Downloading $file ($version)..."
curl -fsSL "$base/$file" -o "$tmp/$file"
curl -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"

# A mismatch stops the script here (set -e), before anything is installed.
cd "$tmp"
if command -v sha256sum >/dev/null 2>&1; then
  grep " $file\$" SHA256SUMS | sha256sum -c -
else
  grep " $file\$" SHA256SUMS | shasum -a 256 -c -
fi
cd - >/dev/null

mkdir -p "$dir"
mv "$tmp/$file" "$dir/task-view"
chmod 755 "$dir/task-view"

# A binary downloaded by a browser is quarantined by macOS Gatekeeper and
# refuses to start; clearing the flag is harmless when it is not set.
if [ "$os" = darwin ]; then
  xattr -d com.apple.quarantine "$dir/task-view" 2>/dev/null || true
fi

"$dir/task-view" version

case ":$PATH:" in
  *":$dir:"*)
    echo "Installed $dir/task-view"
    exit 0
    ;;
esac

case "${SHELL:-}" in
  */zsh) rc="$HOME/.zshrc" ;;
  */bash) [ "$os" = darwin ] && rc="$HOME/.bash_profile" || rc="$HOME/.bashrc" ;;
  *) rc="$HOME/.profile" ;;
esac

# Written once: a later run finds $dir in a new shell's PATH and stops above.
echo "export PATH=\"$dir:\$PATH\"" >> "$rc"
echo "Installed $dir/task-view and added $dir to PATH in $rc."
echo "Open a new terminal, or run: export PATH=\"$dir:\$PATH\""
