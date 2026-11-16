#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
codex_home="${CODEX_HOME:-$HOME/.codex}"
env_file="$codex_home/.env"
proxy_url_file="$script_dir/proxy-url.txt"

fail() {
    local message="$1"
    if command -v zenity >/dev/null 2>&1; then
        zenity --error --title='ChatGPT Proxy' --text="$message" >/dev/null 2>&1 || true
    elif command -v kdialog >/dev/null 2>&1; then
        kdialog --error "$message" --title 'ChatGPT Proxy' >/dev/null 2>&1 || true
    elif command -v notify-send >/dev/null 2>&1; then
        notify-send --urgency=critical 'ChatGPT Proxy' "$message" >/dev/null 2>&1 || true
    else
        printf 'ChatGPT Proxy: %s\n' "$message" >&2
    fi
    exit 1
}

[[ -r "$proxy_url_file" ]] || fail "Не найден файл $proxy_url_file"
default_proxy="$(tr -d '\r\n' < "$proxy_url_file")"
[[ "$default_proxy" == http://* ]] || fail "Некорректный proxy URL в $proxy_url_file"

chatgpt="$(command -v chatgpt || true)"
[[ -n "$chatgpt" ]] || fail 'Не найдена команда chatgpt. Установите ChatGPT Desktop и повторите запуск.'

umask 077
mkdir -p "$codex_home"
[[ -e "$env_file" ]] || : > "$env_file"
unset HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy || true

while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%$'\r'}"
    [[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
    if [[ "$line" =~ ^[[:space:]]*(export[[:space:]]+)?(HTTP_PROXY|HTTPS_PROXY|NO_PROXY|http_proxy|https_proxy|no_proxy)[[:space:]]*=[[:space:]]*(.*)$ ]]; then
        name="${BASH_REMATCH[2]}"
        value="${BASH_REMATCH[3]}"
        value="${value%$'\r'}"
        if [[ "$value" == \"*\" && "$value" == *\" ]]; then
            value="${value:1:${#value}-2}"
        elif [[ "$value" == \'*\' && "$value" == *\' ]]; then
            value="${value:1:${#value}-2}"
        fi
        export "$name=$value"
    fi
done < "$env_file"

HTTP_PROXY="${HTTP_PROXY:-${http_proxy:-}}"
HTTPS_PROXY="${HTTPS_PROXY:-${https_proxy:-}}"
if [[ -z "$HTTP_PROXY" ]]; then
    HTTP_PROXY="${HTTPS_PROXY:-$default_proxy}"
    printf 'HTTP_PROXY=%s\n' "$HTTP_PROXY" >> "$env_file"
fi
if [[ -z "$HTTPS_PROXY" ]]; then
    HTTPS_PROXY="$HTTP_PROXY"
    printf 'HTTPS_PROXY=%s\n' "$HTTPS_PROXY" >> "$env_file"
fi

export HTTP_PROXY HTTPS_PROXY
export http_proxy="$HTTP_PROXY" https_proxy="$HTTPS_PROXY"
export NO_PROXY="${NO_PROXY:+$NO_PROXY,}${no_proxy:+$no_proxy,}localhost,127.0.0.1,::1"
export no_proxy="$NO_PROXY"

if command -v pgrep >/dev/null 2>&1 && pgrep -x chatgpt >/dev/null 2>&1; then
    pkill -TERM -x chatgpt || true
    for _ in {1..20}; do
        pgrep -x chatgpt >/dev/null 2>&1 || break
        sleep 0.25
    done
    if pgrep -x chatgpt >/dev/null 2>&1; then
        fail 'ChatGPT не закрылся. Закройте приложение вручную и запустите launcher ещё раз.'
    fi
fi

nohup "$chatgpt" --proxy-server="$HTTP_PROXY" >/dev/null 2>&1 </dev/null &
