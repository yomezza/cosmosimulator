# Chat Service — точные машины состояний и пошаговые алгоритмы (Codex)

Техническая спецификация v1.1, 2026-10-30. Актуальные приоритеты: согласованный продукт 01/02, аудит 08, уточнённая БД 09, затем 03–07. Цель: разработчику не нужно придумывать порядок конкурентных операций и ветви ошибок. Здесь не добавляются игровые правила.

## 1. Основной шаблон application operation

Все изменяющие операции используют одну внутреннюю схему:

1. Transport читает не более предельно допустимого JSON/WS frame, валидирует схему, cookie и Origin, устанавливает authenticated user ID. В режиме v1 мутации от браузера выполнять **только на действующем Chat WS**, чтобы HTTP не обходил single-session.
2. Проверить service availability: Cosmic + Chat всегда, Planet дополнительно для spatial. Actor не bot, session не expired.
3. AccessResolver определяет разрешённый scope из authoritative DB или Planet-owned visit, но не из browser-provided company/surface/sector.
4. BEGIN short SQL TX. Заблокировать chat_ws_leases(user_id) FOR SHARE и проверить connection_id/generation/expires_at — stale clients больше не мутируют.
5. Заявить идемпотентность в chat_operations. Если operation уже completed с тем же payload digest — вернуть тот же result_json без повторения действий; другой payload для того же request_id → conflict.
6. Взять **только нужные** lock по зафиксированному порядку. Повторить mutable business checks в той же TX (membership, conversation.state, owner, role/mute, current visit). Если с момента предварительной проверки scope поменялся — access_denied/conflict.
7. Провести SQL изменения; операция rate/reservation/notification/audit входит в ту же TX. COMMIT. Сформировать ACK только из immutable result_json после commit.
8. Outbox dispatcher после commit публикует event; на потере событий клиент восстанавливается через bootstrap/history/cursors. База, а не WS, единственный источник истины.

**Общий порядок row locks:** WS lease → DM canonical pair lock (только операции DM) → Cosmic company или Planet visit (если нужны) → chat_conversations → chat_members/invites в возрастающем user_id → chat_channels → rate locks → message_locator по id → receipts/saved/notifications/outbox. Ряд операций не захватывает лишние классы. Любой worker, который меняет unread/удаляет messages, должен следовать тому же порядку и не брать родительские locks ПОСЛЕ locator. Ни одной сетевой операции внутри SQL TX.

## 2. Состояния: однозначные таблицы переходов

### 2.1. Conversation

| Исходное | Событие / предусловия | Итог | Побочные эффекты |
|---|---|---|---|
| отсутствует | create DM: нет pair, valid recipient | pending DM | pair+invite, owner creator, no messages |
| pending DM | адресат принимает до expires_at | active DM | 2 active members; channel; unread=0 |
| pending DM | встречное приглашение второй стороны до expires_at | active DM | то же без второй беседы |
| pending DM | decline/expiry единственного invite | deleting → отсутствует | закрыть invite/pair; хранить cooldown отдельно |
| active DM | creator приглашает третьего | active DM | conversion invite pending, переписка продолжается |
| active DM | приглашённый third принимает | active group | убрать dm_pair, добавить member, history/channel ID НЕ менять |
| active DM | любой участник leave | deleting → отсутствует | немедленный deny, физический purge |
| отсутствует | create group | pending group | 1 creator, писать запрещено |
| pending group | первый accept | active group | >=2 active, остальные invites сохраняются |
| pending group | все invites истекли/отклонены | deleting → отсутствует | отменить остальные и удалить черновик |
| pending group | 7 дней без единого invite | deleting → отсутствует | предложенный технический TTL пустого черновика, без сообщений |
| active group | join нового принятого invite | active group | active count+1, join_floor=current_seq, unread=0 |
| active group | member leave/kick, active count после >=2 | active group | член потерял доступ; history сохраняется |
| active group | leave/kick, active count после =1 | deleting → отсутствует | revoke всем, purge messages, pending invites cancel |
| active group | rename/owner transfer | active group | revision+1, одно system event |
| active group | delete-for-all owner | deleting → отсутствует | немедленный deny, purge, completed only after purge |
| deleting | любая операция кроме cleanup/status | deleting | gone, никаких новых message/invite/owner |
| active group | автоматический TTL старых messages | active group | только messages удаляются, беседа продолжает существовать |

