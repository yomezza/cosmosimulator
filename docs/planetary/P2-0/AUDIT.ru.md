# Обследование текущего проекта

Проверен исходный код 0.14.4, не состояние рабочей БД. Ссылки — на файлы репозитория; наблюдения относятся к исходному commit из README этого этапа. Старые архитектурные документы использованы как ориентир и сверены с реализацией.

## Существующие механизмы

| Код / контракт | Фактическое поведение | Применение во второй очереди |
|---|---|---|
| `internal/platform/postgres.Config/Open`, `DBTX`, `Querier` | Quoted DB_SCHEMA, pool-wide search_path, Ping; не создаёт схему и не запускает мир | Подключение нового процесса без cosmic initialization; передавать tx в helpers |
| `internal/initialization.OpenDatabase` | Advisory lock 784216, последовательное исполнение embedded SQL, затем генерация/upgrade мира; в том числе schema144 | Оставить единственным исполнителем новых DDL; добавить явный маркер версии P2. Планетарный старт его не вызывает |
| `internal/geography.PlanetSurface`, `PlanetSector`, `Sample` | Сохранённая решётка высот/воды, hex-центры и соседства; Sample интерполирует Heights/Water | Один источник географии для overview и сцены; seed локальной природы не меняет сушу |
| `internal/colonies.PlanetSurface`, `storeSurface` | Lazy создание surface + sector rows; `(star,planet)` уникален. API возвращает surface id, format version и отдельную revision | Использовать surface id + sector number + revision; planet ID сам по себе не глобально уникален |
| `planet_surface_controls` / `schema124.sql` | Tombstone и revision отдельно от version формата; reset создаёт новый surface id | Не путать revision с generator_version; привязывать локальные данные к конкретному snapshot |
| `colonies.AdvanceColonies(ctx, tx, tick)` | В уже открытом tx завершает проекты, INSERT planet_colonies, status complete, события и уведомления | В этой же транзакции пометить поверхность dirty для обработки площадок; не менять ресурсы/возвраты |
| `simulation.AdvanceTick` | Exclusive world lock 784218; AdvanceColonies вместе с экономикой; Simulate вызывается раз в секунду, мировой tick регулируется game_settings | Новый быстрый цикл 20 Hz не вызывает Tick. Нельзя выполнять A*/генерацию сцен внутри этой транзакции |
| `colonies.projectAccess` | Для корпоративного проекта member определяется членством, manage — правом colonize | Это **не** разрешение входа P2. Добавить отдельное planetary.enter, сохранив права на финансирование |
| `corporations.CompanyAccess(ctx, q, uid, permission)` | Owner имеет права; остальные — список JSONB company_roles.permissions | Переиспользовать проверку с новым кодом права; обновить allowlist в commands.go и UI checkbox списка |
| `colonies.PlanetAction`, land/leave | Land требует complete и NearObject, UPSERT planet_visits; leave удаляет запись | Заменить переходом через площадку и ticket. Не оставлять старый land как обход нового протокола |
| `fleet.NearObject` | Нет Destination/LocalTarget, ObjectID совпадает с `planet-N` | Базовое правило завершённого манёвра. Космический владелец settle/recheck, планетарный не завершает полёт сам |
| `fleet.EnsureShip`, `ReadShip`, `SettleShips`, `SaveShip` | Read/settle/save космических кораблей; EnsureShip может иметь побочные эффекты | Планетарный read path не должен вызывать EnsureShip/SettleShips или SaveShip |
| `colonies.resetInfo`, `eraseSurface` | Reset сейчас блокируется любыми planet_visits и неполученными возвратами; явный порядок удаления FK, receipts сохраняются/помечаются | Добавить блокировку живого runtime и pending handoff, очистку новых зависимостей только после барьера; текущие возвраты оставить |
| `worldstore` | Load/LoadFrom, кеш и сохранённые ревизии; приватная геология | Планетарному процессу достаточно узкого snapshot/read repository. Не отдавать полный Galaxy или discovery secret клиенту |
| `account.User`, `TokenHash` | Cookie `session`, хеш токена в sessions, expires_at; User связан с HTTP response и читает также кошелёк | Вынести узкий session lookup по Querier для WS и ограниченной DB-роли, не копировать логин/пароли |
| `cmd/cosmosimulator/platform_windows.go` | SCM адаптер вызывает server.Run | Повторно использовать небольшой platform lifecycle для двух команд, без второго cosmic Run |
| `internal/server/web/features/planet/{loader,index,terrain}.js` | Lazy planet module, Canvas overview, текстуры по маскам. landing() — информационное окно «следующая очередь» | Новая lazy sector scene, отдельное состояние/runtime transport; прежнюю обзорную карту сохранить |
| `ui/runtime`, `core/session`, `scoped-view`, `scene-loop`, `asset-cache`, `I18n` | Общие окна, сессия/epoch, отмена чтения, рисование, кеш публичной графики, переводы | Один комплект UI; без новых window globals/параллельных уведомлений/авторизации |

