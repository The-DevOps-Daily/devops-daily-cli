#!/bin/sh
# Install DevOps Daily without telemetry or elevated privileges.
set -eu

repo="The-DevOps-Daily/devops-daily-cli"
base_url="https://github.com/$repo/releases/download"
latest_url="https://github.com/$repo/releases/latest"
dry_run=false

if [ "${1-}" = "--dry-run" ]; then
  dry_run=true
elif [ "${1-}" != "" ]; then
  echo "Usage: install.sh [--dry-run]" >&2
  exit 2
fi

case "$(uname -s)" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  MINGW*|MSYS*|CYGWIN*)
    echo "Windows is not supported by this installer. Use npm or the Windows zip release." >&2
    exit 1
    ;;
  *)
    echo "Unsupported operating system: $(uname -s)" >&2
    exit 1
    ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "Unsupported architecture: $(uname -m)" >&2
    exit 1
    ;;
esac

if command -v curl >/dev/null 2>&1; then
  fetch() {
    curl -fsSL "$1" -o "$2"
  }
  latest_version() {
    curl -fsSL -o /dev/null -w '%{url_effective}' "$latest_url" | sed 's#.*/tag/v##'
  }
elif command -v wget >/dev/null 2>&1; then
  fetch() {
    wget -qO "$2" "$1"
  }
  latest_version() {
    latest_page="$temp_dir/latest-release.html"
    wget -qO "$latest_page" "$latest_url"
    sed -n 's#.*releases/tag/v\([^"?<>]*\).*#\1#p' "$latest_page" | sed -n '1p'
  }
else
  echo "curl or wget is required to install devops-daily." >&2
  exit 1
fi

temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/devops-daily.XXXXXX")
trap 'rm -rf "$temp_dir"' EXIT HUP INT TERM

version=${DEVOPS_DAILY_VERSION-}
if [ -z "$version" ]; then
  echo "Finding the latest release..."
  version=$(latest_version)
fi
version=${version#v}
case "$version" in
  [0-9]*) ;;
  *)
    echo "Could not find a published release. See https://github.com/$repo/releases" >&2
    exit 1
    ;;
esac

archive="devops-daily_${version}_${os}_${arch}.tar.gz"
archive_path="$temp_dir/$archive"
checksums_path="$temp_dir/checksums.txt"

echo "Downloading devops-daily v$version for $os/$arch..."
fetch "$base_url/v$version/$archive" "$archive_path"
fetch "$base_url/v$version/checksums.txt" "$checksums_path"

expected_checksum=$(awk -v file="$archive" '
  { name = $2; sub(/^\*/, "", name); if (name == file) { print $1; exit } }
' "$checksums_path")
if [ -z "$expected_checksum" ]; then
  echo "No checksum found for $archive." >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_checksum=$(sha256sum "$archive_path" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual_checksum=$(shasum -a 256 "$archive_path" | awk '{print $1}')
else
  echo "sha256sum or shasum is required to verify the download." >&2
  exit 1
fi

if [ "$actual_checksum" != "$expected_checksum" ]; then
  echo "Checksum verification failed for $archive. The file was not installed." >&2
  exit 1
fi
echo "Checksum verified."

extract_dir="$temp_dir/extract"
mkdir -p "$extract_dir"
tar -xzf "$archive_path" -C "$extract_dir"
binary_path=$(find "$extract_dir" -type f -name devops-daily -print | sed -n '1p')
if [ -z "$binary_path" ]; then
  echo "The archive did not contain devops-daily." >&2
  exit 1
fi

if [ -n "${DEVOPS_DAILY_INSTALL_DIR-}" ]; then
  install_dir=$DEVOPS_DAILY_INSTALL_DIR
elif [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
  install_dir=/usr/local/bin
else
  install_dir="${HOME}/.local/bin"
fi

if [ ! -d "$install_dir" ]; then
  if ! mkdir -p "$install_dir" 2>/dev/null; then
    echo "Cannot create $install_dir without elevated privileges." >&2
    echo "Run this installer with DEVOPS_DAILY_INSTALL_DIR set to a writable directory." >&2
    echo "If you manage this directory, use: sudo mkdir -p $install_dir" >&2
    exit 1
  fi
fi
if [ ! -w "$install_dir" ]; then
  echo "Cannot write to $install_dir without elevated privileges." >&2
  echo "Run this installer with DEVOPS_DAILY_INSTALL_DIR set to a writable directory." >&2
  echo "If you manage this directory, use: sudo install -m 755 $binary_path $install_dir/devops-daily" >&2
  exit 1
fi

if [ "$dry_run" = true ]; then
  echo "Would install devops-daily v$version to $install_dir/devops-daily."
  exit 0
fi

install -m 755 "$binary_path" "$install_dir/devops-daily"
alias_path="$install_dir/ddy"
if [ -e "$alias_path" ] || [ -L "$alias_path" ]; then
  if [ "$(readlink "$alias_path" 2>/dev/null || true)" = "devops-daily" ]; then
    rm -f "$alias_path"
    ln -s devops-daily "$alias_path"
  else
    echo "Not changing $alias_path because it already belongs to something else."
  fi
else
  ln -s devops-daily "$alias_path"
fi

echo "Installed devops-daily v$version to $install_dir/devops-daily."
if command -v gh >/dev/null 2>&1; then
  echo "To verify release provenance: gh attestation verify <file> -R $repo"
fi
case ":${PATH-}:" in
  *":$install_dir:"*) ;;
  *) echo "Add $install_dir to your PATH to run devops-daily." ;;
esac
