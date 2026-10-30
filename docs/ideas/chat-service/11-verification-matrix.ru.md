# Chat Service — проверяемые сценарии и доказательства корректности (QA для Codex)

Версия 1.1, 2026-10-30. Источники: 01–02 требования, 08 аудит, 09 PostgreSQL, 10 алгоритмы. Ни один из тестов ниже **ещё не запускался**. Тесты должны быть автоматизированы в настоящей PostgreSQL (не SQLite), Go и браузере; результаты и команды положить в docs/ideas/chat-service/implementation-checks/.

## 1. Базовые определения

Фикстуры: U1..U53 — настоящие игроки, B1 — bot, A1 — admin, GM1 — назначенный game moderator. Компании C1/C2 (у C1 owner U1, moderator U2 с chat.moderate, ordinary U3), поверхности P1/P2, сектора S1/S2. Для spatial P2-4+ visit fixtures авторитетно создаёт только Planet. Все пользовательские данные в отдельной тестовой DB_SCHEMA; изоляция от seed и production.

Тестовый helper:
- Возвращает id, status, SQLSTATE, channel_id, seq, message_id, rate count, active participants, unread, text rows, saved-item count, outbox count.
- Clock контролируется тестовым DB/Go time-source либо timestamps fixture с явным UTC. Не подменять clock_timestamp() в production функциях без unit-test injection.
- Для concurrent тестов использовать barriers, WaitGroup и synchronization на момент **после выбора lock**; не полагаться на sleep как на доказательство race.
- Для crash теста kill -9 / SIGTERM перед определённым COMMIT или между фазами durable machine, затем старт worker и проверка eventual invariants.

## 2. Таблица P0/P1 acceptance: предусловие → действие → ожидаемый результат

| ID | Условия | Действие | Обязательный результат |
|---|---|---|---|
| AUTH01 | без cookie / с неверной session | открыть Chat WS/history | 401, никаких text rows |
| AUTH02 | sessions истекла | request через старый WS | auth_required; не записано сообщение |
| AUTH03 | B1 is_bot=true | открыть WS/send | отказ, нет chat membership и messages |
| AUTH04 | U1 имеет доступ к C1, U4 нет | U4 search/history C1 by guessed channel_id | 403/404 без наличия/содержимого чужого сообщения |
| AUTH05 | U1 в группе, U2 kicked | U2 search/subscribe/read старый group | отказ, даже если message ID известен |
| AUTH06 | U1 владеет C1, A1 имеет game admin права | A1 читает C1, U1 читает admin access log | A1 reading audited, U1 denied для служебных просмотров |
| AUTH07 | U1 входит в C1 | Cosmic меняет company_id U1 на C2; U1 SEND/SEARCH C1 | старый channel deny, C2 доступен лишь при действительном membership; UI tag текущий |
| AUTH08 | U1 имеет одну active Chat WS | U1 подключается со второй вкладки | старое живёт, новое отвергнуто duplicate_chat_connection |
| AUTH09 | lease U1 истекла и перехвачена | старый WS всё ещё имеет TCP-соединение, пытается SEND | stale_connection, ноль новых messages |
| AUTH10 | HTTP POST mutations без lease handle | другая вкладка с той же cookie пытается POST /chat/api/v1/messages | 403/405, не обходит одну Chat-сессию |
| AUTH11 | оба chat/cosmic healthy, planet unhealthy | истории Global/Corp/Private/Избранное и Planet/Sector | первые 4 доступны, spatial недоступен |
| AUTH12 | Cosmic unhealthy 3 probe periods | GET/bootstrap/send в любом разделе | все операции пользователя запрещены до повторной readiness, даже при живой БД |
| AUTH13 | юзер U1 в active visit P1/S1 | подделывает P2/S2 в target | реальный scope P1/S1 или отказ, чужие history/search не раскрыты |
| AUTH14 | Planet только legacy planet_visits (P2-3) | попытка spatial send | spatial feature gated, отказ; другие разделы доступны |
| AUTH15 | U1 active visit P1/S1, S1→S2 одновременно с send | transition UPDATE Planet-owned visit конкурирует с Chat FOR SHARE | сериализованный результат; send только в актуальный на момент commit sector; old scope никогда не принимается после подтверждённого перехода |
| AUTH16 | U1 вышел из Planet visit | load sector old history | denied, даже при прежнем открытом UI |

