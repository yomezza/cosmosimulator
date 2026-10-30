# Chat Service — PostgreSQL: схема, индексы, партиции, процедуры

Версия: проект DDL 1.0, 2026-10-30. Задача Codex: преобразовать спецификацию в **проверяемые миграции** после schema170.sql, не исполняя DDL из Chat Service при старте.
Основы: [01](./01-idea-v1.ru.md), [02](./02-approved-decisions.ru.md), [03](./03-architecture-integration.ru.md).
**Обязательное именование:** все новые обычные таблицы, секции, индексы, функции, триггеры и представления — с префиксом chat_. Существующие users/companies/planet_* НЕ переименовывать.

## 1. Типы, ключи и владение данными

- Все новые chat_* объекты создаются в существующей DB_SCHEMA, а не в public по умолчанию. Мигратор использует тот же quoted search_path, advisory transaction lock 784216; новый chat_schema_state(id=1,version) проверяется при старте Chat без авто-миграции.
- Приложение использует UUID, сгенерированные криптографически в Go, для каналов/бесед/сообщений/операций/приглашений/жалоб. Не требовать extension uuid-ossp. В протоколе BIGINT передавать **десятичной строкой** для JavaScript.
- Все времена TIMESTAMPTZ в UTC на сервере; timezone клиента только для показа. Никаких локальных времён, таймеров от браузера.
- Chat DB user: SELECT по необходимым полям users/sessions/companies/company_roles/planet_* (последнее только после согласования), DML только по Chat-owned таблицам. Cosmic DB user: DML по своей таблице chat_company_deletion_requests и chat_game_moderator_grants; SELECT по подтверждениям Chat. Planet не меняет chat_*.
- Не вводить FK ON DELETE CASCADE от Избранного к первоисточнику. Не хранить тело сообщений в журнале событий, outbox, жалобах или idempotency-ответах.

## 2. Таблицы v1: полный перечень и ограничения

