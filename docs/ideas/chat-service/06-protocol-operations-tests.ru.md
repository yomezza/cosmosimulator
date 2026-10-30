# Chat Service — HTTP/WS v1, служебные контракты, удаление и тесты

Версия проектного API 1.0, 2026-10-30. Основание: [01](./01-idea-v1.ru.md), [02](./02-approved-decisions.ru.md), [03](./03-architecture-integration.ru.md), [04](./04-postgresql-design.ru.md), [05](./05-domain-logic.ru.md).
Все маршруты и payload ниже — **контракт для Codex**, а не утверждение, что endpoint уже существует.

## 1. Сетевой и безопасностный контракт

- Один same-origin HTTPS: внешний reverse proxy обслуживает /chat/api/v1/* и /chat/ws/v1, проксируя на 127.0.0.1:8082 (конфиг CHAT_HTTP_ADDR). TLS заканчивается в доверенном reverse proxy, cookie session существующая HttpOnly/SameSite Lax, без второй системы логинов.
- Для Chat WebSocket проверка Origin на handshake против PUBLIC_ORIGIN/allowed origins; Sec-Fetch-Site cross-site запрет; IP из Forwarded/XFF только через trusted proxies. Не принимать user_id, role, planet_id, sector_id, company_id, author_id из client как права.
- Перед upgrade: найти session cookie, SHA256 hex, read sessions JOIN users (expires_at>clock_timestamp(), NOT is_bot). Cosmic readiness обязателен. Chat JWT/token самостоятельно не выпускает. При logout/expiry WS прекращает пользовательские операции и закрывается; ревалидация не реже 30s + перед защищённой mutation.
- Для HTTP POST: application/json, bounded body (рекомендуется max 16 KiB), CSRF/Origin и тот же session; для WS max frame 16 KiB для client commands, max inbound events/sec и общий backpressure buffer. Payload с неизвестной версией/дополнительными запрещёнными полями отклонить. Не принимать бинарные/медиа сообщения.
- Результаты: status 200/201, 400 invalid_argument, 401 auth_required, 403 access_denied, 404 not_found (без раскрытия существования чужого канала), 409 conflict, 410 gone, 429 rate_limited + retry_after_ms, 503 dependency_unavailable. Ошибка без SQL/DSN/session/скрытого текста.
- Сервис возвращает DB BIGINT как decimal string, UUID как string, TIMESTAMPTZ как ISO8601 UTC, ни JS Number для BIGINT, ни локальный timestamp без зоны.

## 2. HTTP endpoints

| Метод и путь | Операция, вход | Проверка / ответ |
|---|---|---|
| GET /chat/api/v1/health/live | liveness без авторизации | статус процесса; не считать auth proof |
| GET /chat/api/v1/health/ready | ready | schema version, DB, readiness Cosmic; spatial readiness отдельным флагом; не возвращать secrets |
| GET /chat/api/v1/bootstrap | — | доступные вкладки, текущие channel identifiers, список доступных DM/groups, pending invites, unread counters, settings, service availability |
| GET /chat/api/v1/history?kind=&target=&before_seq=&limit= | <=50 | текущий ACL и TTL, сообщения DESC + next_cursor; reply references разрешать индивидуально |
| GET /chat/api/v1/search?kind=&target=&q=&cursor= | <=50 | ACL, поиск только по конкретному resolved scope; для Private можно искать по всем активным беседам пользователя через отдельный private/all режим |
| POST /chat/api/v1/messages | request_id,kind,target,body,reply_to?,mentions?,links? | атомарный send; ACK message_id/seq/time |
| DELETE /chat/api/v1/messages/{message_id} | request_id,reason? | авторское удаление без причины; модерационное — с причиной и очисткой Избранного |
| POST /chat/api/v1/read | request_id,kind,target,message_ids<=100 или public_seen_seq | точные Private receipts или курсор публичного просмотра; только свой read |
| POST /chat/api/v1/private/conversations | request_id,type,recipient_id?/title? | create pending DM/group, лимит количества, owner |
| POST /chat/api/v1/private/{id}/invite | request_id, recipient_id | owner only, cooldown/cap; для DM — приглашение в conversion |
| POST /chat/api/v1/private/invites/{id}/decision | request_id, decision=accept/reject | recipient only, expiry/race safety |
| POST /chat/api/v1/private/{id}/leave | request_id,new_owner_id? | правила ручной передачи/автоудаления |
| POST /chat/api/v1/private/{id}/kick | request_id,target_user_id | owner only, возможно автоудаление |
| POST /chat/api/v1/private/{id}/owner | request_id,target_user_id,revision | owner only, актуальный active target |
| POST /chat/api/v1/private/{id}/rename | request_id,title | owner only |
| DELETE /chat/api/v1/private/{id} | request_id,confirm=true | owner only, delete-for-all |
| GET /chat/api/v1/favorites | folder_id?, q?,cursor? | только свой owner_id; виртуальная папка «Все» |
| POST /chat/api/v1/favorites | request_id, note_body? ИЛИ source_message_id | создать личную note или копию доступного original |
| PATCH/DELETE /chat/api/v1/favorites/{id} | request_id, note_body? / folder_id? | редактировать только note, переносить copy разрешено |
| GET/POST/PATCH/DELETE /chat/api/v1/folders[/id] | request_id,name,position | «Все» виртуальная, пользовательские записи сохраняются при удалении папки |
| GET/PUT /chat/api/v1/settings | UI layout,sounds,hidden_tabs | own settings, серверная проверка |
| POST /chat/api/v1/reports | request_id,message_id,reason | сообщение сейчас доступно, dedupe, без body snapshot |
| POST /chat/api/v1/corporation/mutes | request_id,target_id,duration,reason | corporate owner/moderator role + hierarchy |
| DELETE /chat/api/v1/corporation/mutes/{id} | request_id,reason? | authorized early unmute |
| GET /chat/api/v1/admin/reports | state/cursor | только global admin/assigned game moderator |
| POST /chat/api/v1/admin/reports/{id}/resolve | request_id,decision,reason | reviewer authorization, audit |
| GET /chat/api/v1/admin/audit | filters/cursor | authorized game administration only |
| GET /chat/api/v1/operations/{request_id} | — | только собственный статус/результат; без чужого контента |

В production обязательно задать единые schema validators для входных DTO, bounds, sort/cursor, pagination. Административные команды должны использовать отдельное permission check, не наследовать доступ от видимости кнопки. Не нужны одновременно POST и WS с различной логикой: оба вызывают **один application use case**.

## 3. WebSocket v1

- Подключение GET /chat/ws/v1, cookie-authenticated, при отсутствии соединения сессия fail. Первый client message за <=5s: {"v":1,"type":"hello","seq":1,"payload":{"resume_cursor":...}}; не передавать session token в URL или hello.
- Общая форма command: {"v":1,"type":"message.send","seq":42,"request_id":"uuid","payload":{"kind":"private","target":"uuid","body":"text","reply_to":null}}. Поля kind/target **только указание цели**, не полномочие. Отдельные команды можно маппить на HTTP equivalents; единственные writers — application services.
- Server welcome: {"v":1,"type":"welcome","seq":1,"payload":{"connection_id":"uuid","server_time":"ISO8601","available_tabs":[...],"limits":{...}}}.
- Server ACK: {"v":1,"type":"ack","ref_seq":42,"request_id":"uuid","payload":{"message_id":"uuid","channel_id":"uuid","channel_seq":123,"sent_at":"ISO8601"}}.
- Server events: channel.message, channel.message_removed, conversation.changed, invite.created, invite.decided, unread.changed, mute.changed, scope.changed, service.availability, favorites.changed. У всех event_id/seq; событие не содержит тела сообщения, если viewer больше не имеет прав, и никогда не содержит закрытую moderation reason без проверки доступа.
- Ping/pong каждые 15s (настраивается), Chat DB lease TTL 45s, renew каждые 15s и при любом write check: chat_ws_leases(user_id PK,connection_id,expires_at). Если старая действующая lease жива, **новый WS отклоняется**, старый не закрывать. При real disconnect DELETE только WHERE connection_id=own; при dead lease старше DB clock TTL — новый can claim атомарным compare-and-swap. Старая connection, потерявшая lease, немедленно прекращает writes/fanout. Инвариант — нет двух авторизованных writers для одной chat lease generation.
- Утрата WS после commit: client вызывает operations status/повторяет идентичный request_id; событие из outbox может прийти позже/повторно. Client dedupe по message_id + channel seq, затем перечитать пропущенный cursor. Никаких гарантий «событие пришло ровно один раз».
- WS queue ограничить (например 256 events или 2 MiB), медленного читателя разорвать с retryable code; отсутствие рассылки не отменяет сохранённое сообщение. Outbox + LISTEN/NOTIFY только wakeup; source truth — PostgreSQL. Private unread/invites восстановить с bootstrap.

## 4. Межсервисные контракты

### Cosmic ↔ Chat

- Auth: shared DB read-only sessions/users по SHA256, но Cosmic готовность обязательна; отдельная internal ready/read model предпочтительна. Если Cosmic недоступен подтверждённо → запрет всех пользовательских операций Chat.
- Company membership: read only users.company_id/company_role, companies.owner_id, company_roles.permissions. Изменения membership/roles/owner должны либо публиковать durable revision для revocation/fanout, либо Chat повторно читает актуальное состояние при операции и коротком периодическом sync (по умолчанию <=2s); текущий код ещё не содержит revision, поэтому оптимальная интеграция — Cosmic-owned monotonic membership revision + события. **При невозможности согласовать безопасный revocation fence запретить привилегированное действие**, не полагаться на длительный cache.
- Глобальное назначение игровых модераторов: Cosmic-owned grants; авторизованный owner updates, Chat SELECT. Нынешнее users.role=admin работает без расширения CHECK роли.
- COMPANY DELETE: см. §5, не прямой DELETE companies как сейчас.

### Planet ↔ Chat

- **Предпочтительный контракт без дополнительного HTTP на каждое сообщение:** расширенная Planet-owned current visit row (в общем DB_SCHEMA) с полями user_id,visit_id,state,connection_epoch,current_surface,current_sector,runtime_fence,last_presence_at; только Planet выполняет UPDATE и транзакционно меняет current_sector при переходе. Chat имеет только SELECT. Для spatial send Chat внутри своей короткой DB TX делает SELECT ... FOR SHARE по user_id, подтверждает state=active, свежесть last_presence_at <=2s и нужный sector/surface. Конкурирующий UPDATE Planet при переходе/leave сериализуется. Проверять runtime lease и health Planet. Схема не существует в текущем P2-3, до P2-4+ недоступна.
- Контракт использует row-lock fence на той же Planet-owned строке для send: без двойных обращений HTTP и без второго хранения географии. Если Planet в будущем изменит model visit, сохранить этот инвариант посредством эквивалентной одной authoritative строки/версии. Chat не пишет в planet_visits и не выдумывает location.
- Если Planet недоступен/active presence отсутствует → недоступны только Planet/Sector. Остальные разделы не ограничиваются (при доступном Cosmic/Chat).
- Пока P2-4/5 не умеют достоверный визит, feature flag spatial_chat=false. Никакого чтения legacy planet_visits для допусков.

## 5. Удаление корпорации: идемпотентная схема

Важное изменение текущего internal/corporations/commands.go: существующий company_dissolve делает мгновенный refund + users/stations updates + DELETE companies. С Chat Service это станет нарушением продуктового требования «не завершать до подтверждения очистки».

**Предпочтительная durable state machine:**
1. Cosmic получает command с operation_id UUID, проверяет прежние экономические/планетарные условия и владельца компании. В одной короткой SQL TX блокирует companies и фиксирует **deleting** (новый Cosmic-owned флаг companies.chat_deleting boolean default false либо строго эквивалентный state) + INSERT chat_company_deletion_requests(company_id,operation_id,state=deleting). **НЕ** делать refund, обновление members/stations или DELETE компании на этом шаге. COMMIT. Все Cosmic corporate mutators отвергают дальнейшие изменения компании со статусом deleting.
2. Chat worker видит запрос, проверяет operation_id, immediately закрывает членам доступ и отправку (также каждый send/read проверяет companies.chat_deleting). Идемпотентно очищает только её corporation channel: messages, locators, read marks, mutes, связанный channel. Избранное НЕ очищает. Частичная очистка допускается лишь после закрытия доступа и с durable progress; при crash повтор продолжает. В транзакции завершения Chat INSERT/UPSERT chat_company_cleanup_receipts(company_id,operation_id,cleaned_at,channel_deleted=true). Никаких чужих каналов.
3. Cosmic по receipt с **совпадающим operation_id** в одной короткой SQL TX снова сверяет companies/deleting и выполняет исходное завершение company_dissolve: однократный refund, users company_id=NULL/role=NULL, stations detach и DELETE companies. Обновляет state=finished durable. Если TX откатилась, refund/удаление не произошло. При COMMIT повтор не должен возвратить деньги дважды.
4. Crash Cosmic/Chat/DB на любом шаге → скан незавершённых requests/receipts на startup и timer; повтор с тем же operation_id возвращает состояние, иной operation_id на компанию — conflict. Ни одной SQL TX через удалённый HTTP.

Возможно не требовать отдельного HTTP вызова в критическом пути: таблицы request и receipt в **той же PostgreSQL**, права записи разделены по владельцам. Worker polling или outbox hint достаточно. Не использовать BEFORE DELETE trigger на companies.

**Слой блокировок:** на send Corporation коротко блокировать companies row в режиме, конфликтующем с записью состояния deleting, и проверять статус перед commit. Codex обязан привести Cosmic join/leave/company delete и Chat к общему порядку блокировок и протестировать отсутствие deadlock на PostgreSQL.

## 6. Удаление планеты / поверхности

В текущей архитектуре Cosmic может переводить surface в retiring. Это должно выключать новые Planet/Sector чаты для surface (даже если у пользователя старый visit). Перед удалением planet_surfaces/planet_sectors необходимо очистить все соответствующие chat_channels/messages/locators, иначе FK RESTRICT должен **отклонить** удаление географии. Спроектировать аналогичную пару Cosmic-owned chat_surface_cleanup_requests и Chat-owned chat_surface_cleanup_receipts либо расширить существующий P2-3 retirement protocol таким же durable ack. Независимые Favorites остаются. Никогда не использовать ON DELETE CASCADE, которое оставляет невидимые orphan events или удаляет favorites.

## 7. Физическое удаление, партиции, аудит и аварии

- Moderator-delete: закрывает original, чистит все source-linked favorites, deletes message+locator+receipts, сохраняет только reason+IDs и action в chat_audit, COMMIT. При обычном author-delete и expiry saved copies остаются.
- Partition retention <=10m рабочей задержки + read-time TTL check. Старые партиции DROP после очистки locator и подтверждения пустоты; нельзя DROP whole weekly partition раньше его самых свежих сообщений. Секции будущее окно создаются до наступления даты.
- chat_reports не держат тело исходника; при review после истечения оригинала честное «недоступно». Log/outbox/notifications никогда не сохраняют удалённый body. Если физическая копия пережила deletion в backup/WAL, обещание мгновенного полного стирания недействительно. При restore прогнать cleanup до открытия API; хранение копий определяет отдельный операционный регламент из 04.
- Блокировка зависимостей: Cosmic down all; Planet down only spatial; Chat down all. Система не принимает сообщения «на доверии» при тайм-ауте авторизации.
- Operations TTL и request_id: обеспечить минимум 48h защиты повторов; после expiry операция той же UUID не должна приниматься без контроля replay/recent message IDs, иначе нужно прямо документировать границы гарантии и дать клиенту восстановление через историю.

## 8. Acceptance / негативные / конкурентные / crash тесты (обязательно)

**Доступ:** неавторизованный и bot reject; чужие Private/history/search/Favorites запрещены; исключённый участник не видит даже старую историю; company leave немедленно отзывает доступ, новый member видит сохранённую history; чужая planet/sector selection не действует; переход сектора меняет канал; Cosmic outage all vs Planet outage spatial; админ-аудит закрыт корпоративному owner.

**Приглашения:** DM A→B+B→A создаёт один активный диалог; rejected/expired invite не принимается; cooldown 24h, rolling 20/24h, pending unique; conversion A/B→C сохраняет старые messages и происходит без согласия B; C не видит до accept; DM после conversion можно открыть заново; group cap 50, active после первого accept; pending group без приглашённых messages; group=2 остаётся группой; при 1 автоматически delete; owner leave с >=2 требует manual transfer; excluded/rejoined видит history.

**Сообщения:** codepoint lengths 256/500/2500; 1/3s,1/2s,1/1s,1/1s,10/10s; смена scope не сбрасывает rate; ровно одно message+ACK при duplicate request_id; другие body+same ID → conflict; body plain text XSS-safe; reply после удаления показывает unavailable; срок хранения от sent_at; unicode search/russian/english/emoji; 50-page cursor без пропусков/дубликатов.

**Прочтение:** не отмечать unread при background/непросмотренных; вновь вступившие видят старую историю с unread=0; receipt idempotent, blue после прочтения хотя бы одним, group recipients не все; unread не отрицательный после удаления/TTL; invite offline и Private unread воспроизводятся после перезапуска.

**Избранное/жалобы/модерация:** author-delete не стирает чужую saved copy; moderator-delete стирает все source-linked duplicates и original; race save vs mod-delete не оставляет копии; folder delete сохраняет записи в «Все»; report с чужого message reject; report без body, 180d audit; модератор не может mute главу/себя/коллег; owner mute moderator, allowed durations, reason required; mute допускает чтение и запрещает send; unmute logged.

**Конкурентность/отказы:** accept vs expiry; send vs kick/delete; transfer vs leave; двух владельцев не бывает; две WS leases старый жив → новый reject; stale lease → reconnect; прежний connection после takeover не пишет; DB failure during send = rollback; drop WS after COMMIT + replay=one message; LISTEN/NOTIFY loss не теряет unread; company dissolve crash после каждого из 3 этапов, без двойного refund и без оставшегося Corp channel; surface retiring prevents old spatial sends; partition job restart/re-run no cross-channel deletion; recovery after backup and retention cleanup.

**Нагрузочные проверки:** 50 concurrent WS clients, mixed Global/Corp/Private + search, 50-member group, heartbeat/reconnect, p95/p99 latency и DB queries, EXPLAIN (ANALYZE, BUFFERS) ключевых индексов; no memory leak, no goroutine leak, no client-side privilege escalation. Go race/vet/integration Postgres и Windows x64 cross-build.
