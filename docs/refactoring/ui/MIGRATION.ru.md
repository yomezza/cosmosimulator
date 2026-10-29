# Поэтапная миграция клиентского интерфейса

Это план, **не выполненные изменения**. Каждый этап требует отдельного разрешения пользователя. После каждого этапа — проверка игры, отчёт и остановка; разрешение на один этап не распространяется на следующие. Оценки S/M/L обозначают относительный объём/риск, а не обещание календарных сроков.

Общие условия: старые HTTP-контракты сохраняются; сервер остаётся авторитетным для экономики, прав и симуляции; compatibility facade делегирует **одной** реализации. Нельзя временно держать две расходящиеся реализации одного компонента и переводить новые функции на обе.

## M0. Зафиксировать текущие контракты и регрессионные сценарии

- **Цель:** создать проверяемую отправную точку вместо массовой «чистки».
- **Файлы / модули:** тестовая инфраструктура вне production web; index.html, app.js, windows.js, controls.js и основные feature-сценарии как объекты проверок. Текущие файлы на этом этапе не переписывать.
- **Механизм:** минимальный браузерный runner, статические fixtures для UI, отдельная disposable PostgreSQL для интеграционных сценариев. Каталог актуальных компонентов и policies.
- **Зависимости:** нет; возможные dev-зависимости согласовать, не добавлять в Windows runtime.
- **Объём / риски:** S–M / низкий. Главный риск — записать случайный дефект как желаемый контракт.
- **Тесты:** текущие defaults из preferences.md, drag/resize/pin, открытие модулей, reload, RU/EN, keyboard/overlays, protected admin actions, no eager textures. Подтверждённые дефекты пометить отдельными ожидаемыми исправлениями, не как норму.
- **Завершение:** воспроизводимый запуск на чистом checkout, фикстуры не обращаются к рабочим API/БД; контрольные снимки подтверждены.
- **Откат / безопасная остановка:** удалить только тестовую инфраструктуру; production поведение не менялось. Документы оставить отдельно от runtime.

## M1. Владение асинхронным запросом и граница сессии

- **Цель:** перестать применять поздние ответы к закрытой/сменившей аккаунт view; устранить класс ошибок P6.
- **Файлы / модули:** app.js, expedition.js renderUsers; затем renderGameplay/renderBenchmark после отдельной проверки; существующие хорошие примеры ideas.js, renderMarket, logistics.js используются как контракт.
- **Механизм:** RequestScope с auth epoch, generation, optional AbortController, isCurrent/accept-before-render, dispose. Пилот — только Users.
- **Зависимости:** M0. Переименование globals и ESM пока не требуется.
- **Объём / риски:** S–M / средний. Отмена запроса не отменяет уже выполненную сервером мутацию; одной отмены недостаточно без проверки актуальности.
- **Тесты:** поиск A/B с обратным порядком ответов; закрытие окна; logout/login другим игроком; смена роли; rejected/aborted request. Late response не вставляет private DOM, старая ошибка не создаёт toast новому аккаунту.
- **Завершение:** renderer не меняет DOM до проверки scope; старые Users API и pagination работают.
- **Откат / безопасная остановка:** обратимый adapter у одного renderer; откат сохраняет прежние layout/storage keys. Остальные модули остаются на существующих guards до их отдельных переносов.

## M2. Транспорт, snapshot и владелец ресурсов