**Обязательные свойства:** тип group не возвращается в DM; недействительный invite не возрождает deleting; у group active >=2; когда один остаётся после ухода, группа удаляется; в DM может существовать максимум одна активная или ожидающая беседа canonical pair.

### 2.2. Invitation

| Состояние | Событие | Результат |
|---|---|---|
| отсутствует | owner создаёт, capacity и rate доступны | pending, expires_at=created_at+7d |
| pending | recipient accepts до expiry и conversation valid | accepted, запись member, возможная активация/конверсия |
| pending | recipient rejects до expiry | rejected, blocked re-invite same conversation 24h |
| pending | server time >= expires_at | expired, автоматически без согласия адресата |
| pending | conversation deleting / owner отменяет | cancelled |
| accepted/rejected/expired/cancelled | повтор той же decision | сохранённый outcome либо gone, **не** новое решение |
| отсутствует/pending DM | recipient отправил встречную заявку | mutual accepted одного DM, без второго invite |

Между sender/target не более 20 **созданных** приглашений за скользящие 24h по всем conversations. Необоснованные reject по auth, лимиту, отсутствию target и idempotent retry не списывают квоту.

### 2.3. Membership и права владельца

- Статусы active/left/kicked. Повторный accept для left/kicked → active, joined_at обновляется, join_floor_seq=current channel seq, unread_count=0; прошлая история видна без нового unread.
- Group owner должен быть active; owner transfer меняет owner_id и revision одним update, получатель обязательно active.
- Owner leave: если группа остаётся после ухода как >=2 accepted members, new_owner_id обязателен; передача и leave атомарны. Если один остаётся — удаление беседы без передачи.
- Kick owner может на участника, отличного от себя, только если owner действующий. Последний kick вызывает deleting, не оставляет пустой группы.
- DM creator owner conversion инициирует сам; второй собеседник уведомлён, не имеет вето; третья сторона обязана принять.
- При inviter decline/expiry pending DM удалить pair only after cleanup; cooldown keyed by canonical pair и sender/recipient. Existing DM после conversion не блокирует новую пару.

### 2.4. Company deletion

| Фаза | Writer | Preconditions | Result |
|---|---|---|---|
| active → deleting | Cosmic | валидный owner/economic checks, уникальный operation_id | companies.chat_deleting=true и chat_company_deletion_requests |
| deleting → cleanup in progress | Chat | запрос от Cosmic уже committed | запретить Corp доступ и send; chunk purge channel/messages/locator |
| cleanup → cleaned | Chat | физически ничего не осталось в scope | receipt с тем же operation_id |
| cleaned → finished | Cosmic | receipt совпадает, компания всё ещё deleting | однократные refund, detach users/stations, DELETE company, отметить op done |
| crash в любой фазе | owner шага | durable DB state | повторить только незавершённые шаги; операции с другим id conflict |

**Тонкость:** не держать открытую SQL TX из Cosmic, ожидая Chat. Перевод deleting и подтверждение требуют независимых коротких коммитов. Строка companies при send защищает от записи после начала удаления.

### 2.5. Surface retirement

Cosmic помечает surface retiring и создаёт operation_id, Planet блокирует новые визиты и переводит живые в безопасные состояния согласно действующей планетарной машине, Chat запрещает spatial доступ и очищает все Planet/Sector каналы данной surface. Только после receipts Planet+Chat (и соблюдения иных инвариантов географии) Cosmic окончательно удаляет surface. Favorites independent copies остаются. Отдельные TX и повторное восстановление.

## 3. Точные транзакции сложных операций

### T01. Create DM / встречные приглашения

1. Valid target, requester not same target or bot; вычислить low/high.
2. BEGIN + lease/idempotency.
3. INSERT chat_dm_pair_locks(low,high) ON CONFLICT DO NOTHING; SELECT FOR UPDATE pair row. Проверить, не существует ли chat_dm_pairs.
4. Если существует active DM — вернуть existing conversation, новую invite НЕ создавать и квоту не списывать.
5. Если существует pending DM, где sender встречной заявки = первоначальный recipient, автоматически принять, активировать одну conversation/channel; update both member states; никаких двух invites.
6. Если pending того же sender → existing pending, повтор новой заявки не создаёт дубликат.
7. Если пары нет — взять chat_invite_rate_locks(sender,target) FOR UPDATE; применить rolling quota и ранее отказанный DM cooldown; INSERT pending conversation/pair/invite/attempt.
8. COMMIT. Возвратить same operation outcome; notification invite одному адресату.

