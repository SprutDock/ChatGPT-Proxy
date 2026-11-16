# ChatGPT Proxy

Готовые архивы для сотрудников:

- macOS: `dist/ChatGPT-Proxy-macOS.zip` (`./build.sh`)
- Windows: `dist/ChatGPT-Proxy-windows.zip` (`./build-crossplatform.sh`)
- Linux: `dist/ChatGPT-Proxy-linux.zip` (та же команда)

Windows launcher пока требует пилотной проверки на установленном Store-пакете. Linux-клиент ChatGPT находится в preview. Для развёртывания в компании используйте подписанные скрипты/приложения и MDM.

## macOS

Кликабельный launcher сначала проверяет сертификатное устройство через `cert-agent`, затем запускает локальный stunnel и ChatGPT Desktop с proxy `http://127.0.0.1:3128`. Агент создаёт ключ и CSR на Mac, получает сертификат автоматически по одноразовому приглашению и проверяет каждое обновление новым mTLS/CONNECT-соединением. `cert-agent` — одноразовая команда, которую launcher вызывает при запуске, а `launchd` — каждые шесть часов. Постоянно работает только stunnel.

Исходники команды находятся в отдельном git-репозитории `ChatGPT-Proxy-Cert-Agent`. Если он расположен не рядом с папкой приложения, задайте `SPRU_CERT_AGENT_DIR`. Сборщик включает исполняемый файл в `.app`.
На Mac сотрудника Go не нужен: в пакет включены готовые исполняемые файлы выбранной архитектуры.

При первом запуске сотрудник вводит выданное ему одноразовое приглашение; после проверки сервис сразу выпускает сертификат, без очереди и ручного одобрения. Если `~/.codex/.env` уже содержит другой proxy, launcher спрашивает, использовать ли локальный туннель для этого запуска. Файл `.env`, `config.toml` и системные настройки прокси не меняются.

Сборка macOS-пакета требует цепочку клиентского CA, CA серверного сертификата и stunnel 5.82 с совпадающими OpenSSL 4 dylib. По умолчанию собирается универсальный пакет; для Apple Silicon задайте `SPRU_BUILD_ARCH=arm64`. Для распространения нужны production CA, `SPRU_PRODUCTION_BUILD=1`, Developer ID Application и заранее сохранённый профиль `notarytool`; обычная сборка подписывается ad hoc только для локальной проверки.

## Пользователю

1. Переместите `ChatGPT Proxy.app` в `/Applications` или добавьте его в Dock.
2. При первом запуске launcher настроит недостающие параметры автоматически.
3. Запускайте ChatGPT через эту иконку. Запуск через обычную иконку ChatGPT не передаёт ему proxy.

Для macOS используется `macos-proxy-url.txt`; Windows и Linux продолжают брать адрес из `proxy-url.txt`. Если macOS попросит разрешить launcher управлять ChatGPT для перезапуска, разрешите это.

При запуске macOS-версия проверяет публичный GitHub Releases у `SprutDock/ChatGPT-Proxy`. Если опубликована более новая версия, приложение предлагает открыть страницу релиза. Без интернета или если релиз недоступен приложение запускает ChatGPT как обычно. Архив нужно скачать и заменить вручную; проверка не устанавливает обновление автоматически.

## Релизы

Увеличьте `CFBundleShortVersionString` и `CFBundleVersion` в `Info.plist`, соберите новые архивы и опубликуйте их как GitHub Release с тегом `v<версия>`. Для macOS используйте `dist/ChatGPT-Proxy-macOS.zip`; `build-crossplatform.sh` собирает архивы Windows и Linux. `dist/` исключена из Git: сборки прикладываются к релизу, а не коммитятся в исходники.

После сборки и `git push origin main` релиз можно создать так:

```sh
release_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' Info.plist)"
release_tag="v${release_version}"
gh release create "$release_tag" \
  dist/ChatGPT-Proxy-macOS.zip \
  dist/ChatGPT-Proxy-windows.zip \
  dist/ChatGPT-Proxy-linux.zip \
  --target main --title "$release_tag" --generate-notes
```

Developer ID для публикации архива на GitHub не требуется. Пока приложение не подписано Developer ID и не нотарифицировано Apple, macOS может предупредить при первом запуске загруженной версии; текущая локальная сборка подписывается ad hoc.

## Windows

См. [инструкцию Windows](windows/README.md). При запуске PowerShell launcher сам настраивает недостающие proxy-переменные в `.env`, затем запускает `OpenAI.Codex` в контексте MSIX-пакета с proxy-переменными и `--proxy-server`. `config.toml` остаётся без изменений. Если запуск завершается ошибкой, консоль остаётся открытой. Подпишите `Start-ChatGPT-Proxy.ps1` сертификатом компании перед массовым распространением.

## Linux

См. [инструкцию Linux](linux/README.md). Один раз выполните `bash install.sh`; установщик создаст пункт меню с иконкой ChatGPT, а launcher при первом запуске настроит недостающие proxy-переменные в `.env`. `config.toml` остаётся без изменений.