- **Цель:** согласовать HTTP-ошибки и единственный путь обновления данных, не меняя игровые правила.
- **Файлы / модули:** app.js api/refreshData, station-labels.js personalizeStations; pilot MapData metadata read и один market read adapter.
- **Механизм:** ApiClient/ApiError; явные network/protocol/HTTP/abort ошибки; feature API adapter. ResourceStore управляет current snapshot, revision и in-flight metadata запросами. Station labels становятся presentation adapter, а не частью общего HTTP transport.
- **Зависимости:** M1.
- **Объём / риски:** M / средний-высокий. Ряд нынешних Go GET выполняет advancement/ensure/lazy generation: нельзя считать все GET чистыми и ввести общий TTL-кеш или автоматический retry.
- **Тесты:** JSON/non-JSON 401/403/409/500/502, offline, частичный отказ initial seven-endpoint refresh, logout во время ответа, station alias во всех views. Отдельно подтвердить семантику in-flight dedup для каждого выбранного endpoint.
- **Завершение:** прежние семь initial snapshot endpoints и их порядок применения сохраняются; нормализованы ошибки; нет импорта station feature из generic transport. Необязательный backend DTO-проект описан отдельно, не выполнен.
- **Откат / безопасная остановка:** старый api(path,body,signal) остаётся facade того же implementation; можно вернуть один adapter без возврата всех views. Не менять хранимые данные.

## M3. Контракт окна и его жизненный цикл

- **Цель:** отделить workspace window, transient editor и confirmation; ввести управляемое освобождение динамических экземпляров.
- **Файлы / модули:** windows.js Modules.create/title/closeAll, workbench.js UI.confirm, expedition.js actionAmount. Пилоты — confirm и amount, не сразу карты/корабль/все admin windows.
- **Механизм:** WindowRegistry с update title/config, show/hide, dispose; именованный ResizeObserver, снимаемый resize listener, owned timers; optional ephemeral lifetime. Совместимость Modules facade.
- **Зависимости:** M0, M1; overlay details согласовать с M4.
- **Объём / риски:** M / средний. Нельзя уничтожать каждое окно при close: потеряются draft, pin/link и актуальные пользовательские размеры.
- **Тесты:** смена title при повторном confirm, single/batch transfer size policy; 100 открытий того же ID без новых listeners; создание/удаление разных transient IDs; linked ship/cargo, narrow clamp, Escape/focus, logout. Проверка top-layer/inert для любого нового modal variant.
- **Завершение:** lifetime явный, destroy очищает регистрацию и наблюдателей; обычные рабочие окна продолжают быть немодальными и перемещаемыми.
- **Откат / безопасная остановка:** old window API и storage-compatible geometry сохраняются. Новая ephemeral policy включена только у пилотов; можно отключить policy без удаления preferences.

## M4. Всплывающий слой и позиционирование

- **Цель:** устранить разные реализации edge clamp/outside/Escape и ошибку P5.
- **Файлы / модули:** workbench.js menu/tooltip, controls.js picker, windows.js help, galaxy.js star summary. Пилот — UI.menu, затем UI.picker отдельной подзадачей.
- **Механизм:** FloatingPlacement и OverlayHost с explicit owner, anchor, side/flip, close-on-outside, Escape, focus return и scoped resize/scroll tracking.
- **Зависимости:** M1, M3.
- **Объём / риски:** M / средний-высокий. Portal в body может оказаться вне native modal top layer и inert scope. Увеличение z-index эту проблему не решает.
- **Тесты:** подменю справа/слева, пересечение промежутка pointer, ArrowRight/Left/Up/Down/Escape, touch selection, scroll и shrink/rotate viewport при открытом popup, nested popup в modal и немодальном workspace. Header admin menu остаётся открытым после «Пропустить тик».
- **Завершение:** одна реализация геометрии для перенесённых overlays; контекстные меню, combobox и passive tooltip сохраняют разные interaction policies.
- **Откат / безопасная остановка:** переключение adapter только одного overlay вида; отсутствует изменение business callbacks. Inline palette флага можно оставить без изменений.

## M5. Кнопки, поля и владение формой

