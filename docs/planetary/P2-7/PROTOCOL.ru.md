# WS v1 · расширение P2-7

Базовые правила [P2-6](../P2-6/PROTOCOL.ru.md) сохраняются: cookie/Origin, один controller, ограниченные очереди, server-owned движение и свежие DB-проверки.

## Переход

| Направление/type | payload |
|---|---|
| server `transition.offer` | `{offer:uuid,neighbor:int,owner:string,expires_at:ISO8601,ready:bool}` |
| client `transition.confirm` | `{offer:uuid,operation:uuid}` |
| client `transition.cancel` | `{offer:uuid}` |
| server `transition.cancelled` | `{reason:string}` |
| server `transition.complete` | `{operation:uuid,welcome:Welcome}` |

Offer создаёт сервер по реальной остановившейся позиции в пределах 3 единиц от ingress. Произвольный destination/координаты/права в команде не принимаются. TTL 30s, одно предложение на связь. Пока `ready=false`, соседняя сцена загружается асинхронно; управление исходным actor приостановлено, auth/checkpoint/WS продолжают обслуживаться. Тот же offer повторно приходит с `ready=true`. Cancel, движение, истечение TTL, close и ошибка освобождают destination pin. При нехватке capacity/ошибке загрузки — `transition_load_failed`, исходная location сохраняется.

Confirm требует актуальных visit/epoch, offer и UUID операции, повторно проверяет доступ, canonical revision, обе сцены, reciprocal portal и lease/fence. `transition.complete` содержит **новый** connection epoch и исходный постоянный actor/visit. Клиент применяет completion раньше обычного сравнения epoch, очищает динамический snapshot и загружает scene по новому visit/epoch; следующий `sector.snapshot` описывает новый сектор.

В пределах одного WS client seq продолжает возрастать после смены epoch. Команды, уже отправленные со старым epoch непосредственно перед переходом, не перемещают игрока в новой сцене. Повтор последнего confirm с тем же offer/operation возвращает прежний completion после свежей проверки авторизации, без новой tx перехода. Клиент игнорирует повторно применённый operation. PostgreSQL PRIMARY KEY(user,operation) и UNIQUE(user,offer) не позволяют переиспользовать подтверждённую операцию после reconnect; старый RAM offer после reconnect не действителен. Восстановление читает актуальную location из БД, не переигрывает старую очередь confirm.

`transition.cancelled` причины: `cancelled`, `move`, `offer_expired`, `transition_load_failed`, либо безопасный ключ ошибки допуска/службы. Некорректная команда даёт `offer_expired` или обычную протокольную ошибку. При неопределённом результате commit из-за разрыва связи безопасный путь — resume/read model: подтверждённый результат остаётся в БД; координаты браузера не принимаются.

## Публичная сцена

Авторизованный HTTP scene DTO дополнен `exits:[{neighbor,owner,x_q,y_q}]`. Это только разрешённые завершённые соседи с каноническим portal. Данные о чужих закрытых секторах не добавляются. Runtime не создаёт новые физические проходы. `landing` равен null, если в текущем секторе нет реального enabled модуля: зарезервированное генератором место не изображается как существующая площадка.

## Транзакция и runtime

Destination заранее загружается и удерживается Registry pin, без world/SQL lock на время генерации. Commit использует прежний порядок world shared → lease row → user lock и общий per-user gate. Проверки обеих географий выполняются в caller tx, без дополнительного pool acquisition.

В одной tx сохраняются departure coordinates в receipt, arrival coordinates/current sector в character, target colony/current sector/new epoch в visit и новая position_version. Затем Hub.Join под одним mutex удаляет старый controller и включает нового в destination actor с прежним public ID. Другие клиенты получают leave исходного сектора / enter нового. Старая отложенная запись не проходит epoch/version CAS. Если процесс остановлен после DB commit до отправки completion, resume восстанавливает новую location. Если commit не состоялся, исходная location и actor сохраняются.

Исходная посадка/ship reservation и текущая доступная colony — отдельные проверки. Разделение используется в Resolve/Activate, Check, Sweep и Checkpoint; отсутствие второго модуля больше не ломает resume. Потеря прав на текущую или исходную территорию завершает/блокирует визит по прежним правилам. Право входа администратора не повышается автоматически.
