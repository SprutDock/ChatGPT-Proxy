#!/bin/sh
set -eu

root_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
dist_dir="$root_dir/dist"
stage_dir="$(mktemp -d)"
trap 'rm -rf "$stage_dir"' EXIT HUP INT TERM

mkdir -p "$dist_dir"
for platform in windows linux; do
    archive="$dist_dir/ChatGPT-Proxy-$platform.zip"
    rm -f "$archive"
    mkdir "$stage_dir/$platform"
    cp "$root_dir/$platform/README.md" "$stage_dir/$platform/README.md"
    if [ "$platform" = windows ]; then
        printf '\357\273\277' > "$stage_dir/$platform/Start-ChatGPT-Proxy.ps1"
        cat "$root_dir/windows/Start-ChatGPT-Proxy.ps1" >> "$stage_dir/$platform/Start-ChatGPT-Proxy.ps1"
        cp "$root_dir/proxy-url.txt" "$root_dir/windows/Start-ChatGPT-Proxy.cmd" "$root_dir/windows/Create-ChatGPT-Proxy-Shortcut.vbs" "$root_dir/assets/chatgpt.ico" "$stage_dir/$platform/"
    else
        cp "$root_dir/proxy-url.txt" "$root_dir/linux/launch.sh" "$root_dir/linux/install.sh" "$root_dir/assets/chatgpt.png" "$stage_dir/$platform/"
        chmod 755 "$stage_dir/$platform/launch.sh" "$stage_dir/$platform/install.sh"
    fi
    (cd "$stage_dir/$platform" && zip -q -r "$archive" .)
    rm -rf "$stage_dir/$platform"
    printf 'Created %s\n' "$archive"
done
