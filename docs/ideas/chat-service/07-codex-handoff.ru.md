# Chat Service — ТЗ Codex, последовательность реализации и контроль избыточности

**ВНИМАНИЕ:** этот первоначальный этапный план дополнен и уточнён [12-codex-execution-brief-v1.1.ru.md](./12-codex-execution-brief-v1.1.ru.md). Финальные P0-исправления [08](./08-strict-audit-errata.ru.md), [09](./09-postgresql-final-schema.ru.md), [10](./10-state-machines-and-transactions.ru.md), тесты [11](./11-verification-matrix.ru.md).

**Версия:** 1.0, 2026-10-30.
**Важное:** это developer handoff. Исходная идея: [01](./01-idea-v1.ru.md); утверждённые дополнения: [02](./02-approved-decisions.ru.md); границы службы/аудит: [03](./03-architecture-integration.ru.md); PostgreSQL: [04](./04-postgresql-design.ru.md); алгоритмы: [05](./05-domain-logic.ru.md); контракты и тесты: [06](./06-protocol-operations-tests.ru.md).
Изменить продуктовые решения нельзя без владельца. Код игры, исполняемый SQL и миграции в ходе этого проектирования **не менялись и не запускались**.

## 1. Что должен получить разработчик на выходе

1. Новый самостоятельный cosmosimulator-chat.exe на Go, Windows x64 compatible, с /chat/api/v1 и /chat/ws/v1, существующим cookie-auth через серверную БД и same-origin reverse proxy.
2. Новые миграции PostgreSQL в штатном Cosmic runner с **только chat_*** новыми таблицами/индексами/функциями, кроме явно согласованных добавочных столбцов в существующих Cosmic/Planet-owned сущностях.
3. 6 разделов чата по продуктовым решениям, со всеми операциями/TTL/ACL, корректными unread и Private invites.
4. Согласованная интеграция с актуальной корпоративной ролью, компанией и Planet presence. Если Planet ещё не реализован, Global/Private/Corporation/Избранное доступны, Planet/Sector включить только после выполнения предпосылок.
5. Бэкенд/фронтенд/DB acceptance tests, включая race и падения процессов.

## 2. Обязательный план маленьких изменений (не один гигантский PR)

### M0. Схема и безопасный bootstrap

- Сверить main branch, версия кода/фактическая версия PostgreSQL и ограничения pg_trgm. Реализовать chat_schema_state, migration file после schema170.sql, отдельного DB user/GRANT и schema checks в Chat без миграций при запуске.
- Внедрить базовые типы моделей, репозиторий, DB transaction helper без скрытого Begin. Отдельный health/live и /ready, зависимости Cosmic/Planet по правилам.
- Unit/SQL tests CHECK, unique, NULL scopes, partition PK, timestamps and UTC TTL; проверка repeatable migrations, отсутствие вмешательства в game schema.

### M1. Public chat core

- Реализовать auth cookie, WS single active lease, Origin/CSRF, envelope v1, idempotency, send limits и channel resolver, messages/locator, outbox/reconnect, TTL worker + partition scheduler.
- Включить Global; проверять, что до записи выполнена авторизация. Пока не нужен Planet. В browser встроить окно Chat и базовый history/search.
- Сделать полноценные negative/duplicate/rate/retention tests, p95 SQL запросов, smoke для Windows x64.

### M2. Private DM/group

- Реализовать conversations, dm_pairs, active members, invites, invite attempt quota/cooldown, create/accept/reject, conversion, owner/rename/transfer/leave/kick/delete-for-all.
- Все state transitions закрыть transaction tests: взаимные приглашения, auto-delete при одном оставшемся, pending group, 50 active/pending slots, rejoin доступ к старой истории.
- Не дублировать ветвления API/WS: оба транспорта направляют команды в domain service.

### M3. Favorites, replies, mentions, unread

- chat_saved_items/folders/settings, notes vs copies, private read receipts, публичные индикаторы, offline unread, search across each current authorized channel, игровой link type.
- Проверить read receipt с IntersectionObserver, чтобы background не читал; давать пользователю возможность открыть индивидуальное письмо/уведомление.
- Поддержать @имя и @all только в установленных типах каналов с защитой от notification spam; RU/EN интерфейс.