| Таблица | Столбцы/ключи (обязательная логика) | Индексы сверх PK |
|---|---|---|
| chat_schema_state | id SMALLINT PK CHECK(id=1), version INT | — |
| chat_conversations | id UUID PK; type TEXT CHECK(dm/group); state TEXT CHECK(pending/active/deleting); owner_id BIGINT; title TEXT; created_at,activated_at; revision BIGINT | (owner_id,state); при group — название до 100 Unicode codepoints (проектный UI limit) |
| chat_dm_pairs | low_user_id BIGINT,high_user_id BIGINT CHECK(low<high), conversation_id UUID UNIQUE FK; PK(low_user_id,high_user_id) | уникальность защищает встречные заявки и один активный DM |
| chat_channels | id UUID PK; kind TEXT CHECK(global/planet/sector/corporation/private); surface_id BIGINT NULL, sector_number INT NULL, company_id BIGINT NULL, conversation_id UUID NULL UNIQUE; next_seq BIGINT NOT NULL DEFAULT 0; created_at; CHECK допустимого набора scope | UNIQUE kind WHERE global; UNIQUE(kind,surface_id) WHERE planet; UNIQUE(kind,surface_id,sector_number) WHERE sector; UNIQUE(kind,company_id) WHERE corporation; UNIQUE(kind,conversation_id) WHERE private |
| chat_members | conversation_id UUID, user_id BIGINT; status TEXT CHECK(active/left/kicked); joined_at, left_at; join_floor_seq BIGINT; unread_count INT CHECK>=0; PK(conversation_id,user_id) | (user_id,status,conversation_id); (conversation_id,status) |
| chat_invites | id UUID PK; conversation_id UUID; sender_id, recipient_id BIGINT; status TEXT CHECK(pending/accepted/rejected/expired/cancelled); created_at, expires_at, decided_at; unique pending invite on (conversation_id,recipient_id) via partial index | (recipient_id,status,created_at DESC); (status,expires_at) partial pending |
| chat_invite_attempts | id UUID PK; sender_id,recipient_id; conversation_id UUID NULL (без FK, живёт после удаления draft); attempted_at; decision TEXT; rejected_at NULL. Не хранить текст переписки | (sender_id,recipient_id,attempted_at DESC); (sender_id,recipient_id,conversation_id,rejected_at DESC) |
| chat_invite_rate_locks | sender_id BIGINT, recipient_id BIGINT; PK(sender_id,recipient_id). Короткоживущий mutex для точного rolling limit 20/24h | (sender_id,recipient_id) PK; GC не удаляет строку, пока есть конкурирующая операция |
| chat_rate_state | user_id BIGINT, kind TEXT (5 sending kinds), last_sent_at TIMESTAMPTZ; PK(user_id,kind) | — |
| chat_rate_events | user_id BIGINT, kind TEXT CHECK(private), sent_at TIMESTAMPTZ, operation_id UUID; PK(user_id,operation_id) | (user_id,kind,sent_at DESC); GC >11s c safety margin |
| chat_messages | **PARTITION BY LIST(kind), затем каждый kind PARTITION BY RANGE(sent_at)**. id UUID, kind, sent_at TIMESTAMPTZ, expires_at TIMESTAMPTZ, channel_id UUID, seq BIGINT, author_id BIGINT, origin_sector_number INT NULL (в Planet это сектор на момент отправки), body TEXT, format TEXT=plain, reply_to_id UUID NULL, message_type TEXT CHECK(user/system), system_code TEXT NULL, system_data JSONB NULL; PK(kind,sent_at,id) | on each partition: (channel_id,seq DESC), (expires_at,id), pg_trgm GIN body (только user) |
| chat_message_locator | id UUID PK; kind, sent_at, channel_id, seq, author_id, expires_at; UNIQUE(channel_id,seq). **Не содержит текста**, ссылочной целостности с chat_messages через FK нет (поддержка транзакцией и reconciliation) | (channel_id,seq DESC); (expires_at,id) |
| chat_read_receipts | user_id BIGINT, message_id UUID REFERENCES chat_message_locator(id) ON DELETE CASCADE, read_at TIMESTAMPTZ; PK(user_id,message_id). Только Private; хранить метки прочтения даже после выхода участника до TTL сообщения | (message_id,read_at); (user_id,read_at DESC) |
| chat_read_marks | user_id,channel_id, last_seen_seq BIGINT; PK(user_id,channel_id). Для Global/Planet/Sector «новое», корпорации и курсора отображения; Private exact-unread ведёт receipts+members | — |
| chat_folders | id UUID PK, owner_id BIGINT, name TEXT, position INT,created_at | (owner_id,position,id); UNIQUE(owner_id,name) по выбранной case-normalization |
| chat_saved_items | id UUID PK, owner_id BIGINT, folder_id UUID NULL, kind TEXT CHECK(note/copy), body TEXT <=2500 Unicode codepoints, author_id BIGINT NULL, author_name_snapshot TEXT NULL, original_at,source_kind,source_message_id UUID NULL,source_channel_id UUID NULL,surface_id,sector_number, saved_at,updated_at; нет FK на сообщения | (owner_id,saved_at DESC,id DESC); (owner_id,folder_id,saved_at DESC); (source_message_id) WHERE NOT NULL |
| chat_user_settings | user_id BIGINT PK; selected_tab, selected_conversation_id, compact, position_x/y, size_w/h, hidden_tabs JSONB, sound JSONB, updated_at; валидация JSONB/чисел в Go | — |
| chat_corporation_mutes | company_id BIGINT, user_id BIGINT, imposed_by BIGINT, reason TEXT, starts_at, expires_at, lifted_at NULL; id UUID PK; ограничение готовых сроков 5m/30m/1h/24h/7d в Go + DB accepted enum | UNIQUE(company_id,user_id) WHERE lifted_at IS NULL AND expires_at>... **нельзя** делать time-dependent partial index; см. ниже |
| chat_reports | id UUID PK; reporter_id BIGINT, source_message_id UUID (без FK), source_kind, source_channel_id, reason TEXT, created_at, state TEXT CHECK(open/in_review/resolved/rejected), reviewer_id, resolved_at; **без body текста** | (state,created_at,id); (reporter_id,source_message_id) UNIQUE с разумной антидубль-политикой |
| chat_audit | id BIGSERIAL PK; actor_id, subject_id,company_id, action,reason,reference_id UUID NULL,created_at TIMESTAMPTZ, metadata JSONB без текста | (created_at,id); (company_id,created_at DESC); (subject_id,created_at DESC) |
| chat_outbox | id BIGSERIAL PK; event_kind, channel_id UUID NULL, target_user_id BIGINT NULL, reference_id UUID NULL, created_at, processed_at NULL; **только IDs/метаданные** | (processed_at,id) WHERE processed_at IS NULL; (created_at) |
| chat_operations | user_id BIGINT, request_id UUID, action TEXT, payload_digest BYTEA(32), result_ref UUID NULL, result_code TEXT, created_at,expires_at; PK(user_id,request_id) | (expires_at) для GC |
| chat_ws_leases | user_id BIGINT PK,connection_id UUID, server_instance UUID,expires_at, renewed_at | (expires_at) |
| chat_company_deletion_requests | company_id BIGINT PK, operation_id UUID UNIQUE, initiated_at TIMESTAMPTZ, state TEXT CHECK(deleting/finished); запись только Cosmic. Ссылку на companies не каскадировать: receipt нужен после удаления | (state,initiated_at) |
| chat_company_cleanup_receipts | company_id BIGINT PK, operation_id UUID UNIQUE, cleaned_at, channel_deleted BOOLEAN; запись только Chat | (cleaned_at) |
| chat_surface_cleanup_requests | surface_id BIGINT PK, operation_id UUID UNIQUE, initiated_at, state TEXT CHECK(retiring/finished); запись только Cosmic; без каскадного FK | (state,initiated_at) |
| chat_surface_cleanup_receipts | surface_id BIGINT PK, operation_id UUID UNIQUE, cleaned_at; запись только Chat | (cleaned_at) |
| chat_game_moderator_grants | user_id BIGINT PK, granted_by BIGINT, granted_at, revoked_at NULL; Cosmic-owned; пользователи с users.role=admin имеют доступ без этой записи | (revoked_at) |
 
