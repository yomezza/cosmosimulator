# Обязательные сценарии ТЗ §50

Проверки находятся в `internal/server/integration`, а pure география/маски/поиск пути — в соответствующих `internal/geography` и `internal/planet/*`. Нагрузочный сценарий запускается отдельно; обычный `go test ./...` его пропускает без явного `RUN_PLANET_LOAD=1`.

| Требование | Основные автоматические сценарии |
|---|---|
| Постоянная география, вода/реки, согласованность соседей, отсутствие скрытой перегенерации | `TestPlanetGeography120`, `TestSceneSnapshotAndReservedRoutes`, `TestSceneCanonicalWaterAndPairedPortals`, `TestLocalScenes170`, `TestLocalSceneFencingAndSourceCAS170` |
| Изолированная/соседняя/удалённая территория, merge/split, разные владельцы, вода | `TestLandingTerritories160`, `TestLandingAccessAndWater160`, `TestTerritoryOwnershipAndSplit`, `TestCanonicalDryConnectivity`, `TestIslandsCannotBridgeOppositeEdges` |
| Backfill старых колоний, повтор и конкурентное завершение | `TestLandingBackfillAndConcurrentCompletion160` |
| Прибытие корабля, чужой/свободный сектор, corporate privilege, одноразовый ticket | `TestPlanetHandoff180`, `TestPlanetCorporateAndAtomicActivation180`, `TestPlanetAccessRevocation180`, `TestPlanetTicketRecovery180` |
| Позиция/сохранение/takeover/epoch/fence, восстановление БД | `TestPlanetMovement190`, `TestPlanetCheckpoints190`, `TestPlanetPresence200`, `TestPlanetDatabaseRecovery200`, `TestPlanetLoad230` |
| Обход препятствий, смена цели, недоступная точка, запрет ускорения | `TestBoundedPathsAndCorners`, `TestMovementDistanceAndLifecycle`, `TestLatestGoalAndPermit`, `TestPlanetMovement190`, проверка скорости в клиенте `TestPlanetLoad230` |
| Синхронизация двух игроков, enter/leave, отсутствие дубля controller | `TestPlanetPresence200`, браузерный `TestPlanetBrowser180`, `TestSectorPresenceCoalesces` |
| Разрешённый сосед, отзыв/чужой/свободный, cancel/no-confirm, правильный ingress, capacity | `TestPlanetTransitions210`, `TestPlanetTransitionRevocation210`, `TestPlanetTransitionCapacity210`, браузерный `TestPlanetTransitionsBrowser210` |
| Два настоящих процесса, аварийное завершение/перезапуск, продолжение cosmic tick | `TestPlanetLoad230`; in-process проверки `TestPlanetFoundation150`, `TestPlanetFencing150`, `TestPlanetDatabaseRecovery200` дополняют сценарии отказов |
| Отсутствие planet DDL/bootstrap/финансовых прав, несовместимая схема | `TestPlanetSchema150`, `TestPlanetRestrictedRole150/180`, `TestLocalSceneRestrictedRole170`, `TestServiceMetrics230` |
| Reset barrier/retirement, отсутствие dangling refs, свежая revision | `TestLocalSceneRetirement170`, `TestLocalSceneFencingAndSourceCAS170`, `TestPlanetHandoff180`, существующие проверки `planet124_test.go` |
| Регрессии космической игры | Полный `go test ./...` в отдельной `_test` БД; браузерный `TestTerminalBrowser` |
| 50 пилотов в одном/пяти секторах, 250 moves/s, cold/warm, disconnect/eviction, переходы | Отдельный `TestPlanetLoad230` и опубликованный JSON с фазами/сэмплами |

Визуальные RU/EN, узкий экран, поздняя загрузка и art cache проверяются в `tests/ui/planet*.test.mjs` и живом браузере. Пятьдесят клиентов нагрузки — протокольные клиенты, не 50 одновременно запущенных браузерных Canvas. Нативный Windows SCM, длительный soak на сутки и окончательная художественная приёмка этим набором не подменяются.