## 3. Private состояния и concurrency

| ID | Исходное состояние | Действие | Ожидаемые DB-инварианты |
|---|---|---|---|
| PR01 | нет DM U1/U2 | U1 invite, U2 accept | ровно 1 active DM, 2 active members, 1 private channel |
| PR02 | нет DM | U1→U2 и U2→U1 одновременно (>=20 потоков) | одна chat_dm_pairs и одна conversation, ни одной двойной истории |
| PR03 | pending DM | invite expire за 7d, U2 accept после time boundary | expired, никакой active DM |
| PR04 | pending DM | U2 reject, U1 повторяет <24h | invitation_cooldown; после >=24h новая invite разрешена при выполненной quota |
| PR05 | U1,U2 DM active с M1,M2 | U1 invite U3; до accept U3 ищет M1 | denied |
| PR06 | как PR05 | U3 accepts | type group, same channel ID, original M1/M2 unchanged, U3 reads M1/M2; consent U2 не требуется |
| PR07 | PR06 конвертирован | U1/U2 открывают новую DM | новая отдельная conversation и empty channel; прежняя группа остаётся |
| PR08 | PR05 pending conversion | U2 leaves DM перед accept U3 | DM deleting, history physically purged before completed, U3 accept denied |
| PR09 | group active U1,U2,U3 | U1 owner transfers to U2 | U2 full owner capabilities, U1 ordinary, revision++ |
| PR10 | group U1 owner,U2,U3 | U1 leave without target | отказ без изменений |
| PR11 | group U1 owner,U2,U3 | U1 leave with owner U2 | U1 left, U2 owner, U3 active, group active |
| PR12 | group active U1,U2 | U2 leave | group deleting → purged/absent; no active singleton group |
| PR13 | group active U1,U2 | U1 leaves | auto delete, transfer не требуется; U2 теряет history |
| PR14 | group active U1,U2,U3 | U1 kicks U3; снова invite+accept | U3 revoked immediately then full old history on rejoin, unread baseline 0 |
| PR15 | group active 50 | owner invites 51st | group_capacity, no invite; after one leave and group remains >=2 invite succeeds |
| PR16 | group 48 active, 2 pending | third pending invite concurrently | <=50 active+pending, no oversubscription |
| PR17 | pending group U1; все invites rejected | worker expiry/decision | auto-delete draft, no messages |
| PR18 | pending group U1 | owner tries SEND | conversation_pending, no rate slot |
| PR19 | active DM U1/U2 | U1 leave | both lose access; new DM via new invite, never old message |
| PR20 | group U1/U2/U3 | concurrent U2 leave and U1 kick U3 | serialized; group deleting if only 1 remains; never stuck with invalid owner |
| PR21 | group U1/U2/U3 | two concurrent owner transfers | one success by revision, second conflict/current owner update; no dual owner |
| PR22 | rejected group invite U2 in G1 | U1 sends another invite same target same group within 24h | cooldown; invite into independent group G2 follows rate policy |
| PR23 | U1 and U2, rolling window start | 21 valid invite requests in 24h across groups | <=20 accepted creations, 21st rate_limited; exact boundary >=24h frees slot |

## 4. Сообщения, лимиты, прочтение и контент