- **Цель:** согласовать busy/disabled/validity/error и async callback contract без повторных нажатий/затёртой валидации.
- **Файлы / модули:** controls.js UI.button/field/dirty/tabs; пилот resource grant, затем одна quantity form; app auth/tick и planet reset — последующие отдельные переносы.
- **Механизм:** Button с controlled validity и busy, AsyncAction с явным promise, Field feedback и FormState. Existing UI.dirty становится adapter, не конкурирующей второй системой.
- **Зависимости:** M1–M4.
- **Объём / риски:** M / высокий для массового переноса, средний у пилота. Изменение finally может случайно включить кнопку, которую renderer хотел оставить заблокированной.
- **Тесты:** P1/P8; repeated click; async failure; изменение validity; native form Enter; loss of session/role; keyboard; current action ID повторяется при повторе того же server request. Внешний callback должен возвращать promise и не оставлять unhandled rejection.
- **Завершение:** один владелец pending/validity; disabled не выводится из длины переведённой подписи. Сервер по-прежнему проверяет количество, права и деньги.
- **Откат / безопасная остановка:** UI.button facade сохраняет старый статический режим для неперенесённых callers. Dynamic controller opt-in не меняет все 207 lexical sites сразу.

## M6. Defaults и preferences

- **Цель:** единый механизм хранения без навязывания всем элементам persistence.
- **Файлы / модули:** controls.js fold, windows.js storage, galaxy.js syncFilters, i18n.js language; пилоты planner ship/task и map filters.
- **Механизм:** PreferenceStore с default factory, account/browser/instance scope, versioned key, validation и migration; ephemeral state остаётся feature-local. Manual preference отличается от применения default.
- **Зависимости:** M0, M1. Объём / риски: M / средний, миграции preferences — повышенный.
- **Тесты:** anonymous bootstrap, storage denied/corrupt/old/new keys, два аккаунта, reload, poll, language, current per-mode defaults. Planner ship v2 closed, task details open, Maps branch closed, чужие корабли galaxy off/system on. Restore не должен писать server operation или вызывать selected action.
- **Завершение:** политики из preferences.md сохранены либо отдельно согласованы. Миграция старых глобальных window/density keys не угадывает, какой аккаунт был их владельцем.
- **Откат / безопасная остановка:** чтение старых ключей через reversible adapter; не удалять legacy keys до согласованного периода. Никакого сброса только ради единообразия.

## M7. Форматирование и локализация

- **Цель:** развить работающие UI.formatNumber/percent и I18n, убрать изолированные дефекты форматирования.
- **Файлы / модули:** controls.js, i18n.js, ru/en.json; pilot planet funding remaining + finance; затем ship/stellar labels и benchmark UI.
- **Механизм:** Format family с единицами/точностью/unknown policy; I18n поддерживает параметризованные сообщения без второго каталога. Raw имена, заметки и flags не переводятся.
- **Зависимости:** M0/M5. Объём / риски: S–M / низкий-средний.
- **Тесты:** RU/EN; 0/negative/unknown/Infinity; credits safe integers; малые массы/светимости с significant digits, чтобы 0.0078 не стало 0; percentage complements; scientific units vs Kelvin/Celsius; dates/durations/serverOffset. Не округлять payload или DTO.
- **Завершение:** нет нового локального formatter для уже описанной величины. Русские строковые ключи не переписаны массово; кириллические confirmation phrases остаются явно указанным исключением.
- **Откат / безопасная остановка:** маленькие formatter facades; старые catalog keys остаются, данные не мигрируют.

## M8. CSS tokens и визуальные контракты

- **Цель:** сделать текущий terminal style управляемым, не сменить дизайн.
- **Файлы / модули:** style.css и terminal033.css; пилот button/input/picker selected value, затем feedback и window chrome отдельными задачами.
- **Механизм:** semantic tokens, плотность, variants, состояния focus/selected/error/busy. Минимальные presentation classes вместо универсальной темы на одном radius.
- **Зависимости:** M0, M4/M5; компонентный реестр — источник контрактов. Объём / риски: M / средний-высокий из-за каскада.
- **Тесты:** computed style и screenshot разных состояний при RU/EN, compact/normal, 750/desktop, popup/inline context; thin rectangular buttons, квадратный system ID, отсутствие старого оранжевого фона, сохранение одобренных округлений.
- **Завершение:** у перенесённого component bucket один активный source of truth и удалены его прежние перекрывающиеся правила. Не добавлен новый override поверх всех старых.
- **Откат / безопасная остановка:** CSS bucket откатывается одной группой. Не вводить частичный @layer поверх unlayered legacy в предположении, что layer автоматически победит: обычные unlayered rules имеют больший приоритет.

