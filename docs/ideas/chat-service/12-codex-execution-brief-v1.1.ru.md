# Chat Service — окончательный handoff Codex v1.1: порядок, трассировка, ограничения и риски

Дата: 2026-10-30. Документ предназначен для разработчика (Codex), **не является поручением автоматически выполнить весь проект одним изменением**.
Источники по приоритету: 01-idea-v1.ru.md и 02-approved-decisions.ru.md (согласованная игра), 08-strict-audit-errata.ru.md (найденные ошибки), 09-postgresql-final-schema.ru.md (исправленная БД), 10-state-machines-and-transactions.ru.md (транзакции), 11-verification-matrix.ru.md (доказательства), затем 03–07 (контекст).

## 1. Что уже известно из проверенного репозитория

- Cosmic: cmd/cosmosimulator, internal/server, internal/account, internal/corporations. PostgreSQL single migration runner under advisory lock 784216; service packages не импортируют composition root.
- Planet: cmd/cosmosimulator-planet, internal/planetserver, P2-3 версии 0.17.0. Есть HTTP readiness, нет авторизованного WS gameplay/current sector; в текущем состоянии Planet/Sector нельзя включать.
- Одна DB_SCHEMA, одна PostgreSQL, пакеты internal/platform/postgres. Auth: sessions hashed session cookie, users.role player/admin, users.company_id, users.company_role. Corporate permission allowlist пока не знает chat.moderate.
- Данные Cosmic/Planet остаются у владельца. Chat не создаёт двойных users/company/planet location.
- UI: existing web/ui/menu.js, windows.js, i18n ru/en. Chat интегрировать в существующий shell, не создавать новый UI framework.
- Никакой Redis/SQLite, никаких вложений/HTML и скрытых текстовых архивов удалённого оригинала.

## 2. Взаимосвязь требований, объектов и тестов

| Требование | Владелец/таблицы | Основной use case | Приёмочные тесты 11 |
|---|---|---|---|
| One Chat WS | chat_ws_leases с generation | WS handshake/renew/send lease check | AUTH08–10 |
| Global/Planet/Sector limits | chat_channels, chat_rate_state | SendUseCase | MSG01–03 |
| Private 10/10s | chat_rate_state(private mutex), chat_rate_events | SendUseCase | MSG04–08 |
| One DM per pair + mutual invites | chat_dm_pair_locks,chat_dm_pairs,chat_invites | CreateDM,InviteAccept | PR01–04 |
| Convert DM into group with full history | chat_conversations,chat_members; same chat_messages channel | AcceptConversion | PR05–08 |
| 50 member group and owner transfer | chat_members,chat_invites,chat_conversations | GroupCommands | PR09–22 |
| 20 invites/24h + 24h cooldown | chat_invite_rate_locks,chat_invite_attempts | CreateInvite | PR03/04/22/23 |
| Unread and blue point | chat_members.unread_count,chat_read_receipts,chat_read_marks | MarkRead,Send,Delete,Reconcile | MSG14–19 |
| Copies survive ordinary deletion, not mod deletion | chat_saved_items,chat_message_locator | SaveCopy,AuthorDelete,ModDelete | FAV01–05 |
| Complaints/mentions | chat_reports,chat_notifications,chat_audit | SubmitReport,EmitMention,Review | MOD09–11 |
| Corp moderation | users.company_id, company_roles.permission,chat_corporation_mute_state/history | Mute,Unmute | MOD01–08 |
| Company final deletion after chat purge | companies.chat_deleting,chat_company_deletion_requests/receipts,chat_cleanup_jobs | BeginDissolve,PurgeCorp,FinalizeDissolve | DEL03–06 |
| Planet/Sector only current location | future Planet-owned current visit row, chat_channels | AccessResolver,SpatialSend | AUTH11–16 |
| Retention / search / partition | chat_messages,chat_message_locator,all indexes | Search,History,Expire,Detach | MSG23–24,DEL08–12 |
| Restore safety | backup/WAL or independent erasure manifest | RecoveryRunbook | DEL13 |
| Idempotency | chat_operations metadata-only result_json | All mutations | MSG06–08,DEL14 |

В каждом PR разработчик отмечает, какие строки реализованы и какие тесты приложены. Никакой новой логики не вводить без соответствующего test ID.

## 3. Этапы с жёсткими условиями выпуска

### R0 — верификация инфраструктуры и миграций

**Разрешено:** только PostgreSQL migration, таблицы chat_*, функции/индексы, schema state/GRANT, internal Chat bootstrap (health, DB connect), CI tests. **Запрещено:** менять существующие игровые правила.
Acceptance:
- проверенная версия PostgreSQL и pg_trgm доступ;
- 08 P0/A01–A07 закрываются design+SQL tests;
- все новые таблицы/индексы/секции с chat_ prefix;
- отдельная runtime роль не может читать чужие пароли, писать Cosmic/Planet или управлять чужими DDL;
- SQL migration запускается только существующим Cosmic runner, Chat только проверяет совместимость;
- rollback/re-run migration test без потери существующих Cosmic/Planet rows.

### R1 — Global and shared message core

Chat WS auth/lease with generation, bounded transport, SendUseCase, idempotency, rate, schema partitions, outbox, history/search Global, TTL worker. UI минимальный, но полноценная авторизация/отключение.
Не завершать пока AUTH01–03/08–12, MSG01/06–12/23–24, DEL08–12/14–15 не проходят.

### R2 — Private machines and invitations

DM pair locks, group/owner/accept/conversion, 20 invite window/cooldown, 50 cap, owner transfer and deletion durable status. Реализовать API/WS commands из 06 согласно запрету обхода WS lease в 09.
Не завершать пока PR01–23 и DEL01–02 не проходят; no duplicate DM, deadlocks, capacity oversubscription.

