#!/bin/sh
# Builds bin/bitdispatcher-VERSION-macos.pkg: one installer for Apple silicon and Intel Macs (macOS 12+).
#   sh packaging/macos/build.sh VERSION LDFLAGS
# Signing is optional and needs Apple Developer ID certificates in the keychain:
#   APP_SIGN="Developer ID Application: ..."  signs the program (hardened runtime) and the apps
#   PKG_SIGN="Developer ID Installer: ..."    signs the installer
#   NOTARY_PROFILE=name                       notarizes and staples it (xcrun notarytool store-credentials)
# Without them the installer is unsigned: macOS asks before opening it (Privacy & Security › Open Anyway).
set -eu
VER=$1
LDFLAGS=$2
GO=${GO:-go}
M=packaging/macos
OUT=bin/bitdispatcher-$VER-macos.pkg
W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
R=$W/root

# The program: one universal file (arm64 + amd64), FIPS build as on the other systems.
for a in arm64 amd64; do
  GOOS=darwin GOARCH=$a GOFIPS140=v1.0.0 CGO_ENABLED=0 "$GO" build -trimpath -ldflags "$LDFLAGS" -o "$W/bitdispatcher-$a" ./cmd/bitdispatcher
done
mkdir -p "$R/usr/local/bitdispatcher/bin" "$R/usr/local/bitdispatcher/share" "$R/Library/LaunchDaemons" "$R/Applications/bitdispatcher"
lipo -create -output "$R/usr/local/bitdispatcher/bin/bitdispatcher" "$W/bitdispatcher-arm64" "$W/bitdispatcher-amd64"
if [ -n "${APP_SIGN:-}" ]; then
  codesign --force --options runtime --timestamp -s "$APP_SIGN" "$R/usr/local/bitdispatcher/bin/bitdispatcher"
else
  codesign --force -s - "$R/usr/local/bitdispatcher/bin/bitdispatcher"
fi
install -m 755 $M/bitdispatcher-service $M/uninstall.sh $M/reset-admin.sh "$R/usr/local/bitdispatcher/bin/"
mv "$R/usr/local/bitdispatcher/bin/uninstall.sh" "$R/usr/local/bitdispatcher/bin/reset-admin.sh" "$R/usr/local/bitdispatcher/"
install -m 644 $M/bitdispatcher.conf LICENSE "$R/usr/local/bitdispatcher/share/"
install -m 644 $M/com.biodream.bitdispatcher.plist "$R/Library/LaunchDaemons/"

# The apps in Applications › bitdispatcher: AppleScript applets (macOS opens only real programs as apps),
# each with the icon, BioDream's identifier and the version, then signed again (ad hoc, or Developer ID).
app() { # name source bundle-id
  a="$R/Applications/bitdispatcher/$1.app"
  osacompile -o "$a" "$M/apps/$2.applescript"
  cp $M/bitdispatcher.icns "$a/Contents/Resources/applet.icns"
  rm -f "$a/Contents/Resources/Assets.car"
  p="$a/Contents/Info.plist"
  plutil -replace CFBundleIdentifier -string "$3" "$p"
  plutil -replace CFBundleName -string "$1" "$p"
  plutil -replace CFBundleShortVersionString -string "$VER" "$p"
  plutil -replace CFBundleVersion -string "$VER" "$p"
  plutil -replace LSMinimumSystemVersion -string 12.0 "$p"
  plutil -replace NSHumanReadableCopyright -string "Copyright BioDream LLC. Apache License 2.0." "$p"
  plutil -remove CFBundleIconName "$p" 2>/dev/null || true
  plutil -replace NSAppleEventsUsageDescription -string "bitdispatcher shows its dialogs." "$p"
  if [ -n "${APP_SIGN:-}" ]; then
    codesign --force --options runtime --timestamp -s "$APP_SIGN" "$a"
  else
    codesign --force -s - "$a"
  fi
}
app bitdispatcher open-console com.biodream.bitdispatcher.console
app "Reset admin password" reset-admin com.biodream.bitdispatcher.reset-admin
app "Uninstall bitdispatcher" uninstall com.biodream.bitdispatcher.uninstall
cp docs/manual/bitdispatcher-manual.pdf "$R/Applications/bitdispatcher/bitdispatcher manual.pdf"
xattr -cr "$R"

# Three component packages, each installed into a folder of its own, so Installer never touches the
# owner or permissions of /Applications, /Library or /usr/local; and a last one with only the script that
# sets the service up once every file is in place. The apps are not relocatable: Installer must never
# "upgrade" another copy of an app with the same identifier found elsewhere on the disk.
comp() { # identifier folder install-location [pkgbuild options]
  id=$1 dir=$2 loc=$3; shift 3
  pkgbuild --analyze --root "$dir" "$W/$id.plist" >/dev/null
  n=0
  while plutil -extract "$n" xml1 -o /dev/null "$W/$id.plist" 2>/dev/null; do
    plutil -replace "$n.BundleIsRelocatable" -bool NO "$W/$id.plist"
    n=$((n + 1))
  done
  pkgbuild --quiet --root "$dir" --component-plist "$W/$id.plist" --identifier "$id" --version "$VER" \
    --ownership recommended --install-location "$loc" "$@" "$W/pkgs/$id.pkg"
}
mkdir -p "$W/pkgs" "$W/pre" "$W/post"
install -m 755 $M/scripts/preinstall "$W/pre/preinstall"
install -m 755 $M/scripts/postinstall "$W/post/postinstall"
comp com.biodream.bitdispatcher "$R/usr/local/bitdispatcher" /usr/local/bitdispatcher --scripts "$W/pre"
comp com.biodream.bitdispatcher.launchd "$R/Library/LaunchDaemons" /Library/LaunchDaemons
comp com.biodream.bitdispatcher.apps "$R/Applications/bitdispatcher" /Applications/bitdispatcher
pkgbuild --quiet --nopayload --scripts "$W/post" --identifier com.biodream.bitdispatcher.setup --version "$VER" "$W/pkgs/com.biodream.bitdispatcher.setup.pkg"

# The installer the person opens: welcome, licence, install, conclusion, with the artwork.
mkdir -p "$W/res"
sed "s/VERSION/$VER/g" $M/resources/welcome.html > "$W/res/welcome.html"
cp $M/resources/conclusion.html $M/background.tiff $M/background-dark.tiff "$W/res/"
cp LICENSE "$W/res/LICENSE.txt"
sed "s/VERSION/$VER/g" $M/distribution.xml > "$W/distribution.xml"
mkdir -p bin
rm -f "$OUT"
if [ -n "${PKG_SIGN:-}" ]; then
  productbuild --quiet --distribution "$W/distribution.xml" --resources "$W/res" --package-path "$W/pkgs" --sign "$PKG_SIGN" --timestamp "$OUT"
else
  productbuild --quiet --distribution "$W/distribution.xml" --resources "$W/res" --package-path "$W/pkgs" "$OUT"
fi
if [ -n "${NOTARY_PROFILE:-}" ]; then
  xcrun notarytool submit "$OUT" --keychain-profile "$NOTARY_PROFILE" --wait
  xcrun stapler staple "$OUT"
fi
echo "built $OUT ($(du -h "$OUT" | cut -f1))"
