#!/bin/zsh
set -euo pipefail

application_path="$1"
replacement_path="$2"
backup_path="$3"
staging_path="$4"
launcher_pid="$5"
attempt=0
launcher_exited=0

if [[ ! -d "$application_path" || ! -d "$replacement_path" || ! -d "$staging_path" || -e "$backup_path" ]]; then
    exit 1
fi

restore_application() {
    if [[ -d "$backup_path" ]]; then
        /bin/rm -rf "$application_path" || true
        /bin/mv "$backup_path" "$application_path" || true
    fi
    /bin/rm -rf "$staging_path" || true
    if (( launcher_exited )) && [[ -d "$application_path" ]]; then
        /usr/bin/open -n "$application_path" >/dev/null 2>&1 || true
    fi
}

trap restore_application EXIT
while /bin/kill -0 "$launcher_pid" 2>/dev/null && (( attempt < 100 )); do
    /bin/sleep 0.1
    attempt=$((attempt + 1))
done
if /bin/kill -0 "$launcher_pid" 2>/dev/null; then
    exit 1
fi
launcher_exited=1

/bin/mv "$application_path" "$backup_path"
/bin/mv "$replacement_path" "$application_path"
/usr/bin/codesign --verify --deep --strict "$application_path"
trap - EXIT
/bin/rm -rf "$backup_path" "$staging_path" || true
/usr/bin/open -n "$application_path"
