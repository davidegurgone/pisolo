#!/bin/sh
# Builds Pisolo for Windows and Linux (amd64 + arm64) into ./dist
set -e
cd "$(dirname "$0")"
rm -rf dist && mkdir -p dist
export CGO_ENABLED=0
for arch in amd64 arm64; do
  GOOS=windows GOARCH=$arch go build -trimpath -ldflags "-s -w -H windowsgui" -o dist/pisolo-windows-$arch/pisolo.exe .
  GOOS=windows GOARCH=$arch go build -trimpath -ldflags "-s -w" -o dist/pisolo-windows-$arch/pisolo-cli.exe .
  GOOS=linux   GOARCH=$arch go build -trimpath -ldflags "-s -w" -o dist/pisolo-linux-$arch/pisolo .
done
cp ../LICENSE dist/ 2>/dev/null || true
cd dist
for d in pisolo-*; do
  cp LICENSE "$d/"
  case "$d" in
    *windows*) (cd "$d" && zip -q -r "../$d.zip" .) ;;
    *)         tar -czf "$d.tar.gz" "$d" ;;
  esac
done
ls -lh *.zip *.tar.gz