| ID | Тест | Ожидаемый результат |
|---|---|---|
| MSG01 | Global U1 sends at t=0,2.999,3s | first succeeds, second rejects, third succeeds |
| MSG02 | Planet U1 t=0,1.999,2s | success/reject/success; при смене planet не сбрасывается |
| MSG03 | Sector t=0,0.999,1s и Corp аналогично | success/reject/success; смена sector/company не сбрасывает rate |
| MSG04 | Private U1 10 accepted at t0..9.99s, 11th within last 10s | 11th rejects; после истечения первого события один слот свободен |
| MSG05 | 50 конкурентных Private SEND с разными request_id | максимум 10 в 10s, один seq на каждый committed message, непрерывная seq без двух одинаковых |
| MSG06 | те же request_id, identical payload, повтор через 30s после ACK lost | 1 message, same ACK, rate употреблён один раз |
| MSG07 | одинаковый request_id, разные body/action | idempotency_conflict, 0 новых messages |
| MSG08 | duplicate request_id после TTL original, пока tombstone ещё жив | тот же immutable ACK/status, **не** новый message |
| MSG09 | original reply target expired/deleted | ответ остаётся, UI «Исходное сообщение недоступно» |
| MSG10 | user attempts to reply to message in чужом Private | access_denied, не возвращать body |
| MSG11 | ровно 256/500/2500 Unicode codepoints and +1 | корректный лимит; ZWJ emoji и escaping отдельно |
| MSG12 | HTML/script and javascript game link | текст безопасен, arbitrary links rejected, никакого исполнения |
| MSG13 | Planet message in S1 then author moves to S2 | оригинальный сектор S1 сохранён, display number+canonical tooltip |
| MSG14 | UI background tab received Private message | unread +1, receipt не создаётся |
| MSG15 | message visible in active viewport; user ack same ID twice | один receipt, unread -1 ровно один раз |
| MSG16 | U3 joined group with older 10 messages | 10 доступны, unread=0; при их осмотре может появиться read receipt/blue |
| MSG17 | 3-member group U1 sent, only U2 reads | blue для U1, даже если U3 unread |
| MSG18 | expired/deleted 1 unread Private message | unread уменьшается только у ещё не прочитавших active recipients, не ниже 0 |
| MSG19 | Global/Planet/Sector unread UI | только boolean new indicator, не накопительный numeric |
| MSG20 | @all in Corporation / DM / group vs Global/Planet/Sector | первые три создают authorized notifications, остальные лишь обычный текст |
| MSG21 | mentions U2, rename username later | ID уведомлённого неизменен, чужому пользователю notification не переходит |
| MSG22 | server save message before WS close | после reconnect history восстановлена, draft text не отправлен автоматически |
| MSG23 | Cyrillic upper/lower, Latin upper/lower, substring, emoji search | совпадение согласно требованиям, не раскрывает чужие scope |
| MSG24 | expired original still physically present pending worker | read/search фильтруют expires_at, UI недоступно до cleanup |

## 5. Favorites, жалобы и модерация

| ID | Действие | Ожидаемое |
|---|---|---|
| FAV01 | U1 сохраняет 3 копии M1 | 3 самостоятельных rows, source_message_id одинаков, original body в одном живом месте + user-intended 3 copies |
| FAV02 | автор удаляет M1 | оригинал/locator отсутствуют, 3 saved copies остаются |
| FAV03 | game moderator удаляет M1 за нарушение | original и **все** связанные copies во всех пользователях физически удалены |
| FAV04 | 10 SAVE и MOD DELETE одновременно | после модераторского commit 0 linked copies, 0 original, no race leak |
| FAV05 | пользователь удаляет folder с 15 notes/copies | notes/copies остались с folder_id NULL в «Все» |
| FAV06 | user меняет чужую saved copy.body | запрещено; пользователь может изменить лишь note.body |
| FAV07 | U2 без доступа к M1 | save source ID и report ID | оба denied без body/error enumeration |
| MOD01 | глава C1 mute U2 moderator | success с одной из allowed durations и обязательной причиной |
| MOD02 | U2 moderator mute U1 owner | denied |
| MOD03 | U2 moderator mute другого moderator или самого себя | denied |
| MOD04 | U2 moderator mute ordinary U3 | success; U3 SEND Corp denied, READ/search allowed |
| MOD05 | U2 или owner снимает mute другого modifier | authorized unmute, audit id+reason |
| MOD06 | duration 299/301/произвольная вместо 300,1800,3600,86400,604800 | reject |
| MOD07 | O(100) concurrent mutes same company/target | only one current state row, serial updates/history no duplicated active authority |
| MOD08 | corporate owner пытается читать service admin access log | access_denied; admin sees own immutable audit |
| MOD09 | complaint on M1; original expired before review | reviewer sees metadata + «Сообщение недоступно», no hidden text |
| MOD10 | admin investigating complaint on private M1 | access narrowly scoped to reported message, audited; unrelated private history denied |
| MOD11 | repeated @all 5 times within 60s | messages may pass normal rate, but bulk sound suppressed after first; no duplicate notification entries |

## 6. Удаление, партиции и crash

