# Высадка и персонаж · P2-4 / 0.18.0

Теперь можно войти в планетарный визит и увидеть сохранённую локальную сцену: берег/воду, природный декор, место посадки и персонажа. Это **неподвижный технический обзор**. Движение, камера и поиск пути относятся к P2-5, полноценный multiplayer stream — к P2-6, окончательная графика/анимации — к P2-8.

[Полный отчёт](REPORT.ru.md) · [План](../P2-0/PLAN.ru.md) · [Предыдущий этап](../P2-3/REPORT.ru.md).

## Обновление и включение

1. Остановить обе службы, сохранить согласованную резервную копию игровой схемы.
2. Запустить **cosmosimulator 0.18.0** с прежней БД/схемой. Cosmic runner применяет `schema180.sql`, совместимость повышается до **4**. Создание галактик, экономика и старая общая география сохраняются.
3. Выдать планетарной роли права ниже. Запустить **cosmosimulator-planet 0.18.0** на той же БД/схеме.
4. В окружении **космической** службы задать `PLANET_ENABLED=true`. Необязательный `PLANET_INTERNAL_URL=http://127.0.0.1:8081` указывает внутренний адрес второй службы. Перезапустить cosmic после изменения окружения.
5. На planet задать `PLANET_ALLOWED_ORIGINS` равным **точному адресу браузерной игры**, например `https://game.example.com` или `http://192.168.1.10:8080`. Список через запятую; без путей и `*`. Для cosmic при reverse proxy сохраняется соответствующий `PUBLIC_ORIGIN`.

По умолчанию feature flag выключен. Без второй службы космический терминал продолжает работать. Браузер использует прежний адрес игры: cosmic проксирует `/planet/*`, включая WebSocket. Открывать игрокам порт 8081 не требуется. При внешнем reverse proxy он должен поддерживать WebSocket Upgrade до cosmic. Cookies остаются HttpOnly и передаются на том же origin; билет не попадает в URL или localStorage.

Health planet `/planet/api/v1/health/ready` возвращает `stage=P2-4`, `landing_ready=true`. `gameplay_ready=false` пока означает отсутствие полного движения/симуляции, а не отсутствие высадки.

Windows EXE:

- `downloads/cosmosimulator-v0.18.0-windows-amd64.exe`
- `downloads/cosmosimulator-planet-v0.18.0-windows-amd64.exe`

SCM и базовое окружение описаны в [инструкции службы](../P2-1/README.ru.md). Использовать текущие EXE и перечисленные здесь права. Переменные окна PowerShell не меняют автоматически окружение уже установленной службы. Старый planet несовместим с marker=4; запускать согласованные версии обеих служб. Откат — с резервной копией и соответствующими сборками, при остановленном новом runtime.

## Как посмотреть локацию

Завершить личную колонию либо получить корпоративное `planetary.enter`; дождаться готовности посадочной площадки. Исследовать систему и выполнить манёвр **именно к планете** выбранным свободным кораблём. Просто нахождения в системе недостаточно.

Открыть карту поверхности → сектор с доступной площадкой → **«Высадиться»**. В проекте кнопка **«Выбрать площадку»** открывает карту поверхности. Корабль не перемещается при входе и закрепляется за визитом. Остальными кораблями можно пользоваться.

Закрытие окна/вкладки оставляет сохранённый визит. Для восстановления: **Колонии → «Продолжить высадку»**. После перезагрузки восстанавливается тот же персонаж и положение. Вторая вкладка получает новый connection epoch; старая теряет управление. **«Вернуться в космос»** запрашивает завершение и освобождение корабля. При недоступной службе возврат ждёт истечения/fencing её DB lease; потеря связи сама по себе не объявляется успешным возвратом.

Старые информационные высадки помечены `legacy`: они не создают персонажа автоматически. Через «Продолжить высадку» доступен явный возврат. Для окончательного закрытия legacy-записи нужно запустить новую планетарную службу.

## Права PostgreSQL

Полный набор для P2-4, пример схемы `cosmosimulator` и заранее созданного login `cosmos_planet`. Выполнить оператором после миграции; заменить имена с корректным quoting:

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
GRANT SELECT(id,company_id,company_role) ON cosmosimulator.users TO cosmos_planet;
GRANT SELECT(id,user_id,star,object_id,destination,local_target,waypoints)
 ON cosmosimulator.ships TO cosmos_planet;
GRANT SELECT(id,owner_id) ON cosmosimulator.companies TO cosmos_planet;
GRANT SELECT(id,company_id,permissions) ON cosmosimulator.company_roles TO cosmos_planet;
GRANT SELECT(user_id,star) ON cosmosimulator.surveys TO cosmos_planet;
GRANT UPDATE(owner_id,fence,expires_at)
 ON cosmosimulator.planet_runtime_leases TO cosmos_planet;
GRANT INSERT,DELETE ON cosmosimulator.planet_local_locations,
 cosmosimulator.planet_handoff_uses TO cosmos_planet;
GRANT INSERT,UPDATE ON cosmosimulator.planet_characters,
 cosmosimulator.planet_visits TO cosmos_planet;
```

`planet_session_lookup` — узкое представление с hash сессии, пользователем, сроком и признаком бота. Пароли и кошельки planet читать не должен. Не выдавать изменение ships, пользователей, колоний, площадок, controls, tickets, marker или DDL. При общем административном login разграничение записей обеспечивается кодом, а не правами DB.

## Очистка поверхности

Активные/отключённые/legacy-визиты продолжают блокировать reset. Сначала завершить их. Для сохранённых локальных сцен остаётся двухэтапная подготовка P2-3: cosmic переводит поверхность в retiring, planet очищает свои scene/use/character references, затем требуется свежее подтверждение удаления. Завершённая запись визита не удерживает FK на колонию/корабль/площадку. Cosmic удаляет собственные старые tickets перед глобальными объектами; финансовые данные очищаются прежним контролируемым порядком.
