# Данные и права PostgreSQL · 0.23.0

Обе службы используют одну базу и одинаковый `DB_SCHEMA`. Идентификаторы surface/sector/revision общие. Планетарный процесс открывает пул через `platform/postgres.Open` без DDL/bootstrap. Миграции выполняет существующий космический runner; текущий marker **7**, генератор/формат локальной сцены **v1**. В 0.23.0 новых таблиц и миграций нет.

## Ответственные за изменение

| Данные | Единственный игровой writer |
|---|---|
| Галактики, глобальная поверхность, seed/биомы/реки/сектора | Cosmic |
| Колониальные проекты, территории, посадочные площадки | Cosmic |
| Корабли, кошельки, ресурсы, исследования, экономика, мировой tick | Cosmic |
| `planet_transfer_controls`, tickets в `planet_handoffs` | Cosmic |
| `planet_runtime_leases` | Planet, singleton заранее создаёт cosmic migration |
| `planet_local_locations`, immutable geometry/checksum/decor seed | Planet |
| `planet_characters`, `planet_visits`, `planet_handoff_uses` | Planet |
| `planet_transition_receipts` | Planet |
| `planet_schema_state`, `planet_runtime_controls` | Cosmic |

Позиции движущихся персонажей находятся в RAM planet. PostgreSQL получает dirty checkpoints раз в 5.0–5.5s, при takeover/disconnect/return и штатной остановке. На каждый кадр движения INSERT/UPDATE не выполняется. Авторизация/lease и команды всё же обращаются к БД; измеренный SQL QPS включает эти проверки.

Сохранение требует действующих runtime owner/fence, visit generation, connection epoch и position version. Общий порядок блокировок: shared world barrier `784218`, lease, затем пользовательский advisory `784219`; cosmic reset использует exclusive world barrier. Нельзя держать SQL-транзакцию в ожидании загрузки PNG или подтверждения игроком.

`planet_transition_receipts` защищает подтверждённую операцию от дублирования и сохраняет значения перехода. `planet_session_lookup` — узкое read-only представление с hash сессии, сроком, пользователем, bot flag и nickname; не раскрывает пароль или баланс.

## Полный GRANT для planet

Пример для уже созданного login `cosmos_planet` и схемы `cosmosimulator`. Выполнить оператором **после запуска cosmic migration**. Для другого имени схемы/роли заменить идентификаторы с правильным quoting; пароль задаётся отдельно средствами оператора.

```sql
GRANT USAGE ON SCHEMA cosmosimulator TO cosmos_planet;
GRANT SELECT ON
 cosmosimulator.planet_schema_state, cosmosimulator.planet_runtime_leases,
 cosmosimulator.planet_local_locations, cosmosimulator.planet_surfaces,
 cosmosimulator.planet_surface_controls, cosmosimulator.planet_runtime_controls,
 cosmosimulator.planet_colonies, cosmosimulator.planet_projects,
 cosmosimulator.planet_landing_sites, cosmosimulator.planet_characters,
 cosmosimulator.planet_handoffs, cosmosimulator.planet_handoff_uses,
 cosmosimulator.planet_transfer_controls, cosmosimulator.planet_visits,
 cosmosimulator.planet_session_lookup TO cosmos_planet;
GRANT SELECT(id,company_id,company_role,role)
 ON cosmosimulator.users TO cosmos_planet;
GRANT SELECT(id,user_id,star,object_id,destination,local_target,waypoints)
 ON cosmosimulator.ships TO cosmos_planet;
GRANT SELECT(id,owner_id,name) ON cosmosimulator.companies TO cosmos_planet;
GRANT SELECT(id,company_id,permissions) ON cosmosimulator.company_roles TO cosmos_planet;
GRANT SELECT(user_id,star) ON cosmosimulator.surveys TO cosmos_planet;
GRANT UPDATE(owner_id,fence,expires_at)
 ON cosmosimulator.planet_runtime_leases TO cosmos_planet;
GRANT INSERT,DELETE ON cosmosimulator.planet_local_locations,
 cosmosimulator.planet_handoff_uses TO cosmos_planet;
GRANT INSERT,UPDATE ON cosmosimulator.planet_characters,
 cosmosimulator.planet_visits TO cosmos_planet;
GRANT SELECT,INSERT ON cosmosimulator.planet_transition_receipts TO cosmos_planet;
```

Login также должен иметь CONNECT к существующей базе. Ему не нужны CREATE на схему, изменение marker/колоний/площадок/кораблей, чтение password_hash/credits или запись финансов. Поле `role` добавлено к чтению только для административных метрик. Без него старый набор прав продолжает обслуживать игру, metrics возвращает 503.

Ограниченная роль проверяется автоматическими отрицательными сценариями: DDL, изменение world/кошелька/корабля/controls и чтение password_hash/credits должны отклоняться самой PostgreSQL. Общий административный login не даёт такого DB-разграничения, даже когда код соблюдает границы.

## Старые колонии и удаление

Cosmic backfill повторяемый: соседние проходимые колонии одного владельца образуют территорию; оторванной территории нужна своя площадка. Повтор/конкурентное завершение не создаёт дубли. При объединении существующие площадки сохраняются; новый связующий сектор не создаёт лишнюю. Личные и корпоративные владельцы разделены, вода не соединяет территории.

Immutable сцены не перегенерируются при обновлении приложения. Неизвестная версия или несовпадение source/checksum приводит к отказу; это не повод автоматически заменить локацию. Соседняя география берётся из общего сохранённого snapshot, а не другого независимого генератора planet.

Удаление поверхности идёт через cosmic retiring и planetary drain/purge при отсутствии живых визитов. Все служебные и финансовые данные входят в согласованную резервную копию. Ручное удаление ссылок/изменение marker не заменяет штатный протокол.
