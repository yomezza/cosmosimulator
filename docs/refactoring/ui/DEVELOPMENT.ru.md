# Предлагаемые правила разработки интерфейса Cosmosimulator

Это проект правил по результатам аудита 0.12.5, а не внесённые изменения или новый обязательный framework. После согласования guide и каталог можно разместить рядом с исходниками UI и указать в AGENTS/README. Сейчас production-файлы не менялись.

## Перед новой задачей

1. Найти тип задачи в [components.md](components.md), текущее поведение в [preferences.md](preferences.md) и реальную функцию в source-references.md. Быстрый поиск — `rg` по имени существующего UI/Modules helper и его callers.
2. Записать, кто владеет данными: сервер, feature snapshot, transient form, manual preference или public asset cache.
3. Проверить existing behavior: default, reopen, reload, language, logout/login и empty/loading/error. Не менять их попутно.
4. Выбрать reuse / extend / feature-specific реализацию. Одинаковая верстка ещё не означает одинаковую игровую механику.
5. Зафиксировать один значимый сценарий до изменения. Проверки выбирать пропорционально риску, не писать implementation-mirroring тест на каждую мелкую правку текста.

## Где уже есть общие решения

| Задача | Текущий инструмент | Ограничение / правильная граница |
|---|---|---|
| Небольшой DOM-элемент | UI.node | textContent, без HTML из пользовательских данных. SVG требует namespace helper. |
| Рабочее окно | Modules.create/open/close/title | close не destroy; повтор create не меняет title/size. У transient windows есть выявленные ограничения P2/P3. |
| Кнопка | UI.button | Busy/error wrapper есть; disabled сейчас возвращается к первоначальному аргументу. Не строить конкурирующую третью кнопку. |
| Поле / информационная строка | UI.field / UI.row | Game validity/payload — в feature, а не в helper. |
| Выбор | UI.picker / UI.choice | Не делать новый searchable dropdown; station/system options — feature adapters. Async callbacks требуют явного promise/error contract. |
| Таблица / вкладки | UI.table / UI.tabs | Data source и права не внутри generic widget. Нужна дальнейшая keyboard/ARIA спецификация. |
| Ветка | UI.fold / UI.rememberFolds | Persistence opt-in; key stable, defaults feature-owned. Не менять все defaults на один. |
| Подтверждение | UI.confirm / специализированный existing reset flow | Не копировать позиционирование и lifecycle заново. Preview, obligations, forfeit и idempotency — domain policy. |
| Числа | UI.formatNumber / UI.percent | Точность выбирается по величине; percent default нельзя применять к малой светимости. |
| Всплывающая помощь | data-help / existing rich resource card | Не создавать ещё один независимый tooltip host/placement для тех же действий. |
| Краткий feedback | notify(text,level,persist=false) | Technical action toast не должен автоматически попадать в inbox. |
| Голограмма | UI.hologram / UI.updateHologram | Не дублировать SVG cloning/IDs/clock; менять domain model отдельно. |
| Личные названия станций | UI.stationLabel / stationRef / stationEditor | Не подменять все строки вручную в новом модуле. Это feature presentation. |
| Перенос ресурсов | Logistics.draggable/dropzone/chooseDestination | Сохранять source constraints, quantity, direct/queue, receipt. Не новый arbitrary transfer API. |

У этих helpers есть смешанные обязанности и известные дефекты. Catalog не объявляет всё идеальным: сначала использовать/улучшать единственную реализацию после разрешения, не наращивать параллельные копии.

## Reuse, extend или новый компонент

**Reuse:** поведение, lifecycle и ownership совпадают, различается только содержимое. Пример — общая таблица для разных столбцов.

**Extend:** нужен независимый небольшой variant с ясным контрактом. Пример — passive tooltip и content slot, отдельная политика dismiss, optional persistent Disclosure. Variant не должен требовать knowledge/ship/corporation внутри core.

**Новый feature-specific компонент:** различаются authority, координаты, gameplay semantics или data volume. Пример — planet material renderer, route planner, corporate role editor, transfer form. Он использует общую инфраструктуру, но не становится универсальным widget для всех сценариев.

**Не создавать абстракцию:** общий код — несколько простых span/hr/div и дальнейшего reuse нет. Native HTML и общий CSS class часто лучше слоя классов с конфигурацией.

## Контракты компонентов

