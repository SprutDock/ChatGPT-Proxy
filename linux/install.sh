#!/usr/bin/env bash
set -euo pipefail

source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
launcher_dir="$data_home/chatgpt-proxy-launcher"
applications_dir="$data_home/applications"

install -Dm755 "$source_dir/launch.sh" "$launcher_dir/launch.sh"
install -Dm644 "$source_dir/chatgpt.png" "$launcher_dir/chatgpt.png"
install -Dm644 "$source_dir/proxy-url.txt" "$launcher_dir/proxy-url.txt"
mkdir -p "$applications_dir"
escaped_launcher="$(printf '%s' "$launcher_dir/launch.sh" | sed 's/\\/\\\\/g; s/"/\\"/g')"

cat > "$applications_dir/chatgpt-through-proxy.desktop" <<EOF
[Desktop Entry]
Version=1.0
Type=Application
Name=ChatGPT Proxy
Comment=Запуск ChatGPT Desktop через HTTP proxy
Exec=/bin/bash "$escaped_launcher"
Icon=$launcher_dir/chatgpt.png
Terminal=false
Categories=Network;Utility;
EOF

chmod 644 "$applications_dir/chatgpt-through-proxy.desktop"
printf 'Добавлен launcher в меню приложений: %s\n' "$applications_dir/chatgpt-through-proxy.desktop"
