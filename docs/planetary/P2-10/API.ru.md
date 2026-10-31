# Текущие HTTP и WebSocket-контракты

Браузер использует тот же origin космического терминала. Cosmic проксирует `/planet/*` по настроенному `PLANET_INTERNAL_URL`; игрок не выбирает upstream. Нужны действующая HttpOnly cookie `session`, точный разрешённый Origin для WS и право на текущую колонию. Роль администратора сама по себе не разрешает вход в чужую колонию.

## HTTP

| Endpoint | Назначение |
|---|---|
| `POST /api/planet/handoff` | Cosmic: `{operation:uuid,landing_site:string,ship:string}`. Проверяет прибытие/доступ и закрепляет конкретный корабль; выдаёт одноразовый ticket |
| `GET /api/planet/visit` | Cosmic: состояние собственного визита для resume/возврата |
| `POST /api/planet/return` | Cosmic: `{operation:uuid,visit:uuid}`. Устойчивый запрос завершения и освобождения reservation |
| `GET /planet/api/v1/scene?visit=uuid&epoch=int` | Planet: авторизованная публичная геометрия только текущей локации |
| `GET /planet/api/v1/health/live` | Процесс обслуживает HTTP; не гарантирует доступность БД/lease |
| `GET /planet/api/v1/health/ready` | Свежая проверка schema/lease/DB; 200 либо 503 с причиной |
| `GET /api/admin/metrics` | Агрегаты cosmic; только действующий администратор |
| `GET /planet/api/v1/metrics` | Агрегаты planet; только действующий администратор |

В scene: размер 256, `cells` — base64 байтов с public inside/water, `heights` — base64 little-endian uint16; `origin` и `materials` имеют координаты общей поверхности. `objects`, фактическая `landing` либо null и разрешённые `exits` описывают отображение. Seed, частные flags и геология не передаются. Рисунок грунта не является командой изменения географии/коллизий.

Ready текущей версии: `service=cosmosimulator-planet`, `version=0.23.0`, `stage=P2-9`, `schema_version=7`, `status=ready`, `gameplay_ready=true`. `lease_in_use` означает другой действующий runtime, `lease_lost` — потерю своего fencing, `schema_missing/schema_incompatible` — неподготовленную/несовместимую схему, `database_unavailable` — отсутствие свежего подтверждения БД. Не сводить все причины к «сервер онлайн».

## WebSocket v1

`/planet/ws/v1`. Cookie проверяется до Upgrade. Hello — за 5s, входящее сообщение ≤4096 bytes. Client frame:

```json
{"v":1,"type":"move","seq":2,"visit":"UUID","connection_epoch":1,"payload":{"x_q":120320,"y_q":131584}}
```

Сервер отвечает `{v,type,server_seq,visit,connection_epoch,payload}`. `seq/server_seq` монотонны в рамках связи, после нового подключения клиент начинает заново. Данные старого epoch не двигают нового персонажа. До активации error может не иметь visit/epoch. Координаты персонажа — целые Q=1024; surface DB ID в протоколе — строка.

| Клиентская команда | Payload |
|---|---|
| `hello` | `{ticket:opaque64}` либо `{resume_visit:uuid}`, seq=1, без visit/epoch |
| `move` | `{x_q:int,y_q:int}` — только цель, не положение или скорость персонажа |
| `resync` | `{last_server_seq:nonnegative-int}` — полный динамический snapshot, не чаще 1/s |
| `transition.confirm` | `{offer:uuid,operation:uuid}` |
| `transition.cancel` | `{offer:uuid}` |
| `leave` | `{operation:uuid}` |

Основные ответы: `welcome`, `sector.snapshot`, `actor.enter`, `actor.leave`, `state.delta`, `path.accepted`, `error`, `visit.ended`; переходы — `transition.offer/cancelled/complete`. PublicActor содержит actor UUID, nickname, sprite, координаты, moving/suspended, server_time и `facing`. State собственного персонажа дополнительно содержит remaining, command_seq/path_generation и при path.accepted маршрут. `facing`: S=0, SW=1, W=2, NW=3, N=4, NE=5, E=6, SE=7; сообщает сервер, в БД не сохраняется.

Offer приходит только у разрешённой границы из фактической безопасной позиции. Пока `ready=false`, соседняя сцена готовится; TTL 30s. До confirm сектор не меняется. Cancel/новая цель/ошибка сохраняют исходную локацию. Completion содержит новый Welcome с повышенным epoch. Повтор той же операции на текущей связи не создаёт вторую запись перехода. После reconnect восстанавливается сохранённая текущая локация; старые команды автоматически не переигрываются.

Симуляция 20Hz, membership/delta около 10Hz; 10 входящих команд/с с burst 20. До 256 связей суммарно, до четырёх preparing/closing связей аккаунта, один активный controller. Непрочитанные полные кадры coalesce в ограниченных mailboxes. У writer общий deadline 3s на batch; медленный клиент закрывается. Ping 15s/Pong 5s. Клиентский reconnect ограничен восемью попытками/проверкой окна 60s; `session_replaced` требует ручного действия и не запускает войну вкладок.

Ticket не передаётся в URL/localStorage. Клиент не присылает SQL, баланс, прогресс сохранения, физическую скорость или произвольный сектор. Checkpoints остаются ответственностью planet. Подробные правила переходов — [P2-7](../P2-7/PROTOCOL.ru.md), базового stream — [P2-6](../P2-6/PROTOCOL.ru.md); настоящий документ учитывает добавленные графические поля/метрики.