Дополнительные данные: chat_channels.company_id → companies.id через RESTRICT, chat_channels.(surface_id,sector_number) → planet_sectors через RESTRICT (когда sector), surface_id → planet_surfaces при planet. chat_conversations.owner_id должен указывать на действующего участника, проверка в транзакционном use case (не циклический FK). Для chat_corporation_mutes применить индекс (company_id,user_id,lifted_at,expires_at), а уникальность активного наказания поддерживать сериализованной транзакцией с блокировкой company/user и явным завершением предыдущего активного наказания; функция в индексе не может зависеть от now(). Удалённые аккаунты могут потребовать отдельной политики обезличивания архивных author_id: не включать непроверенные CASCADE в историю/Избранное.

### 2.1. Инварианты chat_channels

- Global — ровно один канал. Planet — один на сохранённую canonical surface; Sector — один на (surface, canonical number). Corporation — один на company; Private — один на conversation.
- Scope NULL CHECK исключает случайное присваивание сразу surface и company. Scope-ключи из клиента игнорируются; при чтении scope резолвится только по проверенному местоположению.
- Для последовательности сообщений chat_channels.next_seq увеличивается внутри транзакции отправки (UPDATE ... RETURNING). Это даёт монотонное упорядочение **внутри канала**, независимо от timestamp/UUID, и удобные курсоры.
- Удаление внешних объектов сначала завершает согласованную очистку их каналов, затем Cosmic/Planet удаляют master-данные. FK RESTRICT служит последней защитой от потери целостности, но **не заменяет** workflow.

## 3. Ограничения сообщений

- Сроки (интервалы от **server sent_at**): sector 7d, global 30d, planet 30d, corporation 90d, private 180d. Favorites без TTL. При создании expires_at вычисляет SQL-логика, не клиент.
- Длина: global/planet/sector/corporation 256 Unicode codepoints; private 500; favorite note/copy 2500. char_length в PostgreSQL и utf8.RuneCountInString в Go согласовать на тестах (ZWJ-эмодзи может быть многокодовым).
- Не допускаются пустые/состоящие только из пробелов user messages; нормализовать CRLF, контролировать NUL, максимальный размер запроса и максимальную длину игровых ссылок. HTML/URLs клиента не исполняются; при рендере использовать textContent/DOM.
- reply_to_id может указывать на ранее удалённое сообщение; FK на live-оригинал намеренно нет. При создании ответа оригинал **должен существовать и быть доступен** пользователю в том же канале; при дальнейшей очистке выводится «Исходное сообщение недоступно».
- system messages: body пуст или короткий фиксированный шаблонный код с whitelist metadata; доступны только разрешённой аудитории; TTL своего канала. Закрытые причины модерации не помещать в общедоступную corporate-ленту.

## 4. Реальное партиционирование — не по каждому каналу