### T02. Create group + invite + accept

1. Owner создаёт pending group с одним active member; первая invite не обязательна для самого CREATE, но пока нет accept writing запрещён.
2. INVITE: lease/idempotency; lock conversation → ordered members → invite_rate sender,target; проверить owner/current, recipient user active, member status не active, count(active + pending)<50, duplicate pending/cooldown/quota. INSERT invite+attempt и notification. Новая invite не меняет состояние группы.
3. ACCEPT: lease/idempotency; lock conversation → invite and member; validate recipient/session, now<expires_at, not deleting, capacity, owner still valid; set invited accepted, UPSERT active member joined_floor_seq, unread=0, group pending→active при первом accept, emit system join.
4. Decline/Expire: отметить invitation; если conversation.pending и **не осталось** pending, инициировать delete; иначе группа остаётся до следующих decisions.
5. При повторном вступлении — возобновить доступ к старой истории, но unread baseline заново с channel next_seq.

### T03. Convert DM → group

1. Только первоначальный creator/current owner DM отправляет invite C. До accept тип остаётся dm; B может продолжать переписку и авторски удалять своё.
2. При accept: BEGIN + lease; **сначала** блокировка canonical DM pair, потом conversation, затем invite/member. Check actor C, pending invite, expiry, A/B still active, type dm, state active, max 50.
3. В той же TX UPDATE conversations(type=group,revision+1), DELETE chat_dm_pairs, INSERT member C, INSERT system event, notification A/B/C. Сообщения в chat_messages НЕ меняются, channel_id и seq остаются прежними.
4. После COMMIT A/B вправе создать новый DM как отдельную conversation. При concurrent leave-before-accept accept видит deleting и отказывает; при accept-before-leave leave применяется уже к групповой беседе.

### T04. Send

1. Базовый шаблон lease/idempotency.
2. Проверить + заблокировать current scope: private conversation/member; Corporation company + role/mute current; Planet-owned active visit row, surface/sector и heartbeat.
3. Внутри TX валидировать body UTF-8/runes, mentions/links, reply_to original active+same channel, TTL; group pending/send запрещён.
4. Take rate slot from DB, channel next_seq update, single DB clock timestamp, INSERT locator + partition row with expiry. Private unread counters +1 для других active членов; notifications dedupe; outbox IDs only; operation.result_json; COMMIT.
5. ACK; duplicate request ID returns same ACK even если message позже expired/deleted, без повторного INSERT.

### T05. Read receipt

1. Client сообщает только фактически видимые message IDs; не «все до последнего» без подтверждения. Никакой автоматический read на доставку WS.
2. Lease/idempotency + private membership/active check (только свои receipts); lock conversation/member; read IDs только из этого conversation/channel, существующих non-expired messages. Сообщения до join_floor_seq тоже можно отмечать прочитанными для «синей точки», но они никогда не входят в unread_count после повторного вступления.
3. INSERT receipts ON CONFLICT DO NOTHING RETURNING newly; уменьшить unread_count на число **именно вновь прочитанных** пользовательских сообщений автора != actor. Старые сообщения до join_floor_seq могут получать read receipt, но unread_count не уменьшают.
4. COMMIT; author blue status derived EXISTS receipt(other user), не кешировать blue в messages.
5. После удаления original его receipt удаляется; пересчёт unread производится в рамках cleanup/job и reconciliation.

### T06. Save favorite copy / moderator-delete original

1. Сначала серверный ACL + scope locks по общему порядку; затем locator FOR UPDATE.
2. SAVE: original live, viewer имеет доступ; копировать body, author/ID/time/channel/scope в chat_saved_items, без FK на source locator; можно много копий.
3. MOD-DELETE: owner/moderator authority + обязательная reason, original live; сначала **вычислить затронутых непрочитавших адресатов по существующим receipts**, затем DELETE saved WHERE source_message_id=originalID, DELETE partition row, DELETE locator (receipts cascade), корректно уменьшить только их unread, audit metadata, outbox removal; COMMIT.
4. Если locator уже исчез, MOD-DELETE не создаёт скрытый archive; report остаётся metadata only. Если жалоба после истечения/удаления source — reject с not_found.

