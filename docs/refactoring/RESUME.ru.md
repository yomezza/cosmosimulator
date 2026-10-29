# Продолжение рефакторинга после паузы

> **Архив состояния паузы.** Работа возобновлена и завершена в 0.13.0. Текущие изменения, проверки и оставшиеся границы описывает [итоговый отчёт](ui/IMPLEMENTATION-0.13.0.ru.md), механизмы — [действующий каталог](ui/CATALOG.ru.md). Дальнейший текст сохранён как историческая запись WIP и не является текущим статусом выпуска.

**Статус: незавершённая рабочая версия (WIP), не выпускать и не развёртывать.**

Работа остановлена по прямой просьбе пользователя 29 октября 2026: сохранить изменения и сделать коммит, продолжить после обновления лимита. Пауза заменяет предыдущее поручение выполнить весь план без промежуточных остановок.

Ветка: `wip/ui-refactor-after-m0-2026-10-29`. Основание: `225f5c5087aa93ae519a7cd7dfe05e3380994372` (завершённый M0). Стабильная игровая версия остаётся **0.12.5**; `main` не обновляется этим черновиком. Windows EXE, release tag и новый релиз не создавались.

## Задача после возобновления

Закончить оставшуюся работу по [плану M1–M14](ui/MIGRATION.ru.md), не ограничиваясь тестовой инфраструктурой. Последняя команда перед паузой разрешала весь план без остановок между этапами. Сохранить игровой стиль, функции, совместимость HTTP API, сохранённые предпочтения и серверную авторитетность. Затем проверить, собрать новую версионную Windows-сборку, сохранить старые EXE и синхронизировать законченный выпуск с GitHub. Номер следующего выпуска ещё не установлен.

Учитывать будущую вторую Go-службу. Космический сервер владеет общей географией планеты, секторами, территориями и колонизационными проектами; будущий планетарный сервер — внутренней симуляцией секторов. Общая PostgreSQL и согласованные ID не разрешают двум процессам конкурирующие изменения одних данных. [Архитектурный контракт](service-boundaries.ru.md) сохраняется. Полноценный второй сервер в этой очереди не создаётся.

## Что уже изменено в рабочем коде

1. **ES modules.** `index.html` загружает `bootstrap.js` как `type="module"`. Bootstrap явно вызывает init-функции существующих частей. Старый `i18n.js` перемещён в `core/i18n.js`. Планетарный код загружается динамическим import в `planet-loader.js`.
2. **Core.** Добавлены `core/session.js` (Session, RequestScope), `lifetime.js`, `api-client.js`, `api.js`, `snapshot-store.js`, `world-store.js`, `preferences.js`, `format.js`, `scoped-view.js`. Транспорт нормализует ошибки, сохраняет HTTP status при не-JSON ответе и не подменяет названия станций. Snapshot по-прежнему принимает семь endpoint-ответов совместно. Дедупликация явно предусмотрена только для galaxy metadata, не для всех GET.
3. **Сессия.** Рабочие модули используют импортированный Session вместо неявных script globals. Users, gameplay, benchmark и corporations получили scopes; проверка выполняется до изменения DOM. `scopedView` перехватывает ошибки прямых событий/таймеров и не показывает устаревшую ошибку другой сессии. Полная проверка гонок ещё впереди.
4. **Окна.** Добавлены `Modules.destroy`, Lifetime у экземпляра, освобождение resize listener/observer/timer; поддержка transient-окон. Повторное create обновляет заголовок. Ресурсные/объектные/пользовательские окна, confirm и amount начали использовать transient policy. Сохранение обычных окон, pin и геометрии осталось.
5. **Общие элементы.** Созданы `ui/dom.js`, `floating.js`, `tooltip.js`, `feedback.js`, `quantity.js`, `view-state.js`. Простой и ресурсный tooltip переведены на один контроллер. В меню/picker начат общий placement и отслеживание viewport. Escape в header dropdown добавлен, сохранение admin menu после действия не убрано.
6. **Кнопки.** UI.button разделяет желаемое disabled и pending, не затирает disabled, установленное callback. Добавлен aria-busy; callback picker теперь ожидает async onChange.
7. **Станции и числа.** `personalizeStations` создаёт presentation projection вместо изменения raw DTO; локальная карта присваивает результат. Общие formatters подключены к части чисел/дат; исправлен поиск выдачи ресурсов `search` → `q`.
8. **Извлечение features.** Большая часть функций и приватного состояния из `expedition.js` перенесена в factory-контроллеры: `features/fleet`, `mining`, `markets`, `corporations`, `profile`, `administration/users`, `administration/gameplay`, `administration/benchmark`, `notifications`, `guide`, `automation`, `finance`, `portal`. Остался orchestration-файл около 436 строк. Добавлены selectors, inventory/resource-label, corporations/permissions. Это первый перенос, не завершённая архитектура.
9. **Тесты.** Harness обслуживает подкаталоги ES-модулей и ставит inspection bridge только в тестовом браузере, включая reload. Пять прежних TODO перенесены в обычные проверки `regressions.test.mjs`; старый known-defects файл удалён. Script `test:known` заменён на `test:regressions`.

## Последние результаты