## Существенные пробелы и совместимость

- `planet_visits` имеет только user_id, colony, ship, entered_at. Нет x/y, lease, состояния визита, handoff или поддержки нескольких процессов. Это заготовка, не готовая постоянная модель персонажа.
- В текущих navigation/planner/fleet командах не обнаружена общая защита закреплённого за высадкой корабля через planet_visits. Её нужно добавить у космического владельца во все ручные и автоматические способы движения, включая портал/эвакуацию/admin teleport. Иначе билет быстро устареет, а персонаж сможет управлять улетевшим кораблём.
- Корпоративные разрешения разрешены фиксированным allowlist: finance/manage/projects/warehouse/colonize. Кода planetary.enter пока нет; простое добавление кнопки не обеспечит права.
- `Sample` читает растровую воду, но не проверяет расстояние до полилиний Rivers. Overview дополнительно рисует Rivers линией толщиной 0.04 в координатах поверхности. Поэтому нельзя обещать точное совпадение узких русел, просто вызвав Sample. Нужен общий версионируемый адаптер маски рек, без изменения сохранённых берегов; см. SECTORS.
- Соседство Neighbors геометрическое; оно не доказывает доступность сухопутного прохода. Доля суши Land также не заменяет навигацию: сектор может содержать несколько островов.
- Административное удаление — явные DELETE без CASCADE, с финансовыми ограничениями. Новые FK необходимо включить в этот порядок. Первое безопасное поведение P2 — отказ при незавершённых визитах/передачах, а не удаление под игроками.
- Разделение пакетов готово, но раздельные DB-роли, WebSocket-библиотека и lease/fencing отсутствуют. В go.mod нет WS зависимости. Крупный движок, Redis или второй календарь не требуются.

## Что исследовано

README; docs/refactoring/service-boundaries.ru.md, server/README.ru.md, ui/DEVELOPMENT.ru.md; перечисленные выше Go-пакеты; схемы 120/124/144; lifecycle Windows; маршруты, текущая landing UI и terrain renderer; новый manifest/atlas/PNG. Графическая проверка описана отдельно в ART.

Числа производительности, фактические количества пользовательских колоний и успешность будущих миграций не измерялись. Действующую БД для обследования не читали и не меняли. Вопросы из этого документа превращены в конкретные решения в остальных материалах P2-0.

## Исходные точки входа для реализации

- [Колонизационные проекты и завершение строительства](../../../internal/colonies/planet_projects.go).
- [Текущие land/leave и проверки доступа](../../../internal/colonies/planet_api.go).
- [Удаление/перегенерация поверхности](../../../internal/colonies/planet_admin.go).
- [Каноническая география и Sample](../../../internal/geography/surface.go).
- [Корпоративные полномочия](../../../internal/corporations/commands.go).
- [Миграции и cosmic initialization](../../../internal/initialization/database.go).
- [Чистое подключение PostgreSQL](../../../internal/platform/postgres/postgres.go).
- [Существующий быстрый orchestration космической службы и мировой tick](../../../internal/simulation/ticks.go).
- [Клиентская первая очередь и stub landing](../../../internal/server/web/features/planet/index.js).