### T07. Delete Private conversation

1. lease/idempotency; если type=dm, захватить **canonical chat_dm_pair_locks до conversation** (не наоборот); затем lock conversation; определить owner/leave reason; изменить state=deleting и закрыть membership/access, создать durable chat_cleanup_jobs(scope,operation_id), отменить invites. COMMIT.
2. Если сообщений мало, разрешён путь удаления внутри той же TX с полностью подтверждённым физическим удалением. Иначе worker ходит bounded batches; финальный receipt/status Completed только при нуле сообщений/locators/receipts в scope; избранное не трогает.
3. Нельзя отправить client ACK «удалено навсегда» раньше физического завершения. Первое подтверждение: deleting, отдельный progress endpoint. Пока deleting, всем клиентам channel недоступен.
4. Restart worker: присваивать lease job с условным UPDATE, продолжить from durable cursor либо сканировать remaining locator, гарантировать идемпотентный DELETE и невоздействие на другие scope.

### T08. Corporate mute/unmute

1. Actor current member+moderator or game administration, target current member, target != actor (для corporate role), hierarchy; admin исключения отдельно.
2. Under company/member role checked + chat_corporation_mute_state row lock, validate allowed duration=300,1800,3600,86400,604800 seconds and reason nonempty.
3. Insert mute history; set current state; append audit. At send check company_id, user_id, now<expires_at, независимо от читательских прав. Unmute authorized owner/any corporate moderator or game moderator; set current=NULL and lifted_at in history + audit.
4. На смену member компаниями не переносить mute на другую corp. Внутри бывшей corp старое наказание не даёт прав или блокировок по чужой корпорации.

## 4. Ошибки: стабильный mapping

| Код | Возникает | Retry |
|---|---|---|
| auth_required | cookie отсутствует, expired, bot | после login |
| stale_connection | не та lease generation / другой WS | открыть Chat заново, старую вкладку нельзя продолжать писать |
| duplicate_chat_connection | ещё есть живая другая WS | закрыть старую вкладку |
| access_denied | нет current membership/presence/role | после смены игрового состояния |
| scope_changed | переход/смена company/sector между вводом и отправкой | выбрать актуальную ленту и подтвердить повторную отправку с новым request_id |
| conversation_pending | до первого accept | после accept |
| conversation_deleting | член покинул, auto/delete началось | запрещено |
| invitation_expired | время >= expires_at | новая invitation по правилам |
| invitation_cooldown | отказ менее 24h | ждать retry_after |
| group_capacity | 50 active+pending | после освобождения слота |
| rate_limited | нарушено окно сообщений/20 invites | после retry_after_ms |
| idempotency_conflict | request_id уже использован для другого payload | новый UUID на новое действие |
| dependency_unavailable | Cosmic/Chat/required Planet down | после health recovery |
| presence_unverifiable | Planet visit row/epoch несогласованны | после восстановления Planet |
| message_missing | expired/deleted/foreign message | без автоматического retry копирования |
| internal_error | сбой SQL/partition/worker | только idempotent retry, без раскрытия SQL |

Клиент не восстанавливает удалённую беседу после gone, не отправляет черновик автоматически после reconnect. Все UI ошибки локализуются RU/EN в уже существующем i18n.

## 5. Алгоритмы очистки/обслуживания

- Каждый query использует server TTL filtering, запрещая выдачу просроченного текста до physical worker cleanup.
- Worker expiry: lease+advisory lock; выбрать по времени batch, упорядочить по channel/locator; для Private брать conversation lock до locator, удалить rows/receipts, скорректировать unread. Повторять, пока backlog; метрика lag.
- Partition rotation: заранее создать daily/weekly future partitions узкой maintenance role; DROP только полностью истёкшей секции и после подтверждения, что locator не ссылается на её строки. Запись в missing future partition недопустима.
- Company/surface cleanup: по durable request закрыть доступ, очистить только затронутые chat_channels, а после счётчика remaining==0 выставить receipt и дать Cosmic завершить master deletion.
- Cleanup notifications/status: chat_notifications dedupe, скрыть/не выдавать payload, если target потерял ACL; удалить по операционному сроку. Без копирования body.
- Если read/cleanup/WS сталкиваются по locks — bounded retry на 40P01/40001; нет бесконечных циклов/ждущих транзакций.