- После первых изменений foundation и до выделения features: **16/16 baseline прошли**.
- После выделения features и перевода TODO в обычные проверки: **21 тест, 20 PASS, 1 FAIL, 0 TODO**.
- Прошли все 16 baseline, поздний Users response после logout, повторный confirm title, disabled после callback и HTTP status при HTML 502.
- Не проходит `M4: an open picker must stay inside a resized viewport`, `tests/ui/regressions.test.mjs:32`. Итоговые координаты после уменьшения viewport нарушают проверку `x + width <= 600`. Нужно различить ошибку placement и момент доставки resize; не ослаблять проверку ради зелёного результата.
- Синтаксис созданных JS проверялся Acorn при переносе. Последний браузерный прогон завершён, фонового refactoring test runner не оставлено.
- Новые Go-проверки, полный DB integration, Windows build, visual comparison и CI для этого черновика ещё **не запускались**. Результаты M0 нельзя выдавать за проверку нынешнего рефакторинга.
- [Сохранённый вывод последнего браузерного прогона](checks/refactor-browser-at-pause.txt).

## С чего продолжить

1. Изучить diff от `225f5c5` и этот файл. Не запускать заново временные генераторы переноса: они были одноразовыми и могут повредить уже изменённый код. Оригинал доступен через Git.
2. Разобраться с resize picker и завершить регрессии M1–M5: A/B search в обратном порядке, close/reopen, logout/login другого игрока, смена роли, stale errors, repeat action, dispose/listener counts, вложенные overlays и native modal host.
3. **Проверить восстановление picker в `expedition.js/renderVisible`.** Сейчас добавлен `setValue(value, {emit:true})` для синхронизации с closure. Это промежуточное решение: нельзя допустить, чтобы poll/reopen сам отправил игровую команду (например, выбор корабля). Нужны feature-owned selection/draft state и восстановление без business side effects.
4. Доделать presentation для личных названий станций во всех потребителях и доказать, что raw snapshot не меняется; перенос `personalizeStations` частичный.
5. Завершить PreferenceStore. В `galaxy.js` ещё есть прямое localStorage; у windows reset остаётся removeItem. Проверить legacy plain-string density, corrupted/denied storage, область аккаунта и стабильные defaults. Не менять согласованные defaults произвольно.
6. Завершить callbacks/формы, quantity validity, Tabs keyboard/ARIA, I18n parameters и accessibility labels; сейчас не все эти пункты перенесены.
7. Разделить **вкладки рынка**: пока `renderMarket` по-прежнему большой внутри `features/markets/index.js`. Выделить local/global/orders/data/delivery subviews и state; не объявлять M11 выполненным только из-за переноса файла.
8. Разделить `operations070.js` (Scheduler/AdminResources) и остальные крупные самостоятельные интерфейсы по реальным границам. Очистить чрезмерные/неиспользуемые common imports в сгенерированных factory-модулях. `ui/runtime.js` и `features/runtime.js` — промежуточные регистры; общие UI ещё содержат доменные adapters. Уменьшить связность, оставить одну реализацию каждого механизма.
9. M8/CSS ещё не выполнялся: semantic tokens, component buckets и удаление заменённых конфликтующих правил, без изменения текущего terminal style и без нового слоя overrides.
10. M9 ещё не закончен: Pager/Table/ResourceTile/Selection, сохранение source constraints, physical transfer и existing API. Карточки cargo ещё повторяются.
11. M13 ещё не закончен: visibility-aware scene lifecycle, индексы, ownership private buffers, bounded listeners, существующие 10k label limits и planetary geography/texture contracts. Не менять каноническую географию или cadence вслепую.
12. Go production-код пока **не менялся**. Общий PostgreSQL-пакет ещё не выделен. При выделении отделить конфигурацию/pool/search_path от migration/seed/simulation; сохранить lock order и DB_SCHEMA. Нельзя повторно использовать нынешний `server.openDatabase` вторым сервером как обычный connection helper.
13. После завершения — полные проверки и честный отчёт M1–M14, обновить актуальный каталог компонентов/правила/CI, удалить только заменённые adapters и доказанно ненужные стили. Исходные реестры M0 с pinned commit остаются историческим срезом.

## Проверки и среда

```sh
CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium CHROMIUM_NO_SANDBOX=1 npm --cache /workspace/.cache/npm --prefix tests/ui test
```

При обычной локальной работе `CHROMIUM_NO_SANDBOX` не требуется. Инструкция чистой установки — в `tests/ui/README.ru.md`; её текст о TODO/именах файлов ещё нужно обновить по завершении текущего переноса.

Go в этой среде: `/workspace/toolchains/go/bin`; GOPATH `/workspace/.cache/gopath`, GOMODCACHE `/workspace/.cache/go-mod`, GOCACHE `/workspace/.cache/go-build`. Docker доступен; существующий контейнер `cosmosimulator-dev-db` использует PostgreSQL 17 и порт 5432. В этой очереди рабочие БД/схемы не менялись. Интеграционные тесты запускать только в специально созданной одноразовой БД с суффиксом `_test`, не на существующей игре.

Временные инструменты Acorn/acorn-walk/Prettier ставились в `/tmp/cosmos-refactor-tools`, не в зависимости проекта. Для продолжения исходники и Git достаточны. Перед публикацией проверить embed.FS для новых подкаталогов и lazy imports; никакого обязательного Node runtime для Windows-службы.