## M9. Повторяющиеся составные представления

- **Цель:** общий presentation и управление списком без нового универсального UI-framework.
- **Файлы / модули:** controls.js table/tabs; logistics.js / expedition.js cargo tiles; studies.js и ideas.js pagers как пилоты.
- **Механизм:** Table/Pager/CursorMore, ResourceTile внутри inventory, общие SelectionSet primitives только где реально полезны.
- **Зависимости:** M4–M8. Объём / риски: M / средний.
- **Тесты:** local/server pages и cursor не смешиваются; поиск сбрасывает страницу и корректно сохраняет выбранное; Ctrl/⌘ multi-select в одном source; drag vs click; transfers stay same atomic API; inbox selection остаётся по IDs, не по resource source.
- **Завершение:** duplicate markup заменяется shared presentation, но ограничение по источнику, cargo capacity, права и цены не оказываются в core widget.
- **Откат / безопасная остановка:** заменить обратно только один renderer adapter, не откатывать сервер и не мигрировать inventories.

## M10. Границы JavaScript-модулей и Fleet feature

- **Цель:** безопасно перейти от script-order/global pollution к явным зависимостям; извлечь первый связанный feature.
- **Файлы / модули:** index.html, существующий bootstrap, expedition.js ship/cargo/shipStats/shipyards, holograms.js, test assets. Стартовать с Fleet/catalog view.
- **Механизм:** native ES modules, compatibility exports window.UI/Modules/Game на переходный срок; одна реализация функций. Feature owns state, API adapter и view model. Разделять base hull stats и effective ship stats.
- **Зависимости:** M1–M9 в нужной feature части. Объём / риски: M–L / средний-высокий.
- **Тесты:** cold startup, import order, lazy planet script, embedded Windows assets и URL/MIME; catalog/shipyard/guide same hull; current modules/effective stats, hologram size and 16s motion, cargo linked window.
- **Завершение:** первый feature не читает произвольные DOM/private variables другого; старый Game facade делегирует ему. Node не нужен для запуска Windows EXE.
- **Откат / безопасная остановка:** facade и entry bridge можно вернуть к classic defer, не меняя API/данные. Не сочетать перенос каталогов, смену framework и business changes одним коммитом.

## M11. Market subviews

- **Цель:** разорвать большой диспетчер renderMarket по реальным рынкам и их состоянию.
- **Файлы / модули:** expedition.js renderMarket/tradeForm/renderDeliveries/data offers; Shipyards из M10 подключается adapter.
- **Механизм:** local/global/orders/data/delivery subviews и MarketState. Переносить по одной вкладке; global с его already-correct request gate — хороший первый pilot.
- **Зависимости:** M2, M5, M7, M9, M10. Объём / риски: L суммарно; каждая вкладка M / средний-высокий.
- **Тесты:** reserve/collect/cancel, expected_price conflict, station choice/aliases/favorites, live updates без второй таблицы, сохранение quantity и quote, physical dock requirement. Смена tab закрывает/обновляет scope, а не применяет ответ старой tab.
- **Завершение:** одна tab/state registry; callbacks читают model, а не визуально восстановленный picker с другой closure переменной.
- **Откат / безопасная остановка:** одна вкладка заменяется bridge обратно; receipts/market contracts не меняются.

## M12. Corporations и admin screens — отдельно согласуемые подэтапы

