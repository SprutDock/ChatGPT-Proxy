# ChatGPT Proxy для macOS

Репозиторий содержит только macOS-приложение. Архив для загрузки: `dist/ChatGPT-Proxy-macOS.zip`.

## Сертификаты и сборка

Сборка `./build.sh` компилирует исходники `cert-agent` и встраивает его в `.app`. При запуске `launch.sh` вызывает `cert-agent ensure` до старта ChatGPT; если проверка сертификата не проходит, ChatGPT не запускается. В пакет также включаются публичные цепочки клиентского и серверного CA, stunnel и его библиотеки. Закрытый ключ CA приложению не нужен.

По умолчанию исходники агента ищутся рядом в `../ChatGPT-Proxy-Cert-Agent`. Для другого расположения задайте `SPRU_CERT_AGENT_DIR`. Для сборки также нужны файлы, указанные через `SPRU_CLIENT_CA_CHAIN_FILE`, `SPRU_SERVER_CA_FILE`, `SPRU_STUNNEL_BINARY` и `SPRU_OPENSSL_LIB_DIR`. Go нужен только на машине сборки.

Без Developer ID используйте обычную ad hoc сборку (`SPRU_PRODUCTION_BUILD=0`). macOS может показать предупреждение при первом открытии загруженного приложения. Для Developer ID-подписи и нотарификации нужны `SPRU_PRODUCTION_BUILD=1`, `SPRU_DEVELOPER_ID` и профиль `notarytool`.

## Версия и обновления

Версия приложения задаётся ключами `CFBundleShortVersionString` и `CFBundleVersion` в `Info.plist`. При запуске приложение сравнивает её с последним публичным GitHub Release `SprutDock/ChatGPT-Proxy`. Если доступна новая версия, предлагает открыть страницу релиза. Архив скачивается и устанавливается вручную; без сети приложение продолжает запуск.

## Публикация релиза

Увеличьте обе версии в `Info.plist`, соберите архив командой `./build.sh`, затем отправьте исходники и создайте релиз с macOS-архивом:

```sh
git push origin main
release_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' Info.plist)"
release_tag="v${release_version}"
gh release create "$release_tag" \
  dist/ChatGPT-Proxy-macOS.zip \
  --target main --title "$release_tag" --generate-notes
```
