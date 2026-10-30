# PostgreSQL: проект схемы, прав и миграции

**Предложение; SQL не создан в runtime и не исполнялся.** Одна существующая игровая схема. Новых экономических таблиц и дублирующих пользователей/владельцев нет. Имена уточняются при реализации; основные ограничения должны сохраниться.

## Таблицы

| Таблица | Ключевые поля и ограничения | Единственный писатель |
|---|---|---|
| `planet_runtime_controls` | PK surface → planet_surfaces.id; geography_revision, claims_epoch, processed_epoch, state (`enabled/retiring`), dirty/retry_at/error. Один durable work item на surface, processed_epoch ≤ claims_epoch | Cosmic |
| `planet_landing_sites` | PK id; UNIQUE colony → planet_colonies.id; surface, sector; FK `(surface,sector)`; FK/проверка соответствия colony месту; geometry_revision, layout_version, x_q/y_q, enabled, created_at. Координаты внутри [0,262144], фактическая hex/суша проверяется алгоритмом | Cosmic |
| `planet_transfer_controls` | PK user_id → users; generation BIGINT, reserved_ship → ships, active_operation UUID, revoke_requested_at. Проверяемые состояния reservation; UNIQUE ненулевой reserved_ship. Монотонное поколение допуска | Cosmic |
| `planet_handoffs` | PK id UUID; user_id, operation UUID, payload_digest; landing_site, ship, source session hash, generation, revision; token_hash BYTEA UNIQUE (32 bytes); expires_at, revoked_at. UNIQUE(user_id,operation); token plaintext не хранится | Cosmic |
| `planet_handoff_uses` | PK handoff_id → handoffs; visit_id UNIQUE, redeemed_at. Однократный INSERT в одной транзакции с созданием визита; owner_user сверяется с билетом, не приходит от клиента | Planet |
| `planet_characters` | PK user_id → users; last_surface/sector FK, last_revision, x_q/y_q, position_version. Один персонаж на аккаунт; nullable location только до первого входа/после удаления старой поверхности | Planet |
| `planet_visits` — расширение существующей | user_id PK остаётся; добавить visit_id UUID UNIQUE, state (`legacy/pending/active/disconnected/ended`), generation, runtime_fence, connection_epoch, current_surface/sector/revision, origin landing_site, timestamps, end_reason; существующие colony/ship сохраняются для совместимости на период перехода | Planet после переключения; старый cosmic land/leave отключается |
| `planet_local_locations` | PK/UNIQUE(surface,sector,revision,local_generator_version); location_id UUID UNIQUE; FK `(surface,sector)`; source checksum, decor_seed, dimensions=256, compact immutable geometry BYTEA + format/compression version/checksum; параметры генератора. Не 65 536 строк клеток | Planet |
| `planet_runtime_leases` | Singleton PK id=1; runtime_id UUID, fence BIGINT монотонный, expires_at. Продление/захват через условный UPDATE в DB, проверка актуального fence при каждом checkpoint/переходе | Planet |

`x_q/y_q` — целые значения 1/1024 игровой единицы. Это исключает NaN/Infinity в постоянной позиции; граница диапазона сама по себе не означает валидную точку. Визит при пешем переходе обновляет текущий сектор, но сохраняет исходную площадку и корабль. Нет второго owner у local_locations: собственность всегда выводится из завершённой канонической колонии.

Новые JSONB допустимы для неизменяемых параметров/описаний сцены. Воспроизводимые природные объекты пока восстанавливаются из seed/version; отдельные таблицы урожая, инвентаря, «запасов дерева» и будущих overrides не создаются.

Индексы: площадки `(surface,enabled)` и colony UNIQUE; handoffs `(user_id,expires_at)`; live visits `(current_surface,current_sector)` WHERE state IN ('pending','active','disconnected'); персонаж по user PK; dirty controls partial index `(retry_at,surface)` WHERE claims_epoch>processed_epoch. Срок хранения истёкших билетов/uses выбирается с учётом FK и журнала операций; cleanup не удаляет активный визит.

## Владение и FK

Площадка физически принадлежит конкретной colony, а не кешируемой «компоненте территории». Юридический владелец читается из colony; при объединении ничего не переносится. UNIQUE colony ограничивает только автоматически выдаваемый стартовый модуль P2, не является обещанием ограничения на будущие строящиеся площадки.