Нужны 5 дочерних LIST-секций chat_messages_global / planet / sector / corporation / private с RANGE(sent_at):
- Sector: **дневные** секции, по UTC.
- Global и Planet: **дневные** секции.
- Corporation и Private: **недельные** секции с UTC-недели (понедельник).
- Секции создаются на 14 дней вперёд для дневных и на 8 недель вперёд для недельных; worker ежедневно поддерживает окно. При невозможности создать секцию **не писать в default** бесконечно: операция возвращает безопасную retryable ошибку, мониторинг сообщает об отсутствии будущих секций.
- PostgreSQL partitioned PK обязан включать **все ключи партиционирования**: (kind,sent_at,id). Глобальную уникальность id обеспечивает **непартиционированная** chat_message_locator PK(id), вставленная в одной tx до chat_messages. Не объявлять несовместимый UNIQUE(id) на partitioned parent.
- Полный текст в PostgreSQL искать через pg_trgm GIN на lower(body) / ILIKE по разрешённому scope. Если расширение отсутствует, мигратор должен явно сообщить и предложить согласованный администратором шаг установки; не создавать тихий дорогой поиск по всей БД. Создание индексов на каждую секцию/attach проверяется тестами версии PostgreSQL.
- Курсовая пагинация: channel_id + seq < cursor, ORDER BY seq DESC LIMIT 50; при открытии последняя страница, предыдущие по требованию. Поиск по разрешённым channel IDs, временному диапазону (не шире TTL), LIMIT 50, max query length 128, тайм-аут запросов и rate cap.

## 5. Ретенция и физическое удаление

Worker в Chat Service, **SQL DML выполняется PostgreSQL**:
1. Каждые 10 минут для каждого kind удаляет порциями до 1000 просроченных chat_messages по expires_at <= clock_timestamp(), и соответствующие chat_message_locator в **одной короткой tx**; receipt каскадируется по locator; обновляет Private unread_count для удалённых непрочитанных сообщений.
2. Когда верхняя граница всей секции + TTL <= сейчас, работник после проверки отсутствия живых сообщений может **DETACH/DROP** эту секцию; сначала удаляет/очищает относящиеся locator и receipts, чтобы не оставить «живой» указатель. DDL в отдельном контролируемом этапе; никакой транзакции с удалёнными вызовами.
3. Для удалённого автором/модератором конкретного сообщения независимо от секции: lock locator FOR UPDATE → проверить права → (если mod-delete) DELETE saved copies WHERE source_message_id=... → DELETE messages по (kind,sent_at,id) → DELETE locator → UPDATE unread/emit outbox/audit → COMMIT. При авторском удалении избранное НЕ затрагивать.
4. При удалении целого Private/Corporation: сначала запретить доступ/отправку, затем порционно или транзакционно удалить все messages + locators + receipts, invites/read state/members/channels; **Избранное не трогать**. Система должна выдержать повтор операции после crash, без повторного удаления чужого канала.
5. Все новые отправки/сохранения из оригинала сериализуются с удалением через lock locator: модерационное удаление либо видит уже созданную копию и удаляет её, либо сохранение получает not_found. Это закрывает race «успел сохранить после удаления».
6. Чистка chat_invite_attempts после >=48h (24h rate/cooldown + запас), expired invites — по 7d, chat_rate_events — после >11s, старых chat_operations — не раньше 48h (replay после этого интервала не гарантируется, endpoint требует nonce + отдельный ограничитель), outbox — после подтверждённой обработки и max7d, chat_audit — 180d. Reports: метаданные до разрешения + 180 дней после закрытия, без тел сообщений; задать отдельную job.
7. Бессрочные Избранное и пользовательские папки **не партиционировать по времени**.

**Важная деталь:** партиции не дают посекундную TTL автоматически. Поэтому **адресное удаление истёкших строк плюс DROP пустой секции**, а не хранение просроченных записей до конца недели. Целевой lag <=10 минут, аварийный лимит — alert при lag >30 минут. В дополнение к worker каждый read/send/search проверяет expires_at > now(), чтобы просроченный текст не был доступен в интервале до физической уборки.

## 6. Что делать в БД, а что в Go

### В БД оправдано

