# Разработка интерфейса после 0.13.0

Начинать с [действующего каталога](CATALOG.ru.md) и владельца состояния. Исторические components/preferences реестры полезны для совместимости, но старые JS-пути из M0 больше не являются текущим графом импортов.

## Граница новой функции

1. Определить владельца: серверные данные, принятый snapshot, private feature state, draft, ручная preference или public art cache.
2. Найти существующий механизм в каталоге. Одинаковый вид не означает одинаковую игру: local trade, reservation и физическая доставка сохраняют свои API.
3. До изменения проверить defaults, close/reopen, reload, язык, logout/login и loading/empty/error.
4. Использовать core/UI либо расширить существующую единственную реализацию. Новый feature-specific компонент нужен, когда действительно различаются lifecycle или игровые правила.
5. Добавить проверку значимого риска и выполнить нужные существующие тесты.

`core` не импортирует UI/features. Generic `ui` не импортирует features, не читает цены/роли/кошелёк и не создаёт игровые команды. В `features` остаются presentation adapters станций/объектов, resource tile с доменными данными, права и ограничения источников груза. Нельзя вернуть неявные script globals или второй HTTP wrapper.

## Async и окна

Для заменяемого чтения использовать `RequestScope` и проверять lease до DOM:

```js
const scope = new RequestScope(() => Session.signedIn && Modules.isOpen(id));
async function render() {
  const lease = scope.begin();
  try {
    const data = await api(path, undefined, lease.signal);
    if (!lease.current()) return;
    draw(data);
  } catch (error) {
    if (lease.current() && error.name !== 'AbortError') notify(error.message, 'error');
  }
}
```

На close/смену параметров/logout инвалидировать scope и свои timers. Для admin view active predicate учитывает роль; privacy logout очищает private buffers/DOM даже у dirty/pinned редактора. Обработчик не должен применить результат только потому, что HTTP успешно завершился. Abort не отменяет уже принятую сервером команду.

Рабочие окна обычно немодальные. `Modules.close` сохраняет обычное окно, `Modules.destroy` освобождает динамическое. Для краткого editor указывать `{transient:true}`; dirty/pin имеют явную политику. Чувствительным административным окнам указывать `audience:'admin'`. Заголовок менять через registry API; не заводить параллельный resize listener на каждый render.

## Actions и формы

`UI.button`/`bindAction` владеют pending, а feature — desired disabled. Callback обязан вернуть promise своей операции. Не отрывать `async` работу через fire-and-forget, не выводить disabled из длины переведённой надписи. `finally` не должен безусловно включать кнопку, которую новая валидация оставила заблокированной.

`UI.field`, `UI.dirty`, `FormState` отвечают за форму. Draft хранится у feature с нужной областью (аккаунт/корабль/вкладка/источник) и очищается при logout. Restore не вызывает onChange бизнес-команды. Сервер независимо проверяет деньги, диапазоны и полномочия. Существующие request/receipt ID и expected price не менять при UI-переносе.

## Popup и представление

Menu, picker и tooltip используют общий `floating` host/placement. Native modal требует host внутри top layer; одного большого z-index недостаточно. Не объединять passive tooltip и keyboard menu в один interaction policy. Async callback picker/menu должен возвращать promise; ошибки принадлежат актуальному окну/сессии.

Форматирование — `Format`, перевод — `I18n`, пользовательские имена и заметки — raw text. Не менять DTO ради локального имени станции: использовать `features/stations` projection/reference. DOM с пользовательским текстом строить через textContent/`UI.node`, не через innerHTML.

Нумерованные страницы используют `pager`, курсорная выдача — отдельный cursor contract. Общие resource tiles не должны разрешать смешивание источников, обход docking, capacity или permission checks. Selection inbox по notification IDs не является cargo selection.

## Настройки, CSS и сцены

Preferences opt-in; default и scope выбирает feature. Сохранять старые ключи и plain-string совместимость. Poll/перевод/повторный render не считаются ручным действием игрока. Без отдельной задачи не переселять все browser-scoped keys в account scope.

Новые стили добавлять в соответствующий component bucket `styles/terminal.css`, общие значения — в `styles/tokens.css`. Удалять заменённую декларацию, а не добавлять очередной override в конец. Сохранять тонкие прямоугольные actions, квадратный system ID и отдельно согласованные rounded элементы. Обычные media/state правила не являются дубликатами только из-за одинакового селектора.

Draw scheduling не управляет серверной симуляцией. При закрытой/фоновой карте приостанавливать рисование, очищать private scene state при logout. Public PNG/SVG cache ограничен и не содержит игровые API-ответы. Planet loader остаётся lazy; canonical geography и её revision задаёт космический сервер. Нельзя вводить независимый клиентский генератор береговой линии или повышать cadence до 60 fps без измерения.

## Проверка перед выпуском

Запуск: [tests/ui/README.ru.md](../../../tests/ui/README.ru.md). Browser mocks проверяют клиентское поведение, Go integration с одноразовой БД — серверные транзакции. Для новых concurrent mutations выбирать реальные race-сценарии; не считать целевой race полным прогоном. Проверить RU/EN, narrow viewport, отсутствие private DOM после logout и состояние preferences.

Windows EXE включает web assets через Go embed. npm/Playwright — dev-only, frontend build framework не нужен. Версии в downloads сохраняются, SHA256 обновляется для нового файла. Правила будущей второй службы — в [service-boundaries](../service-boundaries.ru.md).
