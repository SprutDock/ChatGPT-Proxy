#!/bin/zsh
set -euo pipefail

SCRIPT_DIR="${0:A:h}"
PROXY_URL_FILE="$SCRIPT_DIR/macos-proxy-url.txt"
CERT_AGENT="$SCRIPT_DIR/cert-agent"
CHATGPT_APP="/Applications/ChatGPT.app"
APP_EXECUTABLE="$CHATGPT_APP/Contents/MacOS/ChatGPT"
LAUNCHER_EXECUTABLE="$SCRIPT_DIR/../MacOS/ChatGPTProxyLauncher"
LOCAL_PROXY="$(/usr/bin/tr -d '\r\n' < "$PROXY_URL_FILE")"
LOG_DIR="$HOME/Library/Logs/SprutDock ChatGPT Proxy"
LOG_FILE="$LOG_DIR/launcher-events.log"

/bin/mkdir -p "$LOG_DIR" 2>/dev/null || true
log_event() {
    /usr/bin/printf '%s %s\n' "$(/bin/date '+%Y-%m-%d %H:%M:%S')" "$1" >> "$LOG_FILE" 2>/dev/null || true
}
log_event "launcher invoked"

[[ -x "$CERT_AGENT" ]] || { print -u2 "Не найден cert-agent в ресурсах приложения"; exit 1; }
[[ "$LOCAL_PROXY" == http://127.0.0.1:3128 ]] || { print -u2 "Некорректный локальный proxy URL"; exit 1; }

ENSURE_STARTED="$(/bin/date +%s)"
log_event "proxy ensure started"
AGENT_PID=""
cancel_ensure() {
    if [[ -n "$AGENT_PID" ]]; then
        /bin/kill -TERM "$AGENT_PID" >/dev/null 2>&1 || true
        wait "$AGENT_PID" >/dev/null 2>&1 || true
    fi
    exit 130
}
trap cancel_ensure INT TERM
exec 3<&0
"$CERT_AGENT" ensure --events <&3 &
AGENT_PID=$!
if wait "$AGENT_PID"; then
    ENSURE_STATUS=0
else
    ENSURE_STATUS=$?
fi
trap - INT TERM
exec 3<&-
AGENT_PID=""
if [[ "$ENSURE_STATUS" -ne 0 ]]; then
    log_event "proxy ensure failed seconds=$(( $(/bin/date +%s) - ENSURE_STARTED ))"
    exit "$ENSURE_STATUS"
fi
log_event "proxy ensure ready seconds=$(( $(/bin/date +%s) - ENSURE_STARTED ))"
CODEX_STATUS=0
"$CERT_AGENT" configure-codex || CODEX_STATUS=$?

[[ -x "$APP_EXECUTABLE" ]] || { print -u2 "Не найдено приложение /Applications/ChatGPT.app"; exit 1; }
[[ -x "$LAUNCHER_EXECUTABLE" ]] || { print -u2 "Не найден системный запускатель ChatGPT"; exit 1; }

export HTTP_PROXY="$LOCAL_PROXY"
export HTTPS_PROXY="$LOCAL_PROXY"
export http_proxy="$LOCAL_PROXY"
export https_proxy="$LOCAL_PROXY"
export NO_PROXY="${NO_PROXY:+$NO_PROXY,}${no_proxy:+$no_proxy,}localhost,127.0.0.1,::1"
export no_proxy="$NO_PROXY"

while /usr/bin/pgrep -x ChatGPT >/dev/null 2>&1; do
    if ! /usr/bin/osascript -e 'display dialog "ChatGPT уже открыт. Существующий экземпляр не переключается на новый туннель. Закройте ChatGPT и нажмите «Повторить» для безопасного запуска." buttons {"Отмена", "Повторить"} default button "Повторить" cancel button "Отмена"' >/dev/null 2>&1; then
        exit 1
    fi
done

if ! "$LAUNCHER_EXECUTABLE" --open-application "$CHATGPT_APP" "--proxy-server=$LOCAL_PROXY"; then
    log_event "ChatGPT launch failed"
    exit 1
fi
log_event "ChatGPT launched through Launch Services"
if [[ "$CODEX_STATUS" -ne 0 ]]; then
    log_event "Codex CLI setup failed code=$CODEX_STATUS"
    /usr/bin/osascript -e 'display notification "Проверьте .env в папке Codex. Для работающего фонового сервера выполните codex app-server daemon restart." with title "Настройка Codex CLI не завершена"' >/dev/null 2>&1 || true
fi