- Общий UI принимает data/slots/labels/callbacks; ничего не знает об endpoint именах, правах, кошельке и seed.
- Feature получает authoritative DTO и capabilities, собирает view model и выполняет действие через свой API adapter. Сервер повторно проверяет права и инварианты.
- У stateful controller явные update и dispose; у простого formatter/node factory они не нужны.
- Владелец запроса проверяет session/generation/instance **перед** DOM update. Abort — оптимизация; stale acceptance guard остаётся.
- Action callback возвращает promise до окончания действия. Не запускать async build/render внутри callback без return/await/catch, если error boundary должен его увидеть.
- Не автоматизировать повтор финансовой/разрушительной мутации. Повтор использует тот же request ID только по существующему domain idempotency contract.
- Один владелец busy/validity. Не смешивать несколько finally, которые независимо включают кнопку.
- Runtime-render не должен править исходный server DTO или переводить имена игроков. Не deep-clone весь мир на каждую hover-операцию ради формальной immutable архитектуры.

## Настройки и идентичность

- Default — правило feature; manual preference — выбор игрока; draft/selection/pending — transient; balance/inventory/ownership — server data.
- Stable key: feature + component role + entity/mode + schema version. Не summary text, translated label или номер строки в изменяемом списке.
- Browser scope допустим для языка до входа. Account scope нужен персональным предпочтениям. Instance scope отличает galaxy/system, корабли и задачи.
- Restoration не вызывает business action и не записывает новый manual preference само по себе. Не определять origin только наличием native toggle event.
- Повреждённое/недоступное storage даёт default и рабочий UI. Не требовать PostgreSQL для каждого UI toggle.
- Миграция сохраняет старый выбор. Reset допустим только по явному согласованному правилу, как v2 planner в 0.12.3, а не автоматически при любом рефакторинге.

## События, окна и overlays

- App-lifetime и instance-lifetime listeners различаются. Dynamic controller хранит disposer/AbortController для событий, timers, observers и requests.
- Hide, pause и destroy — разные операции. Close окна не отменяет серверную добычу или очередь.
- Новые events именовать по feature/действию, payload документировать через typed refs, например star/planet/surface/ship. Старые events не переименовывать одновременно без facade/migration.
- Не читать чужой DOM для получения игрового выбора. Передавать явную navigation command или обращаться к feature selector.
- Workspace остаётся немодальным там, где пользователь работает с несколькими окнами. Confirmation/editor policy выбирается явно, а не потому, что все сейчас построены через dialog.
- Outside click, Escape, focus return и placement — инфраструктура OverlayHost. Для header admin menu action-dismiss=false может быть правильным; Escape — отдельная policy.
- Проверять top-layer/native modal и portal descendants. Не лечить stack конфликт очередным огромным z-index.

## Оформление и тексты

- Сначала reuse token/variant существующего component bucket. Новую literal метрику обосновать; не добавлять поздний CSS override, если можно изменить единственный согласованный source.
- В будущем использовать предсказуемые UI/feature prefixes, например ui-button и fleet-card. Идентификаторы — для экземпляра/доступности, не глобальная styling стратегия.
- Не вводить ещё один глобальный `.selected`, влияющий на чужие features. State передавать class/data attribute в пределах component.
- Тонкие прямоугольные controls, квадратные system ID, одобренные menu/tooltip/notification варианты сохраняются. Унификация не означает одинаковые corners всех объектов.
- Новая статическая строка проходит через действующий I18n. Динамические сообщения строятся из key/parameters; пользовательские имена/заметки — raw text.
- Форматирование явно выбирает precision, unit, locale, unknown policy. Не менять исходные числа или server payload.
- Не вводить второй каталог переводов, не переписывать все русские ключи на английские IDs только ради новой структуры папок.
- Нужны label/aria-name, корректная клавиатура и focus states. Одного role=combobox/tab недостаточно для полного interaction contract.

## Карты, assets и будущий 2D-режим

- Общими делать только действительно совпадающие camera/math/listener/asset conventions. Граф галактики и шестиугольная поверхность — разные renderers.
- Каноническая geography и ownership принадлежат космическому серверу. Texture pixels не задают игровую воду, запас ресурса или возможность колонизации.
- Публичные PNG/SVG можно кешировать независимо от аккаунта. Private snapshots, labels/notes, выбранный корабль и открытая карта имеют session boundaries.
- Загружать тяжёлые материалы/локальные sprites по входу в соответствующий режим. Не увеличивать частоту кадров без trace и не делать terrain raster заново на каждый pointermove.
- Future local-sector scene имеет собственный render/input scope и согласованную server geography; это не причина встроить всю 2D-игру в generic UI components.

## Перед завершением согласованного этапа

Проверить изменённый контракт и затронутые реальные flows; для session/forms/overlays обязательно late response, repeat action, close/reopen, RU/EN и нужные viewport/input modes. Обновить component/preference registry и кратко описать изменения, проверки и ограничения. Не переходить к следующему этапу без разрешения пользователя.
