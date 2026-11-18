# ChatGPT Proxy для macOS и Windows

Репозиторий содержит приложения для macOS и Windows. macOS-архив: `dist/ChatGPT-Proxy-macOS.zip`; текущий Windows-архив для x64: `dist/ChatGPT-Proxy-Windows-x64.zip`. ARM64 можно собрать, задав `SPRU_WINDOWS_ARCH=arm64` и передав совместимые ARM64 stunnel-бинарники.

## Сертификаты и сборка

Сборка `./build.sh` компилирует исходники `cert-agent` и встраивает его в `.app`. При запуске `launch.sh` вызывает `cert-agent ensure` до старта ChatGPT; если проверка сертификата не проходит, ChatGPT не запускается. В пакет также включаются публичные цепочки клиентского и серверного CA, stunnel и его библиотеки. Закрытый ключ CA приложению не нужен. Агент также проверяет устройство при запуске приложения и по расписанию примерно каждые 12 часов; ротация начинается, когда до истечения сертификата остаётся не больше 10 дней.

Исходники агента находятся в `cert-agent/`. Для другого расположения задайте `SPRU_CERT_AGENT_DIR`. Сборка использует актуальную клиентскую CA-цепочку из `assets/client-ca-chain.pem`; при необходимости её можно переопределить через `SPRU_CLIENT_CA_CHAIN_FILE`. Эта цепочка должна быть PEM и содержать issuing CA и root CA. Также нужны файлы, указанные через `SPRU_SERVER_CA_FILE`, `SPRU_STUNNEL_BINARY` и `SPRU_OPENSSL_LIB_DIR`. Go нужен только на машине сборки.

Без Developer ID используйте обычную ad hoc сборку (`SPRU_PRODUCTION_BUILD=0`). macOS может показать предупреждение при первом открытии загруженного приложения. Для Developer ID-подписи и нотарификации нужны `SPRU_PRODUCTION_BUILD=1`, `SPRU_DEVELOPER_ID` и профиль `notarytool`.

## Сборка Windows

Запустите `build-windows.ps1` на Windows-машине с Go. Перед сборкой задайте `SPRU_SERVER_CA_FILE` и `SPRU_WINDOWS_STUNNEL_DIR`; укажите каталог с `stunnel.exe` и DLL (обычно `bin` из официальной установки), рядом с которым или в родительском каталоге есть `config/openssl.cnf`, `config/fipsmodule.cnf` и `ossl-modules`. Сборщик включает OpenSSL-конфигурацию и provider-модули в архив. `SPRU_CLIENT_CA_CHAIN_FILE` можно не задавать, если подходит `assets/client-ca-chain.pem`. Архитектура по умолчанию — `x64`; для ARM64 задайте `SPRU_WINDOWS_ARCH=arm64`. Скрипт собирает `cert-agent.exe` и создаёт ZIP в `dist/`.

Пользователь распаковывает архив в доступную для записи папку и запускает `ChatGPT Proxy.exe`. Нужна установленная версия ChatGPT из Microsoft Store. При первом запуске потребуется приглашение SprutDock для регистрации устройства. Лаунчер проверяет сертификат перед запуском и пытается зарегистрировать пользовательскую задачу для фоновой проверки и продления. Если Windows запрещает создать задачу, лаунчер использует вход пользователя в систему и следующие запуски приложения для проверки сертификата. Лаунчер держит локальный туннель на `127.0.0.1:3128` и запускает приложение через Windows App Activation. Для обновления папка приложения должна оставаться доступной для записи. Пакет содержит только публичные CA-сертификаты, закрытые ключи создаются локально.

## Версия и обновления

Версия приложения задаётся ключами `CFBundleShortVersionString` и `CFBundleVersion` в `Info.plist`. При запуске приложение сравнивает её с последним публичным GitHub Release `SprutDock/ChatGPT-Proxy`. Если доступна новая версия, предлагает скачать архив, проверяет его SHA-256 и подпись приложения, затем заменяет текущий `.app` на том же месте с откатом при ошибке. Если папка приложения недоступна для записи, переместите его в `~/Applications`. Без сети приложение продолжает запуск. Для версии 1.0.2 и старше обновление до 1.0.3 нужно установить вручную с GitHub Releases; после этого приложение устанавливает новые версии напрямую.

Windows-лаунчер ищет `ChatGPT-Proxy-Windows-x64.zip` или `ChatGPT-Proxy-Windows-arm64.zip`, проверяет SHA-256 и заменяет файлы в текущей папке с резервной копией для отката.

## Публикация релиза

Увеличьте обе версии в `Info.plist`, соберите macOS-архив командой `./build.sh` и Windows-архивы на Windows-машинах, затем отправьте исходники и добавьте архивы в один GitHub Release. Windows-лаунчер ищет архив для своей архитектуры:

```sh
git push origin main
release_version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' Info.plist)"
release_tag="v${release_version}"
gh release create "$release_tag" \
  dist/ChatGPT-Proxy-macOS.zip \
  dist/ChatGPT-Proxy-Windows-x64.zip \
  --target main --title "$release_tag" --generate-notes
```

Если собран ARM64-пакет, добавьте в команду `dist/ChatGPT-Proxy-Windows-arm64.zip`.
