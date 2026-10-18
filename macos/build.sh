#!/bin/zsh
# Builds Pisolo.app (universal: arm64 + x86_64) into ./build
set -e
cd "$(dirname "$0")"
APP=build/Pisolo.app
rm -rf build && mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
for arch in arm64 x86_64; do
  swiftc -O -target $arch-apple-macos13.0 Sources/*.swift -o build/Pisolo-$arch
done
lipo -create build/Pisolo-arm64 build/Pisolo-x86_64 -output "$APP/Contents/MacOS/Pisolo"
rm build/Pisolo-*
cp Resources/Info.plist "$APP/Contents/"
codesign --force --sign "${SIGN_ID:--}" --options runtime "$APP"
ditto -c -k --keepParent "$APP" build/Pisolo-macOS.zip
echo "Built $APP and build/Pisolo-macOS.zip"