### M4. Corporation, модерация, отчёты

- Подключить existing users.company_id, companies.owner_id, roles JSONB; добавить chat.moderate в корпоративный allowlist и admin grants назначенного игрового модератора через Cosmic.
- Мute с готовыми сроками/иерархией, reason, report UI, audit 180 days, restricted corporate browsing admin audit.
- **Сначала внедрить** durable company deletion workflow с флагом deleting, request/receipt, безопасным refund-on-finalize, затем разрешить corporation chat. Нельзя принимать corporate messages, если ещё работает мгновенный company_dissolve.
- Тестировать user leave/role revoke во время WS send/search, no unauthorized visibility.

### M5. Planet/Sector — только когда Planet реализует runtime presence

- После P2-4 и дальнейших Planet этапов расширить Planet-owned current visit row (current_surface/current_sector, connection_epoch, state, presence freshness). Строку пишет **только Planet**. При short Chat send tx выполнять SELECT ... FOR SHARE по актуальной Planet visit row; Planet transition/leave/revoke обязаны обновлять **ту же** строку. Scope determined server-side.
- Добавить surface retirement cleanup request/receipt либо интегрировать в текущий retirement barrier. Добавить dynamic Planet/Sector tabs и номер исходного сектора; старые данные чужих локаций не доступны.
- При отсутствии настоящего active visit **не** считать legacy planet_visits валидным источником; пространственная функциональность отключена до готовности.

### M6. Надёжность и выпуск

- 50 simultaneous sockets, 50 members group, mixed workload, EXPLAIN indexes, pg_trgm search, 180d retention simulation with clock injection, partition rotation/drop, crash+restore, deadlocks, no text in audit/outbox.
- Продакшен runbook: миграции, архив/backup TTL, worker leadership, аварийный перезапуск, ошибочная версия схемы/партиций, rollback и восстановление корректных прав.
- Go test/-race/vet, Postgres integration tests, JS UI tests, Windows x64 cross-build; release notes.

## 3. Как убрать избыточные проверки и условные ветви

| Антипаттерн | Правильное решение |
|---|---|
| Шесть самостоятельных send endpoints с копированием ACL/TTL/rate | Один ChannelPolicy config (limit, retention, rate) + общий SendUseCase; только resolution scope специфичен kind |
| Проверка авторизации в UI, WS handler, domain service и repository с разными правилами | На границе session/Origin, в application один AccessResolver, внутри SQL tx повторно только **изменяемые инварианты** (membership, deleting, mute, presence). UI — лишь affordance |
| Сначала HTTPS Planet presence, затем БД, затем снова HTTPS Planet | Health gate отдельно; для send один **авторитетный DB row-lock fence** Planet-owned visit под короткой tx. Не дублировать location в chat_* |
| Отдельные роли в Chat поверх users.role/company_roles | Считывать существующую модель; назначенные игровые модераторы — узкий Cosmic-owned grant |
| Переходы Private разбросаны по обработчикам / триггерам | Одна ConversationStateMachine; подтверждения и изменения состояния в одной repository transaction |
| Одна очередь unread в памяти и вторая в PostgreSQL | Источник истины — chat_members/receipts; outbox только уведомление о необходимости подтянуть данные |
| Хранить скопированный текст для reply/жалоб/админ-аудита | Хранить original message ID, без body snapshot; единственное исключение — явно согласованная самостоятельная копия Favorites |
| «Удалил» как deleted=true, партиции продолжают содержать текст | Настоящий DELETE + transactional favorite cascade при moderator deletion + retention worker |
| Дополнительная таблица account/company/geography только для чата | Не создавать: текущие authoritative owners уже есть |
| Частотные ограничения в Go RAM mutex | Transactional chat_rate_state и chat_rate_events / invite_rate_locks в PostgreSQL, чтобы restart не сбрасывал квоту |
| Одновременно SQL trigger и Go INSERT для одного outbox event | Выбрать **одну** реализацию; рекомендован explicit INSERT в той же tx |
| Генерировать отдельную таблицу на каждый сектор/планету/корпорацию | Одна chat_channels со scope + фиксированные типовые partition ranges для message history |
| Блокировать весь чат при недоступности Planet | **Неверно**: блокируются только Planet/Sector, остальные зависят от Cosmic/Chat |