- CHECK/FK/UNIQUE/partial unique, атомарный next_seq, transaction-level locks; разграничение ролей DB и GRANT.
- Узкая SQL-функция chat_message_expiry(kind,sent_at) с корректной volatility (обычно STABLE для TIMESTAMPTZ+INTERVAL; не объявлять IMMUTABLE без проверки): маппинг на 7/30/90/180d. Исключает дублирование срока в нескольких Go-методах. Сроки считать как фиксированную продолжительность часов (168/720/2160/4320), не календарными месяцами и не локальными DST-днями.
- Узкая SQL-функция chat_take_send_slot(actor,kind,at,operation_id) или транзакционный SQL в репозитории: блокирует строку chat_rate_state на (actor,kind); для Private считает chat_rate_events за (at - 10 seconds,at], максимум 10; для остального compare с last_sent_at (global 3s, planet 2s, sector 1s, corporation 1s). Изменяет счётчик **в той же tx, что и INSERT**. При ROLLBACK лимит не тратится.
- Узкая SQL-функция chat_take_invite_slot(sender,target,now) / repository tx: lock строки chat_invite_rate_locks(sender,target) (INSERT ... ON CONFLICT DO NOTHING, затем SELECT FOR UPDATE), подсчитать chat_invite_attempts за скользящие 24 h (макс 20), соблюсти 24h cooldown отказа для (sender,target,conversation), уникальность pending и 50 мест. При сбое rollback.
- chat_check_scope_channel() CHECK/trigger только для инварианта конфигурации канала, который невозможно выразить обычным CHECK (если понадобится). **Не** делать SQL-триггер, который на каждой вставке дергает чужой сервис.
- Допустим один лёгкий AFTER INSERT trigger chat_messages_enqueue_event для **идентификатора**, без копирования body; предпочтительнее явный INSERT chat_outbox в транзакции приложения — так меньше скрытых действий. Выбрать **один** способ, не оба.
- SELECT VIEW chat_channel_scope_summary (не security boundary) для служебной диагностики; пользовательские ACL всегда проверять до выдачи данных. Ни публичных views без user context, ни SECURITY DEFINER «обхода прав».

### В Go обязательно

- Game/UI правила, session+Origin, Cosmic/Planet health, Planet presence, corporate/DM permissions, приглашения, управление владельцем, модерация, упоминания, ссылки, поиск с ACL, unread visibility, транспорт WS.
- Воркеры по расписанию, идемпотентное удаление, partition lifecycle с миграционным advisory lock, уведомления/listen-notify, диагностика.
- Сложные операции управлять **короткой tx** в Go. Не превращать DB в монолит бизнес-логики из десятков триггеров, управляющих пользователями/корпорациями/внешними HTTP.

### Триггеры, которые НЕ нужны

- Автоматическая «проверка Planet-сектора» внутри БД (для этого нет надёжного DB snapshot).
- Массовые DELETE из Избранного на любой DELETE сообщения (нарушает продуктовый замысел). Только специальная транзакция модерации.
- Автоматическое удаление компании из BEFORE DELETE trigger (невозможны гарантии второго Go-сервиса и восстановление).
- Скрытый триггер, самовольно меняющий корпоративные права, session или создателя беседы.

## 7. Надёжность, обслуживание и бэкапы

- Один scheduler для chat cleanup с advisory lock по DB_SCHEMA, чтобы перезапуск/второй процесс не запустил уборку параллельно. Идемпотентность; job может возобновиться без перемещения expiry.
- Сессии/операции не хранят plaintext session tokens. Не включать тексты удалённых сообщений в WAL логическую репликацию, application logs, exception text, диагностические дампы и metrics labels.
- Физический DELETE в активной PostgreSQL **не стирает уже созданные backup/WAL**. Рабочее предложение для операционного регламента: зашифрованные резервные копии на 14 дней, WAL retention не больше 7 дней (точное значение согласовать с RPO/RTO), после restore немедленно запускать блокирующий доступ cleanup до открытия чата и прогонять список pending moderator removals. Нельзя обещать немедленное стирание из копий. Проверить восстановления и политику хранения до запуска.
- Мониторинг: message count по kind, p95 send/search, DB pool, failed health, unread reconciliation, pending deletion jobs, expiry lag, size/index bloat, partition future horizon, outbox lag, stuck WS lease, DB deadlocks.
- Миграция с rollback безопасна: **данные не терять из-за версии**; additive DDL, backward incompatible change только с feature flag, backfill и проверкой.
- Не создавать schema с неустановленным pg_trgm и «как-нибудь потом»; готовность поиска входит в readiness v1.

## 8. Два уточнения для Codex по реализации DDL

1. Это **схема-спецификация, а не исполняемый SQL**. При написании SQL тщательно проверить PostgreSQL запреты на partial UNIQUE с переменной now(), составную PK партиций, FK с partitioned таблицами, ALTER/cascade и создание/attach partitioned indexes. Тестировать реальную PostgreSQL, а не SQLite.
2. Уникальные индексы chat_channels по kind (WHERE ...) должны реально обеспечивать один scope независимо от NULL; не использовать стандартный UNIQUE(kind,surface_id,sector) без partial conditions, поскольку NULL не равен NULL.
