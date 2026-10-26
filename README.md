# Cosmosimulator · 0.1.2

Легковесная браузерная экономическая песочница. Go + PostgreSQL, HTML/CSS/JavaScript без npm, сторонних шрифтов и CDN. Интерфейс встроен в один исполняемый файл.

## Что работает

- Регистрация, вход, выход; пароли bcrypt, сессии на 7 дней в PostgreSQL, HttpOnly cookies, проверка источника запросов, ограничение попыток входа.
- Роли `player` и `admin`. Регистрация всегда создаёт игрока. Администратор просматривает список пользователей; назначения ролей через браузер пока нет.
- Шахта каждого игрока производит 1 единицу руды каждые 10 секунд. Продажа всего запаса по фиксированной цене 5 CR. Начальный баланс 1000 CR.
- Три бота производят 2 единицы руды за такт и продают запасы каждые 3 такта. Это простые участники симуляции, не ИИ-агенты.
- Состояние мира сохраняется в базе. Во время выключения сервера производство останавливается. Общий такт атомарен и не удваивается при нескольких экземплярах сервера.
- Автоматическая начальная схема БД при запуске; блокировка и транзакция защищают параллельную инициализацию.

## Быстрый запуск в Windows

Требуется PostgreSQL 17 (либо совместимая версия) и доступ к существующей базе. Go для готового `.exe` не нужен. Сборка x64: `downloads/cosmosimulator-windows-amd64.exe`.

Отдельная база не требуется: укажите существующую базу в `DATABASE_URL` и схему игры в `DB_SCHEMA`.
Если схема отсутствует, служба создаст её автоматически (нужно право `CREATE` на базу).
Можно заранее создать схему от имени администратора PostgreSQL и передать её пользователю службы:

```sql
-- Выполняется в уже существующей базе; роль cosmos должна существовать.
CREATE SCHEMA cosmosimulator AUTHORIZATION cosmos;
```

Для существующей схемы пользователь службы должен иметь `USAGE` и `CREATE` на схему, а также права на её существующие игровые таблицы и последовательности. Рекомендуется, чтобы роль службы владела отдельной схемой игры. Не используйте схему с чужими таблицами `users`, `sessions`, `world`.

PowerShell, из каталога репозитория:

```powershell
$env:DATABASE_URL = 'postgres://cosmos:YOUR_URL_ENCODED_PASSWORD@127.0.0.1:5432/existing_database?sslmode=disable'
$env:DB_SCHEMA = 'cosmosimulator'
$env:HTTP_ADDR = '127.0.0.1:8080'
$env:ADMIN_USERNAME = 'operator'
$env:ADMIN_PASSWORD = 'YOUR_UNIQUE_ADMIN_PASSWORD_12_OR_MORE_BYTES'
.\downloads\cosmosimulator-windows-amd64.exe
```

Откройте `http://localhost:8080` в браузере. Остановка: Ctrl+C. Для других устройств задайте `HTTP_ADDR=0.0.0.0:8080` и настройте Windows Firewall. Не открывайте HTTP с паролями в интернет: используйте HTTPS reverse proxy, `COOKIE_SECURE=true`, `PUBLIC_ORIGIN=https://your-domain` и TLS к удалённой БД (`sslmode=verify-full`). `sslmode=disable` выше предназначен для локальной базы.

Имя аккаунта: 3–24 символа ASCII (буквы, цифры, `_`, `-`), регистр учитывается. Пароль: 12–72 байта UTF-8. Администратор создаётся только при явно заданных `ADMIN_USERNAME` и `ADMIN_PASSWORD`. Существующий пароль не перезаписывается, игрок не повышается до администратора. После первого запуска удалите эти две переменные: `Remove-Item Env:ADMIN_USERNAME, Env:ADMIN_PASSWORD`.

`DB_SCHEMA` задаёт одно точное имя схемы, а не список схем. По умолчанию используется `public` — старые установки продолжают работать. Схема применяется ко всем соединениям пула, включая новые подключения; обращения к таблицам не переходят автоматически в `public`. Имена с пробелами и кавычками поддерживаются, максимальная длина — 63 байта; системные схемы и специальное имя `$user` запрещены.