- **Цель:** вынести оставшиеся крупные features из expedition/operations, используя уже проверенную инфраструктуру.
- **Файлы / модули:** corporations views (members/roles/finance/storage/projects/flag), users/resourcegrant/gameplay/admin logs/benchmark, ideas admin view.
- **Механизм:** feature controllers/API/view-model; generic UI не знает roles, money, galaxy preview или grant target.
- **Зависимости:** M1–M10. Объём / риски: L суммарно; перенос одного экрана M. Не разрешать массовый перенос всех этих экранов одной технической подзадачей.
- **Тесты:** основатели/академия перед player corps, 7 custom roles + authority, raw corporation names, server admin rejection, resource grant exact target + q query, role change/logout races, dirty cancel, disabled actions, preview token invalidation, benchmark только disposable DB.
- **Завершение:** один экран отделён, API/permissions не изменены, его sensitive context очищается при session change. Исправление q/search — отдельно отмеченный bugfix, не скрытое изменение поведения в рефакторинге.
- **Откат / безопасная остановка:** независимый screen adapter и feature bridge; не менять пользователя/корпорацию или seed ради UI переноса.

## M13. Карты и будущий Scene lifecycle

- **Цель:** переиспользовать небольшие инфраструктурные части, сохранив разные движки/координаты и ограничения видимости.
- **Файлы / модули:** galaxy.js, galaxy-admin.js preview, planet.js/planet-loader.js, holograms.js, workbench.js Visuals.
- **Механизм:** Read-model indexes, camera helpers где совпадает математика, hit-test adapters, visibility-aware draw scheduling, owned scene lifecycle. Space overview, star system и planetary surface остаются разными features.
- **Зависимости:** M1–M10; API contracts/revision fixtures. Объём / риски: L / высокий; три renderer переносить отдельно.
- **Тесты:** galaxy/system independent filters; постоянная route line и waypoint disclosure; label cursor radius/limits на 10k stars; unique-owner tooltip; pan/wheel/pinch, private visibility; exact terrain interpolation/geography hash; no eager textures, PNG failure/retry, late assets after exit; SVG IDs and reduced motion. Не превращать 100ms cadence в 60fps без измерения.
- **Завершение:** private state принадлежит scene и сбрасывается при logout; public art кеш отдельный; география и симуляция не меняются. Future LocalSectorScene использует версию server geography и отдельный runtime loop.
- **Откат / безопасная остановка:** renderer adapter переключается обратно при неизменных DTO/seed; не перегенерировать карты и не менять сохранённые сектора.

## M14. Очистка переходного слоя и закрепление правил

- **Цель:** завершить миграцию, а не оставить две системы и новый слой CSS-overrides.
- **Файлы / модули:** только уже перенесённые facades/selectors, docs/UI-catalog, минимальная CI job и release checks.
- **Механизм:** удалить доказанно неиспользуемые adapters/styles; тесты архитектурных границ, common callbacks и full feature regressions; developer guide с примерами.
- **Зависимости:** только завершённые feature buckets из предыдущих этапов, не обязательная одномоментная миграция всего проекта. Объём / риски: S–M на bucket / средний.
- **Тесты:** чистая сборка Go/Windows, все UI flows, RU/EN, user/admin, page reload/preferences, fixtures leakage guard, отсутствие private DOM после logout, отсутствие новых orphan listeners после destroy. Checksums всех старых EXE остаются корректными.
- **Завершение:** для задачи есть один documented component/API, CI проверяет поведение, нет неперенесённых callers удалённого facade.
- **Откат / безопасная остановка:** cleanup отделён от переноса поведения; revert восстанавливает только adapter/styles. Архитектурные defaults и данные не мигрируют вместе с cleanup.

## Решения, которые надо согласовать до соответствующего этапа

1. Какие preferences должны стать account-scoped: window geometry, density, sidebar. Язык до входа разумно оставить browser-scoped.
2. Где confirmation должен блокировать другие окна, а где нужен привычный немодальный редактор.
3. Нужно ли сохранять после reload guide categories, logistics branches, market tab и camera; сейчас политика неодинакова, и это не автоматически ошибка.
4. Какой объём кеша закрытых динамических окон нужен пользователю; dirty/pinned windows нельзя безусловно выбрасывать.
5. Какие статические/интеграционные UI-тесты и dev-зависимости принять. Production framework/build migration не является предварительным условием.

Это предложения для будущего согласования. На этапе аудита не изменено ни одно из перечисленных правил.