Составной FK на colony `(id,surface,sector)` потребует соответствующего UNIQUE у planet_colonies либо эквивалентного ограничения при INSERT. Это небольшое расширение ключей существующей таблицы, не переименование/смена ID. Типы nullable полей/согласованность origin colony и current location задаются CHECK и FK по состояниям визита.

Для производных ссылок RESTRICT; никаких автоматических CASCADE в финансы/вклады. Reset сначала блокирует новые handoffs, требует отсутствие pending/живых визитов, архивирует/закрывает ссылки завершённых визитов и персонажей на удаляемую location, очищает uses/tickets/sites/locations в явном порядке, затем выполняет прежний eraseSurface и повышает revision. Финансовые receipts, возвраты и данные астрономической планеты не теряются. В первой реализации операция отказывается работать, если этот порядок пока нельзя выполнить безопасно.

## Транзакции и блокировки

Сохраняется текущий порядок `world advisory 784218 → user/ship/project` в космических командах. Migration lock 784216 остаётся отдельно и берётся при старте, не в движении.

Planet DB-переходы: короткий shared world lock 784218 → нужные surface controls по возрастанию surface → handoff/control пользователя → visit/character → locations по порядку ключей. Reconciler cosmic использует существующий exclusive world lock лишь для чтения/фиксации короткого snapshot; тяжёлые расчёты вне lock. Нового глобального exclusive lock для каждого кадра нет.

Реализация должна подтвердить порядок против всех затронутых callers. Не ждать сетевого ответа/actor channel при удерживаемом tx. Helpers получают тот же DBTX, не создают вложенные pool connections; расчет пути не требует SQL. LISTEN/NOTIFY можно использовать для ускорения invalidation, но durable generations/версии остаются источником истины.

## Безопасное обновление

1. Остановить обе службы (на первом внедрении planet ещё отсутствует); снять согласованную резервную копию **всей игровой схемы** и проверить восстановление в одноразовой `_test` БД. Зафиксировать версии EXE и перечень старых planet_visits.
2. Аддитивная миграция существующим cosmic runner под 784216: новые таблицы/ограничения, nullable поля старого visits, compatibility marker. **DDL не запускает генерацию всех локальных карт.** Нет второго runner у planet.
3. Старые visits пометить `legacy`, без фиктивных координат/активного персонажа. Они не означают живой WebSocket. Показать пользователю восстановление/явный возврат; не телепортировать корабль и не присваивать случайный новый сектор.
4. Добавить mark-dirty всем surface с завершёнными колониями. Cosmic worker после старта размещает площадки по связанным компонентам. Нет повторного списания colony_core/кредитов, новых проектов или наград. Неудачное размещение даёт ожидающее состояние, не Terraform.
5. Отключить прежние cosmic INSERT/DELETE в visits и stub landing; включить новый протокол за feature flag только при compatible planet ready. Перед этим остановить старый EXE: одновременный запуск старого писателя запрещён.
6. Запустить planet с отдельной ролью и чистым postgres.Open. При отсутствии marker/неподдерживаемой версии — readiness 503 без DDL/seed.
7. Открыть высадку после backfill выбранной поверхности. Техническая готовность каждой площадки видна клиенту; глобальная игра не ждёт обработки всех планет.

Откат первой аддитивной схемы допускает сохранить новые таблицы, но перед запуском прежнего кода нужно отключить planet, закрыть/заморозить новые визиты и убрать reservation controls безопасной процедурой. Нельзя просто запустить старый land handler одновременно с новым runtime. Восстановление backup теряет изменения после его момента и требует отдельного согласования.

## Роли PostgreSQL

DDL/владельцем таблиц остаётся operator/migration role. Runtime роли cosmic и planet получают только необходимые SELECT и INSERT/UPDATE/DELETE своего столбца ответственности, USAGE на свою sequence. Planet не имеет DML на users, ships, world, кошельки, колонии, площадки и tickets. Cosmic не получает DML на local position/uses. Чтение sessions ограничить узким lookup; не передавать password_hash/credits клиенту.

Создание login roles и передача секретов — настройка развёртывания оператором, не автоматическое создание паролей приложением. GRANT/REVOKE шаблоны и фактические negative tests обязательны в P2-1/P2-9. При одном общем DB login single-writer остаётся правилом кода, но DB ещё не обеспечивает его разграничение; это необходимо явно указать при таком запуске.
