#!/bin/zsh
set -euo pipefail

SCRIPT_DIR="${0:A:h}"
CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
ENV_FILE="$CODEX_HOME/.env"
PROXY_URL_FILE="$SCRIPT_DIR/macos-proxy-url.txt"
CERT_AGENT="$SCRIPT_DIR/cert-agent"
APP_EXECUTABLE="/Applications/ChatGPT.app/Contents/MacOS/ChatGPT"
LOCAL_PROXY="$(/usr/bin/tr -d '\r\n' < "$PROXY_URL_FILE")"
LOG_DIR="$HOME/Library/Logs/SprutDock ChatGPT Proxy"
LOG_FILE="$LOG_DIR/launcher-events.log"

/bin/mkdir -p "$LOG_DIR" 2>/dev/null || true
log_event() {
    /usr/bin/printf '%s %s\n' "$(/bin/date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$LOG_FILE" 2>/dev/null || true
}
log_event "launcher invoked"

[[ -x "$APP_EXECUTABLE" ]] || { print -u2 "Не найдено приложение /Applications/ChatGPT.app"; exit 1; }
[[ -x "$CERT_AGENT" ]] || { print -u2 "Не найден cert-agent в ресурсах приложения"; exit 1; }
[[ "$LOCAL_PROXY" == http://127.0.0.1:3128 ]] || { print -u2 "Некорректный локальный proxy URL"; exit 1; }

ENSURE_STARTED="$(/bin/date +%s)"
log_event "proxy ensure started"
if ! "$CERT_AGENT" ensure; then
    log_event "proxy ensure failed seconds=$(( $(/bin/date +%s) - ENSURE_STARTED ))"
    /usr/bin/osascript -e 'display alert "Прокси SprutDock не готов" message "Регистрация, сертификат или защищённое соединение не прошли проверку. ChatGPT не запущен." as critical' >/dev/null 2>&1
    exit 1
fi
log_event "proxy ensure ready seconds=$(( $(/bin/date +%s) - ENSURE_STARTED ))"

proxy_http=""
proxy_https=""
if [[ -r "$ENV_FILE" ]]; then
    set -a
    source "$ENV_FILE"
    set +a
    proxy_http="${HTTP_PROXY:-${http_proxy:-}}"
    proxy_https="${HTTPS_PROXY:-${https_proxy:-}}"
fi

if [[ "$proxy_http" != "$LOCAL_PROXY" || "$proxy_https" != "$LOCAL_PROXY" ]]; then
    if [[ -n "$proxy_http" || -n "$proxy_https" ]]; then
        /usr/bin/osascript -e 'display dialog "ChatGPT Proxy переключит этот запуск на локальный сертификатный туннель http://127.0.0.1:3128. Существующий ~/.codex/.env останется без изменений." buttons {"Отмена", "Продолжить"} default button "Продолжить" with icon caution' >/dev/null
    fi
fi

export HTTP_PROXY="$LOCAL_PROXY"
export HTTPS_PROXY="$LOCAL_PROXY"
export http_proxy="$LOCAL_PROXY"
export https_proxy="$LOCAL_PROXY"
export NO_PROXY="${NO_PROXY:+$NO_PROXY,}${no_proxy:+$no_proxy,}localhost,127.0.0.1,::1"
export no_proxy="$NO_PROXY"

if /usr/bin/killall -0 ChatGPT >/dev/null 2>&1; then
    /usr/bin/osascript -e 'tell application id "com.openai.codex" to quit' >/dev/null 2>&1 || true
    for _ in {1..20}; do
        /usr/bin/killall -0 ChatGPT >/dev/null 2>&1 || break
        sleep 0.2
    done
    /usr/bin/killall -TERM ChatGPT >/dev/null 2>&1 || true
    sleep 1
fi

/usr/bin/nohup "$APP_EXECUTABLE" --proxy-server="$LOCAL_PROXY" >/dev/null 2>&1 </dev/null &
log_event "ChatGPT process launched"
