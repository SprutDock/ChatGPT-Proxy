#!/bin/zsh
set -euo pipefail

ROOT="${0:A:h}"
CERT_AGENT_DIR="$ROOT/../ChatGPT-Proxy-Cert-Agent"
DIST="$ROOT/dist"
APP="$DIST/ChatGPT Proxy.app"
ARCHIVE="$DIST/ChatGPT-Proxy-macOS.zip"
STAGE_DIR="$(/usr/bin/mktemp -d)"
STAGE_APP="$STAGE_DIR/ChatGPT Proxy.app"
STAGE_ARCHIVE="$STAGE_DIR/ChatGPT-Proxy-macOS.zip"
STUNNEL_VERSION="${SPRU_STUNNEL_VERSION:-5.82}"
PRODUCTION_BUILD="${SPRU_PRODUCTION_BUILD:-0}"
BUILD_ARCH="${SPRU_BUILD_ARCH:-universal}"
trap '/bin/rm -rf "$STAGE_DIR"' EXIT HUP INT TERM

ARCHES=(arm64)
GO_ARCHES=(arm64)
if [[ "$BUILD_ARCH" == universal ]]; then
    ARCHES+=(x86_64)
    GO_ARCHES+=(amd64)
elif [[ "$BUILD_ARCH" != arm64 ]]; then
    print -u2 "SPRU_BUILD_ARCH must be arm64 or universal"
    exit 1
fi

: "${SPRU_CLIENT_CA_CHAIN_FILE:?Set the production client CA chain file}"
: "${SPRU_SERVER_CA_FILE:?Set the public server CA file}"
: "${SPRU_STUNNEL_BINARY:?Set a verified stunnel binary}"
: "${SPRU_OPENSSL_LIB_DIR:?Set the directory containing the matching OpenSSL dylibs}"
[[ -r "$SPRU_CLIENT_CA_CHAIN_FILE" && -r "$SPRU_SERVER_CA_FILE" && -x "$SPRU_STUNNEL_BINARY" ]] || { print -u2 "A certificate bundle or stunnel input is unavailable"; exit 1; }
[[ -r "$SPRU_OPENSSL_LIB_DIR/libssl.4.dylib" && -r "$SPRU_OPENSSL_LIB_DIR/libcrypto.4.dylib" ]] || { print -u2 "The matching OpenSSL 4 dylibs are unavailable"; exit 1; }
[[ -f "$CERT_AGENT_DIR/go.mod" ]] || { print -u2 "The cert-agent source is unavailable"; exit 1; }

if ! /usr/bin/lipo "$SPRU_STUNNEL_BINARY" -verify_arch "${ARCHES[@]}"; then
    print -u2 "stunnel must support $BUILD_ARCH"
    exit 1
fi
for library in libssl.4.dylib libcrypto.4.dylib; do
    if ! /usr/bin/lipo "$SPRU_OPENSSL_LIB_DIR/$library" -verify_arch "${ARCHES[@]}"; then
        print -u2 "$library must support $BUILD_ARCH"
        exit 1
    fi
done

mkdir -p "$STAGE_APP/Contents/MacOS" "$STAGE_APP/Contents/Resources" "$DIST"
cp "$ROOT/Info.plist" "$STAGE_APP/Contents/Info.plist"
cp "$ROOT/assets/chatgpt.icns" "$STAGE_APP/Contents/Resources/chatgpt.icns"
cp "$ROOT/macos-proxy-url.txt" "$STAGE_APP/Contents/Resources/macos-proxy-url.txt"
cp "$ROOT/launch.sh" "$STAGE_APP/Contents/Resources/launch.sh"
cp "$SPRU_CLIENT_CA_CHAIN_FILE" "$STAGE_APP/Contents/Resources/client-ca-chain.pem"
cp "$SPRU_SERVER_CA_FILE" "$STAGE_APP/Contents/Resources/server-ca.pem"
cp "$SPRU_STUNNEL_BINARY" "$STAGE_APP/Contents/Resources/stunnel"
cp "$SPRU_OPENSSL_LIB_DIR/libssl.4.dylib" "$STAGE_APP/Contents/Resources/libssl.4.dylib"
cp "$SPRU_OPENSSL_LIB_DIR/libcrypto.4.dylib" "$STAGE_APP/Contents/Resources/libcrypto.4.dylib"
chmod 755 "$STAGE_APP/Contents/Resources/launch.sh"
chmod 644 "$STAGE_APP/Contents/Resources/client-ca-chain.pem" "$STAGE_APP/Contents/Resources/server-ca.pem"
chmod 755 "$STAGE_APP/Contents/Resources/stunnel"