### R3 — Favorites, mentions and read model

Stable user mentions, @all Corporation/Private including DM, notifications without text, saved copies, folder all, read receipts/unread, search available channels, replies/links.
Не завершать пока MSG13–22, FAV01–07, MOD09–11 проходят. Отдельно проверить сохранение copу vs mod-delete atomicity.

### R4 — Corporation, game moderation, company dissolve

Cosmic chat.moderate allowlist and game moderator grants, corporate AccessResolver, muted state/history, audit/report access. Сначала переделать company_dissolve в barrier, затем включить corporate channel. Без double refund и утечек через admin reads.
Не завершать пока AUTH04/06/07, MOD01–10, DEL03–06/16–18 проходят.

### R5 — spatial integration (НЕ начинать до P2-4+)

Planet-owned active visit/sector/heartbeat/fence, surface retirement request/receipt, spatial read/send/search and sector switching. До реализации контрактов feature off; никакие fake spatial chats.
Не завершать пока AUTH13–16, DEL07, MSG02–03/13 и real Planet transition race не проходят.

### R6 — UI finishing + operations

Общий UI окно/compact, context menu on hover/touch/keyboard, RU/EN, settings, sound, help states, recovery runbook, backup retention and erasure manifests, EXPLAIN indexes, Windows x64, load tests 50 sockets.
Не завершать пока все test IDs пройдены с evidence; prod deployment требует rollback plan и мониторинг.

## 4. Особые границы безопасности, которые Codex не должен обходить

- Не выдавать Planet/Sector по legacy planet_visits в P2-3; нет authoritative live sector.
- Не заменять физический DELETE на deleted=true без настоящего purge.
- Не удалять чужие favorites при author-delete или TTL.
- Не оставлять favorites после MOD-DELETE оригинала; lock source locator.
- Не позволять браузеру присылать якобы author_id/company_id/current_sector/role как право.
- Не полагаться на ws connection_id из произвольного HTTP-запроса: вся мутация должна иметь verified live Chat lease и generation; рекомендовано v1 WS-only mutations.
- Не обрабатывать role/membership только при handshake — проверка в commit path и фильтрация outgoing event обязательны.
- Не выполнять HTTP из SQL-trigger, не создавать retry storms при восстановлении Planet.
- Не писать тела сообщений в audits/reports/notifications/outbox/operation result.
- Не утверждать, что restore backup автоматически устраняет текст модераторских удалений, совершённых после snapshot: нужен WAL до верного момента либо независимый deletion manifest; иначе ограничить восстановление и запросить административное решение.

## 5. Состояния ошибок и graceful degradation

- Cosmic down confirmed: весь Chat заблокирован (в том числе ws mutations).
- Planet down confirmed: только Planet/Sector заблокированы; Global/Private/Corp/Favorites продолжают работать.
- Chat down: весь Chat недоступен, Cosmic/Planet игровой процесс отдельно работает.
- Chat DB migration mismatch: readiness 503, нет DDL в Chat startup.
- Missing future partition: сохранение сообщения не подтверждать, alert, worker repair, безопасный retry с тем же request_id.
- DB commit uncertain: вернуть pending/unknown, клиент проверяет operation status, а не создаёт новую UUID.
- Slow WS consumer: ограничить очередь и закрыть, не терять committed data; после reconnect sync.
- Client mute: send in Corporation denied, read/search continue.
- Group deleting: все операции кроме status denied; complete только после физического purge.
- Admin report after original TTL: metadata visible, body «недоступно», никакого тайного архива.

## 6. Что НЕ менять без владельца

Любое изменение следующих продуктовых решений требует отдельного согласования: шесть разделов и порядок; 256/500/2500 длины; 7/30/30/90/180 дней и вечное Избранное; 1 WS на аккаунт со старым приоритетом; отсутствие editing; сохранение истории при DM→Group; приглашения 7 дней, отказ cooldown 24h, 20/24h, 50-group cap; owner-only group management; auto delete при 1; возвращение excluded; @all Corp/Private; blue read status; обязательные причины наказаний, доступность при отказе Planet; строгая очистка корпорации до final deletion.

Технические параметры health/heartbeat/locks/partitions/backup можно корректировать на основании тестов при сохранении пользовательских гарантий и документации.

## 7. Чек-лист финального PR

- [ ] Независимый cosmosimulator-chat.exe и same-origin reverse proxy.
- [ ] Нет компиляционной зависимости chatserver → cosmic/planet composition root.
- [ ] Новый DDL и compatibility marker, grants, feature flags.
- [ ] Таблицы, индексы, partition lifecycle протестированы на PostgreSQL.
- [ ] Единый ACL engine и тот же use case для каждого разрешённого transport.
- [ ] Правильно реализованы приглашения, conversion, owner, группы, cooldown, rate, unread.
- [ ] Game/corp moderation и physical deletion исправны; сохранённые копии обрабатываются по правилам.
- [ ] Нет SQL/network lock inversion или второго writer чужой таблицы.
- [ ] Все rollback/crash/race/security checks из 11 пройдены.
- [ ] Мониторинг: expiry lag, missing partitions, cleanup jobs, deadlock, lease stale, outbox backlog.
- [ ] CI: Go tests/race/vet, PostgreSQL integration, JS browser/UI, Windows x64.
- [ ] Реальный recovery drill из backup/WAL и честный privacy runbook.
- [ ] Отчёт с тестовыми командами, baseline/EXPLAIN, известными ограничениями и точно указанной версией кода.