Чтобы сменить схему уже установленной Windows-службы, добавьте `DB_SCHEMA=cosmosimulator` в её параметр реестра `Environment` (сохранив `DATABASE_URL` и остальные настройки), затем перезапустите службу. Переключение схемы не переносит старые данные: другая схема — отдельное игровое состояние. Если хотите сохранить прежний мир, оставьте прежнюю схему или отдельно перенесите данные.

`.env.example` — справочник. Файлы `.env` автоматически **не загружаются**. Не коммитьте пароли и строки подключения.

## Windows-служба

Исполняемый файл поддерживает Windows Service Control Manager, имя службы `Cosmosimulator`. Сначала проверьте обычный запуск и создайте администратора. Затем скопируйте `.exe` в постоянный каталог, например `C:\Cosmosimulator`. В PowerShell администратора:

```powershell
sc.exe create Cosmosimulator binPath= 'C:\Cosmosimulator\cosmosimulator-windows-amd64.exe' start= auto
# Переменные именно для службы; подставьте свою строку подключения.
New-ItemProperty -Path 'HKLM:\SYSTEM\CurrentControlSet\Services\Cosmosimulator' -Name Environment -PropertyType MultiString -Value @('DATABASE_URL=postgres://cosmos:YOUR_URL_ENCODED_PASSWORD@127.0.0.1:5432/existing_database?sslmode=disable','DB_SCHEMA=cosmosimulator','HTTP_ADDR=127.0.0.1:8080') -Force
sc.exe start Cosmosimulator
sc.exe query Cosmosimulator
```

Служба получает настройки из своего окружения, а не из окна PowerShell. Ограничьте доступ к конфигурации службы: строка подключения содержит пароль. Остановка: `sc.exe stop Cosmosimulator`. При обновлении остановите службу, замените `.exe`, запустите снова. Запуск через SCM поддержан кодом, но в Linux-среде не проверялся на настоящей Windows.

## Разработка и проверки

Go 1.26 или новее, PostgreSQL 17. Из корня проекта:

```sh
go mod download
go test ./...
go vet ./...
DB_SCHEMA=cosmosimulator DATABASE_URL='postgres://cosmos:password@127.0.0.1:5432/existing_database?sslmode=disable' go run ./cmd/cosmosimulator
```

Для интеграционного теста создайте **отдельную одноразовую** базу `cosmosimulator_test`. Тесты создают в ней временные схемы и удаляют их после проверки; имя базы обязательно должно заканчиваться `_test`. Нужны права создания схем.

```sh
TEST_DATABASE_URL='postgres://cosmos:password@127.0.0.1:5432/cosmosimulator_test?sslmode=disable' go test -race -count=1 -v ./...
```

Без `TEST_DATABASE_URL` интеграционный тест пропускается. Он проверяет регистрацию, валидацию, вход/выход, запрет повышения роли, доступ администратора, защиту Origin, добычу, поведение ботов и продажу руды. Дополнительные тесты проверяют изоляцию двух схем в одной базе, разные соединения пула, безопасную обработку имени схемы и повторный запуск. `GET /api/health` проверяет соединение с БД.

Windows x64, из Linux:

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o downloads/cosmosimulator-windows-amd64.exe ./cmd/cosmosimulator
sha256sum downloads/cosmosimulator-windows-amd64.exe > downloads/SHA256SUMS
```

Windows PowerShell:

```powershell
$env:CGO_ENABLED='0'; $env:GOOS='windows'; $env:GOARCH='amd64'
go build -trimpath -ldflags='-s -w' -o downloads/cosmosimulator-windows-amd64.exe ./cmd/cosmosimulator
Get-FileHash .\downloads\cosmosimulator-windows-amd64.exe -Algorithm SHA256
```

## Границы первого прототипа

Это базис для разработки, не готовая масштабная MMO: пока нет строительства, рынка ордеров, восстановления пароля, подтверждения email, резервного копирования и панели управления ролями. У администратора только просмотр аккаунтов. Первые 20 компаний отображаются по балансу. Перед публичным запуском потребуются эксплуатационная настройка и дальнейшее развитие защиты от злоупотреблений. При каждом обновлении исходники и Windows-сборка в `downloads` сохраняются вместе в Git.
