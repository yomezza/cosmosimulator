# Реализованный WS v1 · P2-6

Same-origin `/planet/ws/v1` через cosmic gateway. Cookie `session` проверяется до Upgrade, browser Origin должен точно совпасть с allowlist. Hello в течение 5s, входящее сообщение ≤4096 bytes. Ticket никогда не передаётся в URL. Одна управляющая связь на аккаунт; до четырёх preparing/closing связей и 256 соединений всего. Администратор не обходит доступ к колонии.

Клиент: `{v:1,type,seq,visit,connection_epoch,payload}`. `hello` имеет seq=1 без visit/epoch, затем seq строго возрастает в рамках controller. Дубликат игнорируется. Другой visit/epoch закрывает связь. Новое подключение начинает свой счётчик; прежние команды не переигрываются.

Сервер: `{v:1,type,server_seq,visit,connection_epoch,payload}`. `server_seq` монотонен для всех пакетов конкретной связи. На ошибке до активации visit/epoch могут отсутствовать. DB ID surface — строка, public actor/visit — UUID, epoch/revision/seq — целые счётчики. Никаких credentials, внутренних account IDs, seed, финансов и маршрутов чужого игрока.

## Команды

| type | payload | Поведение |
|---|---|---|
| hello | `{ticket:opaque64}` либо `{resume_visit:uuid}` | Авторизованный вход или замена controller существующего визита, новый epoch |
| move | `{x_q:int,y_q:int}` | Только цель клетки в текущей сцене, Q=1024; скорость и маршрут вычисляет сервер |
| resync | `{last_server_seq:nonnegative-int}` | Свежий динамический snapshot; не чаще 1/s, без replay истории и без выбора чужого сектора |
| leave | `{operation:uuid}` | Сохранение безопасной позиции + завершение визита в одной tx; затем cosmic снимает reservation |

Общий входящий token bucket 10/s, burst 20. Неизвестные поля move/resync отклоняются. Transition-команды появятся в P2-7, сейчас `unsupported_command`.

## Ответы

| type | payload |
|---|---|
| welcome | `visit, connection_epoch, actor, nickname, planet, surface, sector, x_q, y_q, speed, server_time` |
| sector.snapshot | `surface, sector, revision, server_time, actors:[PublicActor], self:State` |
| actor.enter | один `PublicActor` |
| actor.leave | `{actor,reason:"disconnected"}`; общий безопасный reason, без причины выхода из аккаунта |
| path.accepted | собственный `State` с `path:[{x,y}]` в мировых единицах, command_seq/path_generation |
| state.delta | собственный `State` без path + `actors:[изменённые PublicActor], surface, sector, revision` |
| error | `{code,seq?}`; стабильный ключ локализации, безопасный текст без SQL |
| visit.ended | `{operation,reason:"return"}` |

`PublicActor`: `actor` постоянный UUID персонажа, `nickname` игровое имя на момент входа, `sprite:"colonist"`, `x_q,y_q`, `moving,suspended`, `server_time` Unix ms. Имя рисуется текстом Canvas, не HTML. Данные соседних секторов не включаются.

`State`: `x_q,y_q,moving,suspended,remaining,path_generation,command_seq,server_time,path?,error?`. `remaining` позволяет убрать пройденные сегменты своей линии. HTTP `/planet/api/v1/scene?visit=...&epoch=...` отдельно отдаёт геометрию только после повторной проверки текущего допуска, без secret/seed; WS snapshot не дублирует PNG/маску всей сцены.

## Очереди, время и восстановление

Симуляция 20Hz, рассылка около 10Hz. На sector actor собирается одна неизменяемая membership-выборка. У каждого подписчика один заменяемый sector frame и один собственный state frame. Enter/leave вычисляются относительно последнего **успешно доставленного** состава, поэтому пропуск промежуточного frame не оставляет вечного персонажа-призрака. Snapshot задаёт новую базу; frame с прежним sector sequence не применяется.

Один WS writer, общий deadline 3s для полного event+delta batch, без очереди неограниченного размера. Медленный клиент закрывается. Сервер Ping каждые 15s, Pong должен прийти за 5s; отдельный reader продолжает обслуживать control frames. Браузер также считает отсутствие любых кадров >6s потерей связи. При недоступности auth/lease actor сам останавливается по разрешению движения сроком 2s.

Reconnect: exponential 0.5s → 8s с jitter ±20%, до восьми попыток и проверкой общего окна 60s перед следующей попыткой. Стабильное соединение 10s сбрасывает счётчик. Handshake timeout 5s, HTTP status lookup 8s. После предела — ручной повтор/возврат. `session_replaced`, auth/access/visit/version отказ прекращают автоперехват; 401/403/400/409 HTTP также не зацикливаются. Закрытие окна/logout отменяет таймеры, запросы, renderer и WS.

Позиции интерполируются с задержкой около 100ms, без экстраполяции и клиентского ускорения. Скрытая вкладка не рисует кадры, но сервер остаётся владельцем симуляции. Checkpoints пишутся только при dirty-позиции с периодом 5.0–5.5s (небольшое разнесение по аккаунтам), при takeover/disconnect/return/штатной остановке. Fencing/epoch/version CAS обязательны в прежнем порядке блокировок. Нет SQL на frame и нет клиентской команды «сохранить координаты».