| ID | Инъекция / сценарий | Доказательство |
|---|---|---|
| DEL01 | 1000+ сообщений Private, owner delete | immediately state=deleting/denied, completion only when count(messages,locators,receipts,invites,channel)=0; saved items remain |
| DEL02 | crash Chat before or after chunk commit | restart continues job without repeating wrong scope or reporting completed prematurely |
| DEL03 | company dissolve crash after Cosmic deleting committed | Corp closed, retry Chat physical purge, receipt, only then Cosmic refunds/deletes |
| DEL04 | crash after Chat receipt before Cosmic finalize | receipt survives, Cosmic finalizes once, no second credit refund |
| DEL05 | two concurrent company dissolve operations different UUID | only one operation chosen; no double refund, no cross-company delete |
| DEL06 | company blocked on Planet project/funds | prior Cosmic economic checks unchanged, chat cleanup **не инициируется** if invalid |
| DEL07 | surface retirement crash after Chat purge | durable receipt; geographical deletion waits on both Planet and Chat barriers |
| DEL08 | message retention boundaries Sector 7d, Global/Planet 30d, Corp 90d, Private 180d | expired original no longer accessible, physically deleted worker, favorites remain |
| DEL09 | weekly partition partly expired | remove individually expired rows; do NOT DROP partition containing newer messages |
| DEL10 | partition completely expired + locator cleaned | DETACH/DROP safe; no orphan locators/receipts |
| DEL11 | maintenance multiple processes | advisory lock elects 1 worker, repeated deletes idempotent |
| DEL12 | missing future partition | send fails safely, health/alert flags, no silent default partition |
| DEL13 | backup restored with messages expired in restored DB | до открытия Chat API выполнить TTL cleanup/reconciliation; отдельно доказать, что поздние moderator-delete после точки бэкапа повторены по WAL/независимому deletion manifest. Если такого журнала нет, система не утверждает гарантированное отсутствие воскресших оригиналов и не открывает доступ без ручного решения восстановления |
| DEL14 | PostgreSQL connection drops mid-send before COMMIT | no ACK of success; on retry exactly one message if commit actually occurred |
| DEL15 | outbox event dropped while WS client disconnected | notifications/unread/history sync from DB after reconnect, no lost committed message |
| DEL16 | ordinary remove from user after former company membership | user cannot search older corporate channel; Favorites copies still own |
| DEL17 | report/audit/outbox/operations/notifications table scan after deletion | no original body appears in any metadata column |
| DEL18 | PostgreSQL grants and migration roles | chat_runtime cannot ALTER/DROP non-chat tables, cannot SELECT password_hash, cannot execute Cosmic/Planet writes |

## 7. Checks for SQL query plans and throughput

For each hot query perform EXPLAIN (ANALYZE, BUFFERS) on representative fixture sizes, not empty tables:

1. channel history ORDER BY seq DESC LIMIT 50 → partition pruning (by kind/time where possible) + channel_seq index, avoid full table scans.
2. search with lower(body) LIKE pattern → GIN trigram when selective, index may not help for queries shorter than 3 characters; implement bounded short-query fallback restricted to authorized channel/time, timeout and result limit.
3. favorites by owner and folder → owner index, no global scan.
4. pending invitations by recipient and expires_at → partial status index.
5. expiry worker by expires_at → partition index, bounded batches.
6. mute lookup by (company_id,user_id) → PK current state.
7. repeated WS lease check/heartbeat → PK user_id row, no global scans.
8. list 50 active group members → composite index (conversation_id,status).

**NFR initial target**: 50 simultaneously connected Chat sockets, 50-member group, <=1000 operations/synthetic actions in load experiment; establish p95 and p99 actual timings and memory/DB size, with baseline and thresholds chosen based on measured resource budget. Do not claim imaginary throughput/SLA without measurements. Race tests must run with Go -race and PostgreSQL for reliable concurrency semantics.

## 8. Evidence, not just PASS labels

Каждый этап Codex должен приложить:
- SQL migration file and repeated migration result, real PostgreSQL version, schema diff, list of installed extensions.
- Test command, exit code and log path (no secrets/user message contents).
- EXPECTED and ACTUAL for negative/race test IDs; tests of access must show absence of message leak.
- Output EXPLAIN query plans (без privacy text), candidate indexes and final chosen indexes.
- Windows x64 cross-build and process smoke of Chat independently and with cosmic/planet.
- Known gaps with clear feature gate: no Planet/Sector until authoritative presence and retirement barrier, no Corp until company deleting protocol.
