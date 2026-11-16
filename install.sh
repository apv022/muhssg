#!/bin/sh
# Install a checksum-verified muhssg release binary. No Go needed.
#
#   curl -fsSL https://raw.githubusercontent.com/apv022/muhssg/main/install.sh | sh -s -- --version v0.2.2
#
# Options (or environment variables):
#   --version vX.Y.Z   MUHSSG_VERSION   release to install (default: latest)
#   --dir DIR          MUHSSG_DIR       where to put the binary (default: ./bin)
#
# Pin a version on build hosts so every deploy uses the same binary.
set -eu

repo=apv022/muhssg
version=${MUHSSG_VERSION:-latest}
dir=${MUHSSG_DIR:-./bin}
# Where release files are downloaded from; overridable for testing.
base=${MUHSSG_DOWNLOAD_BASE:-https://github.com/$repo/releases/download}

say() { printf 'muhssg install: %s\n' "$*" >&2; }
die() { say "error: $*"; exit 1; }

while [ $# -gt 0 ]; do
  case $1 in
    --version) [ $# -ge 2 ] || die "--version needs a value"; version=$2; shift 2 ;;
    --version=*) version=${1#*=}; shift ;;
    --dir) [ $# -ge 2 ] || die "--dir needs a value"; dir=$2; shift 2 ;;
    --dir=*) dir=${1#*=}; shift ;;
    -h|--help) sed -n '2,10p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown option $1 (try --help)" ;;
  esac
done

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --retry 3 -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  die "curl or wget is required"
fi

case $(uname -s) in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS $(uname -s); on Windows download the .zip from https://github.com/$repo/releases" ;;
esac
case $(uname -m) in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported CPU $(uname -m)" ;;
esac

if [ "$version" = latest ]; then
  command -v curl >/dev/null 2>&1 || die "--version is required without curl"
  # github.com/<repo>/releases/latest redirects to .../releases/tag/<version>.
  url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") ||
    die "could not look up the latest release"
  version=${url##*/}
fi
case $version in
  v*) ;;
  *) version=v$version ;;
esac

name=muhssg_${version}_${os}_${arch}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "downloading $name"
fetch "$base/$version/$name.tar.gz" "$tmp/$name.tar.gz" || die "could not download $base/$version/$name.tar.gz"
fetch "$base/$version/checksums.txt" "$tmp/checksums.txt" || die "could not download the checksums for $version"

want=$(awk -v f="$name.tar.gz" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "$name.tar.gz is not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  got=$(sha256sum "$tmp/$name.tar.gz" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
  got=$(shasum -a 256 "$tmp/$name.tar.gz" | awk '{ print $1 }')
else
  die "sha256sum or shasum is required to verify the download"
fi
[ "$got" = "$want" ] || die "checksum mismatch for $name.tar.gz (expected $want, got $got)"

tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
mkdir -p "$dir"
cp "$tmp/$name/muhssg" "$dir/muhssg"
chmod 755 "$dir/muhssg"
say "installed $("$dir/muhssg" version 2>/dev/null || echo "$version") to $dir/muhssg"