for binding in stunnel:libssl.4.dylib stunnel:libcrypto.4.dylib libssl.4.dylib:libcrypto.4.dylib; do
    binary="${binding%%:*}"
    library="${binding#*:}"
    binary_path="$STAGE_APP/Contents/Resources/$binary"
    for architecture in "${ARCHES[@]}"; do
        linked_path="$(/usr/bin/otool -L -arch "$architecture" "$binary_path" | /usr/bin/awk -v library="$library" '$1 ~ "/" library "$" { print $1; exit }')"
        [[ -n "$linked_path" ]] || { print -u2 "$binary is not linked to $library for $architecture"; exit 1; }
        if [[ "$linked_path" != "@executable_path/$library" ]]; then
            /usr/bin/install_name_tool -change "$linked_path" "@executable_path/$library" "$binary_path"
        fi
        linked_path="$(/usr/bin/otool -L -arch "$architecture" "$binary_path" | /usr/bin/awk -v library="$library" '$1 ~ "/" library "$" { print $1; exit }')"
        [[ "$linked_path" == "@executable_path/$library" ]] || { print -u2 "$binary still needs an external $library for $architecture"; exit 1; }
    done
done
for resource in libcrypto.4.dylib libssl.4.dylib stunnel; do
    codesign --force --sign - "$STAGE_APP/Contents/Resources/$resource"
done
if ! STAGED_STUNNEL_INFO="$("$STAGE_APP/Contents/Resources/stunnel" -version 2>&1)"; then
    print -u2 "The bundled stunnel could not start"
    exit 1
fi
[[ "$STAGED_STUNNEL_INFO" == *"stunnel $STUNNEL_VERSION "* ]] || { print -u2 "Expected stunnel $STUNNEL_VERSION"; exit 1; }
[[ "$STAGED_STUNNEL_INFO" != *"Update OpenSSL shared libraries"* && "$STAGED_STUNNEL_INFO" != *"rebuild stunnel"* ]] || { print -u2 "The bundled stunnel and OpenSSL runtime do not match"; exit 1; }

for architecture in "${GO_ARCHES[@]}"; do
	GOOS=darwin GOARCH="$architecture" CGO_ENABLED=0 go -C "$CERT_AGENT_DIR" build -trimpath -ldflags='-s -w' -o "$STAGE_DIR/cert-agent-$architecture" .
done
if [[ "$BUILD_ARCH" == arm64 ]]; then
    cp "$STAGE_DIR/cert-agent-arm64" "$STAGE_APP/Contents/Resources/cert-agent"
else
    /usr/bin/lipo -create "$STAGE_DIR/cert-agent-arm64" "$STAGE_DIR/cert-agent-amd64" -output "$STAGE_APP/Contents/Resources/cert-agent"
fi
chmod 755 "$STAGE_APP/Contents/Resources/cert-agent"
CLANG_ARCHES=()
for architecture in "${ARCHES[@]}"; do
    CLANG_ARCHES+=(-arch "$architecture")
done
clang -fobjc-arc -framework AppKit -mmacosx-version-min=13.0 "${CLANG_ARCHES[@]}" "$ROOT/Launcher.m" -o "$STAGE_APP/Contents/MacOS/ChatGPTProxyLauncher"

if [[ "$PRODUCTION_BUILD" == 1 ]]; then
    : "${SPRU_DEVELOPER_ID:?Set the Developer ID Application identity}"
    : "${SPRU_NOTARY_PROFILE:?Set the notarytool Keychain profile}"
    for resource in libcrypto.4.dylib libssl.4.dylib stunnel cert-agent; do
        codesign --force --options runtime --timestamp --sign "$SPRU_DEVELOPER_ID" "$STAGE_APP/Contents/Resources/$resource"
    done
    codesign --force --deep --options runtime --timestamp --sign "$SPRU_DEVELOPER_ID" "$STAGE_APP"
else
    codesign --force --sign - "$STAGE_APP/Contents/Resources/cert-agent"
    codesign --force --deep --sign - "$STAGE_APP"
fi
plutil -lint "$STAGE_APP/Contents/Info.plist"
codesign --verify --deep --strict "$STAGE_APP"
ditto -c -k --sequesterRsrc --keepParent "$STAGE_APP" "$STAGE_ARCHIVE"

if [[ "$PRODUCTION_BUILD" == 1 ]]; then
    xcrun notarytool submit "$STAGE_ARCHIVE" --keychain-profile "$SPRU_NOTARY_PROFILE" --wait
    xcrun stapler staple "$STAGE_APP"
    /bin/rm -f "$STAGE_ARCHIVE"
    ditto -c -k --sequesterRsrc --keepParent "$STAGE_APP" "$STAGE_ARCHIVE"
fi

/bin/rm -rf "$APP"
/bin/mv "$STAGE_APP" "$APP"
/bin/mv -f "$STAGE_ARCHIVE" "$ARCHIVE"
print "Built $APP"
print "Created $ARCHIVE"
