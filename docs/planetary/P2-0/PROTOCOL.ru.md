# WebSocket v1 — проект

Endpoint `/planet/ws/v1`, одна игровая управляющая WS-связь на аккаунт. Origin сверяется с allowlist **до upgrade**, session проверяется перед авторизацией; разрешённый Origin не заменяет аккаунт. TLS снаружи. Библиотеку WebSocket следует выбрать и зафиксировать в P2-1/6 (кандидат — maintained Go-библиотека coder/websocket); собственного RFC6455 parser или большого движка не писать.

Общий JSON-конверт: `{v:1,type:"...",seq:42,visit:"public-uuid",connection_epoch:3,payload:{...}}`. Новый hello имеет seq=1 и ещё не имеет visit/epoch. Неподдерживаемая версия отклоняется. Actor/visit public UUID не являются токенами авторизации. Новые DB ID в HTTP/WS сериализуются строками, чтобы JS не терял int64 precision.

## Клиент → сервер

| type | payload | Проверка |
|---|---|---|
| `hello` | `{ticket:"opaque"}` либо `{resume_visit:"uuid"}` | Один вариант; auth deadline 5 s; ticket не в query string |
| `move` | `{x_q:1234,y_q:5678}` | Целые, границы, актуальный visit/epoch, цель; не текущая позиция/скорость |
| `transition.confirm` | `{offer:"uuid",operation:"uuid"}` | Предложено этим сервером, TTL, тот же исходный сектор; права повторно |
| `transition.cancel` | `{offer:"uuid"}` | Только своё актуальное предложение, без перемещения |
| `leave` | `{operation:"uuid"}` | Дедупликация, сохранение, завершение визита |
| `resync` | `{last_server_seq:42}` | Только свой сектор, rate limited |

## Сервер → клиент

| type | payload |
|---|---|
| `welcome` | visit, connection_epoch, actor, current location, revision, server_time, rates; никаких session/ticket secrets |
| `sector.snapshot` | location id, canonical revision, public scene geometry/chunks, static objects, разрешённые площадки, персонажи текущего сектора; self position |
| `path.accepted` | command seq, path generation, waypoints, authoritative speed/start tick |
| `state.delta` | server seq/time, location revision, actor positions/facing/moving; нет приватных аккаунтов |
| `actor.enter` / `actor.leave` | public actor ID, nickname, sprite type, position / причина удаления |
| `transition.offer` | offer UUID, destination sector/owner label, TTL; не выдаёт вход без confirm |
| `transition.complete` | operation, новая location и snapshot revision; атомарное переключение |
| `visit.ended` | operation, reason (`return`, `permission_revoked`, `session_expired`, `control_revoked`), итог сохранения |
| `error` | related seq, стабильный code, безопасные параметры перевода, retryable; SQL/secret не включаются |

Пример:

```json
{"v":1,"type":"move","seq":8,"visit":"1b3b31cb-8844-4a8f-b094-80585b709e1c","connection_epoch":2,"payload":{"x_q":132096,"y_q":125952}}
```

Сервер не принимает от клиента `owner`, `admin`, `credits`, `speed`, готовый путь или время симуляции. `seq` монотонен в рамках connection epoch; reconnect начинает новый epoch и получает snapshot, не replay старого movement stream. Одна late команда со старым epoch не возвращает персонажа в предыдущую локацию.

## Сеть и нагрузка

Предлагаемые стартовые лимиты: входящее сообщение ≤4 KiB, не больше 10 move/s с коротким burst 20, один pending path на персонажа; handshake 5 s, ping 15 s, отсутствие pong 30 s. Значения проверяются нагрузкой и настройками P2-9, сейчас не объявляются измеренными.

Snapshot сцены отдавать сжатым HTTP/chunks до включения live stream; WS snapshot содержит метаданные/хеши и небольшую динамическую часть, не бесконечный монолит географии. Исходящие очереди ограничены: позиции можно объединять до последней; критические enter/leave/transition требуют порядка. Медленный клиент получает resync/закрытие, а не неограниченную RAM очередь. Присутствие транслируется только текущему сектору; область видимости внутри него добавляется по измеренной необходимости.

Reconnect с exponential backoff и jitter, ограниченный по времени/попыткам; затем явные «Повторить» и «Вернуться на корабль». Нет бесконечного reconnect loop при 403/несовместимой версии. Browser hidden приостанавливает render, но сервер остаётся владельцем движения; закрытая вкладка не запускает новую высадку.

Нормализованные коды для UI: `auth_required`, `ticket_expired`, `ticket_used`, `access_denied`, `geometry_changed`, `target_blocked`, `target_unreachable`, `path_budget_exceeded`, `offer_expired`, `runtime_unavailable`, `session_replaced`, `save_failed`. RU/EN строки живут в существующих каталогах. Ошибка или WS close не доказывают, что команда не была принята: состояние визита восстанавливается через авторизованный read model.