**Обязательный стандарт:** в каждом use case отдельно документировать preconditions, row locks, one transaction, postconditions, idempotent retries, effect on unread/favorites/audit. Избежать огромных switch-структур: typed policy + маленькие domain transitions; не распылять необратимые правила по UI, SQL и WS.

## 4. Технические решения, которые можно принять без новых вопросов владельцу

- Heartbeat 15s/lease 45s, health probe 3s/3 failures, retry/jitter и grace — config defaults, менять по нагрузочным тестам.
- Пакет 16 KiB, page 50, limit search 128 codepoints, timeout/statement_timeout, invite quota rolling 24h, private rate rolling 10s; boundary тесты обязательны.
- Партиции Sector/Global/Planet daily, Corporation/Private weekly; в каждом read TTL filter, background cleanup >=10m, drop только гарантированно expired partitions; monitoring alert при lag >30m.
- Body lengths в Unicode codepoints; PostgreSQL char_length vs Go rune count тест. HTML никогда не выполняется.
- Срок audit 180d согласован; backup/WAL 14d/7d в 04 — **операционные defaults для проверки и утверждения эксплуатационной политикой**, не обещание мгновенного стирания.
- Нет Cron внутри СУБД: планировщик у Chat worker + SQL с advisory locks. Не нужна pg_cron extension или суперпользователь PostgreSQL для выполнения штатной очистки.

## 5. Блокирующие условия перед включением конкретных частей

- Global/Private/Избранное можно внедрять уже с текущими Cosmic sessions и PostgreSQL, после безопасной миграции и Chat readiness.
- Corporation **нельзя** включить, пока company_dissolve делает немедленный DELETE без подтверждения chat cleanup и не добавлено chat.moderate.
- Planet/Sector **нельзя** включить, пока нет достоверного Planet-owned active visit row + актуального sector/transition fence и проверки Planet health.
- Game moderator feature требует назначения Cosmic-owned grants; пока доступ только у existing admins, а UI для назначенного модератора скрыт.
- Для удаления поверхности после включения spatial chats требуется новое подтверждение очистки в retirement flow. Нельзя просто каскадно снести чаты.
- Миграция и API rollout не должны сломать текущую космическую/планетарную игру, включая существующие в данный момент P2-3 функции.

## 6. Acceptance gate перед признанием готовым

1. Все сценарии 06 проходят в PostgreSQL integration, race/negative/crash tests.
2. На 50 websocket connections нет двойных сообщений/утечек доступа, TTL и индексы выдерживают нагрузку.
3. Физическое удаление подтверждено запросом к активной PostgreSQL, включая saved copies после moderator deletion; сохранённые оригиналы в backup/WAL задокументированы честно.
4. Здоровье Cosmic/Planet/Chat ведёт себя как в 02: Planet failure не блокирует Global/Private/Corporation/Избранное.
5. Результаты сборки для Windows x64, process smoke, UI tests, DB migration upgrade и восстановления приложены в docs/ideas/chat-service/implementation-checks/ или другом явно обозначенном каталоге отчётов.
6. Нельзя маркировать Chat v1 «готовым» по одному успешному happy-path: нужен полный пройденный acceptance и отсутствие известных access-control bypass.

## 7. Что НЕ делать

Не писать очередную «идею» вместо спецификации; не исправлять продуктовые решения без согласования; не создавать Chat-account/password tables, SQLite/Redis, дублирующий Planet presence; не исполнять SQL из технического проекта автоматически; не переносить cleanup в триггер company delete; не делать логи с message body; не хранить body в жалобах; не доверять браузерским ролям, городам, surface/sector или ссылкам; не пытаться реализовать пространственный режим раньше подтверждённого Planet presence.

## 8. Краткий статус handoff

Проектирование основных операций, таблиц и зависимостей завершено **на уровне подробного ТЗ**. SQL migrations и Go реализации ещё не написаны, production performance не измерена. Для независимых этапов M0–M4 разработчик может начать реализацию по этим документам; M5 зависит от завершения Planet presence и retirement integration. Не считать технические defaults, которые явно помечены как предложения, дополнительными утверждёнными решениями владельца.
