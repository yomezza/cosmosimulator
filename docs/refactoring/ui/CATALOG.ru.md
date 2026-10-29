# Действующий каталог интерфейса · 0.13.0

Корень всех приведённых JS-путей — `internal/server/web`. Это каталог работающего кода. [Реестры M0](README.ru.md#исходный-аудит-m0) остаются историческим срезом 0.12.5, а не актуальным графом импортов.

## Слои и зависимости

| Слой | Ответственность | Точки входа |
|---|---|---|
| Bootstrap | Однократная сборка приложения, явный порядок init | `bootstrap.js`, единственный `type="module"` в `index.html` |
| Core | Сессия, HTTP, принятые данные, настройки, форматирование, ресурсы | `core/session.js`, `api-client.js`, `snapshot-store.js`, `world-store.js`, `preferences.js`, `format.js`, `asset-cache.js` |
| UI | Общие элементы и немодальное рабочее пространство | `ui/index.js`, `ui/runtime.js` и компоненты ниже |
| Features | Игровые команды, права, состояние экранов и представление DTO | `features/*`, сборка контроллеров в `features/workspace/index.js` |
| Styles | Текущий terminal style, семантические переменные | `styles/tokens.css`, `styles/terminal.css` |

`core` не импортирует `ui` или `features`; `ui` не импортирует `features`. Features используют core/UI и явные игровые adapters. Архитектурный тест проверяет эти направления, существование импортов и отсутствие неявных JS globals. Общего production-объекта `window.UI`, `window.Game` или `window.gameData` больше нет.

`ui/runtime.js` — библиотека компонентов и API окон, зарегистрированные один раз при bootstrap. `features/runtime.js` — небольшие порты взаимодействия features. Они сохраняются как действующие точки связывания, а не как второй комплект старых реализаций. Проверочный bridge в браузерном harness существует только в тестах.

## Общие механизмы

| Задача | Единственный механизм | Что остаётся у потребителя |
|---|---|---|
| Актуальность async-ответа | `core/session.js`: `RequestScope`; `core/scoped-view.js` | Активность окна, роль, параметры поиска; проверка lease **до** изменения DOM |
| HTTP | `core/api-client.js`, facade `core/api.js` | Endpoint, payload, идемпотентность бизнес-команды; автоматического retry мутаций нет |
| Принятый snapshot | `core/snapshot-store.js` | Семь ответов принимаются совместно; при частичном отказе предыдущий snapshot остаётся |
| Метаданные мира | `core/world-store.js` | Индекс звёзд, session/revision guard; dedup явно включён только для `galaxy` |
| Ресурсы экземпляра | `core/lifetime.js` | Dispose вызывается владельцем; process-wide обработчики не создаются на каждый render |
| Публичная графика | `core/asset-cache.js` | SVG-кеш до 64 шаблонов, PNG-кеш до 6 текстур; private DTO туда не попадают |
| Настройки | `core/preferences.js` | Прежние ключи, defaults и scope; ручной выбор не заменяется default при poll |
| Форматирование | `core/format.js`, `core/i18n.js`, `ru.json`, `en.json` | Выбор величины/точности; raw имена и заметки не переводятся, DTO не округляется |
| DOM | `ui/dom.js`: `UI.node` | Только текст пользовательских данных; SVG namespace отдельно |
| Окно | `ui/windows.js`: `Modules.create/open/close/destroy/title` | Workspace либо transient, audience, dirty/pin; связанные ship/cargo расположены в `features/fleet/layout.js` |
| Кнопка / действие | `ui/actions.js`: `UI.button`, `bindAction` | Контролируемое disabled и причина; pending не отменяет новую валидацию |
| Поля / dirty | `ui/fields.js`, `ui/form-state.js` | Проверки игры, save/cancel и область draft; восстановление поля не вызывает команду |
| Popup | `ui/floating.js`, `ui/menu.js`, `ui/combobox.js` | Разные interaction policies меню и выбора; общий clamp/host, native modal top layer |
| Подсказка | `ui/tooltip.js` | Plain/rich provider; доменные карточки подключаются features |
| Подтверждение / количество | `ui/confirm.js`, `ui/quantity.js` | Текст, maximum и бизнес-команда; серверная валидация обязательна |
| Краткое сообщение | `ui/feedback.js`: `notify(text, level, persist=false)` | Технический toast не создаёт inbox-запись |
| Ветки | `ui/disclosure.js`: `UI.fold`, `UI.rememberFolds` | Ключ, default и opt-in persistence; не единый default для всей игры |
| Вкладки / таблица | `ui/tabs.js`, `ui/table.js` | Данные и права; associated panel для рынка, клавиатурная навигация вкладок |
| Страницы | `ui/pager.js`: `pager`, `cursorMore` | Нумерованные страницы и cursor остаются разными протоколами |
| Голограмма | `ui/holograms.js` | Модель и состояние движения; уникальные SVG IDs, общий 16-секундный clock, reduced motion |
| Плитка ресурса | `features/inventory/resource-tile.js` | Source, selection, drag, quantity и право перемещения задаёт cargo/logistics |

## Игровые разделы

| Раздел | Код / владелец состояния |
|---|---|
| Shell, вход, навигация, сервер | `features/shell/{app,navigation,icons,server-status}.js` |
| Корабль, трюм, верфи | `features/fleet/index.js`, `presentation.js`, `activity.js`, `layout.js` |
| Добыча | `features/mining/index.js` |
| Рынок | `features/markets/index.js`: tab/state/scope; `local.js`, `global.js`, `orders.js`, `data.js`, `delivery.js`, `trade-form.js` |
| Логистика | `features/inventory/logistics.js`, общие `resource-label.js`, `resource-tile.js` |
| Корпорации | `features/corporations/index.js`, `flag.js`, `permissions.js` |
| Планировщик | `features/automation/index.js`, `scheduler.js` |
| Исследования | `features/research/index.js` |
| Личные станции, выбор станции/системы | `features/stations/index.js`, `features/navigation/pickers.js` |
| Профиль, уведомления, руководство, финансы | `features/profile`, `notifications`, `guide`, `finance` |
| Идеи | `features/feedback/ideas.js`, включая административное представление |
| Пользователи, настройки, бенчмарк, ресурсы, галактики | `features/administration/{users,gameplay,benchmark,resources,galaxies}/index.js` |
| Объектные карточки и модели | `features/objects/{cards,presentation,visuals}.js` |
| Галактика и звёздная система | `features/maps/galaxy.js`, со своими камерами/фильтрами/индексами |
| Поверхность | `features/planet/loader.js` → lazy `index.js`, `terrain.js` |
| Межгалактический портал | `features/portal/index.js` |

Имена станций применяются в presentation projection из **новых** personal notes; raw snapshot и общий транспорт не переписываются. Прямой обмен грузом и очередь перевозки используют существующие разные команды. Наличие общего `resourceTile` не делает перенос на удалённую станцию мгновенным.

## Сохранённые политики

- Ветка «Карты» закрыта до ручного открытия; ручной выбор сохраняется.
- В планировщике группы кораблей закрыты по умолчанию, задания и счётчик принадлежат конкретному кораблю.
- Чужие корабли по умолчанию выключены в галактике и включены внутри системы; настройки режимов и аккаунтов независимы.
- Категории руководства закрыты изначально, поиск раскрывает совпавшие категории.
- Геометрия, pin и drafts обычных рабочих окон переживают их закрытие. Transient-окна уничтожаются при обычном закрытии, если не dirty/pinned; logout очищает их независимо от этого.
- Язык до входа остаётся browser-scoped; прежние plain-string настройки читаются. Изменение scope всех старых настроек не выполнялось.
- Закрытие карты останавливает её draw loop, а не серверный полёт или добычу. Глобальная география планеты и версии секторов остаются серверными.

Подробный исходный реестр ключей/defaults: [preferences.md](preferences.md). Результаты и границы проверки: [отчёт 0.13.0](IMPLEMENTATION-0.13.0.ru.md).
