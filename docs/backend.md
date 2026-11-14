# Бэкенд: принятые решения

Документ для исполнителя задач #4–#12. Здесь записано, **как** делаем бэкенд; **что** он отдаёт фронту, описано в [`api.md`](api.md).
Если что-то из этого документа мешает задаче, не обходи молча: напиши владельцу бэка и поправь документ в том же PR.

Приоритеты при любом выборе: (1) занятие не должно сорваться, (2) игру можно восстановить после падения, (3) кода мало и он читается. Производительность не важна: это 5–15 команд и один препод.

## 1. Пакеты и зависимости

```
cmd/server/            main: конфиг, сборка зависимостей, запуск, graceful shutdown
internal/api/          DTO контракта: только типы и константы, без импортов из internal/
internal/enginebridge/ единственный пакет, который импортирует gameengine
internal/store/        SQLite: миграции и запросы
internal/game/         Manager: конечный автомат, состояние активной игры в памяти
internal/ws/           WebSocket-хаб
internal/httpapi/      роутер, middleware, хендлеры, маппинг ошибок
migrations/            *.sql + embed.go
```

Направление импортов (стрелка = «импортирует»), циклов быть не должно:

```
cmd/server → httpapi → game → enginebridge → gameengine
                 │       ├──→ store
                 └→ ws   └──→ api (типы)        enginebridge, store, ws, httpapi → api
```

- `internal/api` остаётся пакетом типов. Хендлеры в `internal/httpapi`, иначе получится цикл `api → game → enginebridge → api`.
- `game` не импортирует `ws`. Он объявляет у себя интерфейс `Notifier` (см. раздел 6), а `cmd/server` передаёт в него хаб.
- `store` ничего не знает про движок и про `game`. Он принимает и возвращает простые структуры и DTO из `api`, сериализованные в JSON.

Внешние зависимости, других не добавляем:

| Зависимость | Зачем |
|---|---|
| `modernc.org/sqlite` | SQLite без CGO |
| `github.com/coder/websocket` | WebSocket |

Роутер — стандартный `net/http.ServeMux` с шаблонами Go 1.22 (`mux.HandleFunc("POST /api/teams", h)`, `r.PathValue("gameId")`). Логи — `log/slog`. Если зависимость потребует более новую директиву `go` в `go.mod`, поднимаем её.

## 2. Конфигурация и запуск (#4)

| Env | По умолчанию | Смысл |
|---|---|---|
| `ADMIN_TOKEN` | нет, обязателен | токен препода. Пустой или короче 8 символов — сервер не стартует |
| `PORT` | `8080` | слушаем `:<PORT>` |
| `DB_PATH` | `game.db` | файл SQLite |
| `SCENARIOS_DIR` | `scenarios` | папка со сценариями и подпапкой `images/` |
| `WEB_DIR` | `web/dist` | собранный фронт. Если папки нет, статика не раздаётся (только API) |

Порядок старта: конфиг → логгер → открыть БД и применить миграции → загрузить сценарии → `game.NewManager` (внутри восстановление активной игры, раздел 8) → HTTP-сервер. Ошибка на любом шаге — лог и `os.Exit(1)`.

Маршруты вне контракта:
- `GET /healthz` → `200 ok`;
- `GET /images/...` → файлы из `SCENARIOS_DIR/images/`;
- всё остальное, что не начинается с `/api/`, `/ws`, `/images/`, → статика из `WEB_DIR`, а если файла нет, отдаём `index.html` (SPA).

Остановка по `SIGINT`/`SIGTERM`: `http.Server.Shutdown` с таймаутом 5 секунд, затем закрыть хаб и БД.

Middleware (в этом порядке): recover (паника → лог со стеком → `500 internal`), лог запроса (метод, путь **без query**, статус, длительность), `http.MaxBytesReader` на 64 КБ, авторизация для групп `/api/team*` и `/api/admin/*`.

## 3. Движок через `enginebridge` (#6)

### 3.1. Загрузка сценариев

`gameengine.LoadGames()` **не используем**: папка в нём зашита как `scenarios` относительно рабочей директории, и он не умеет отдавать список id. Вместо этого мост делает то же самое через публичные функции движка:

1. читает `SCENARIOS_DIR/*.json`;
2. для каждого файла: читает байты, `json.Unmarshal` в `gameengine.Game`, проверяет (ниже), кладёт в свой каталог `id → Scenario{ID, Raw []byte, game *gameengine.Game}`;
3. экземпляр создаёт через `game.CreateInstance()` и ставит `state.ID = id`.

Глобальный реестр движка (`RegisterGame`/`GetGameInstance`) не трогаем. Сырые байты сценария нужны, чтобы сохранить их в БД при старте игры.

Проверка сценария при загрузке. Невалидный сценарий пропускаем с `slog.Error` (с именем файла и причиной), сервер продолжает работать:
- `StartingBudget.InitialMoney > 0`; у каждого атрибута `MaxValue > MinValue` (иначе движок делит на ноль);
- id уникальны: атрибуты (базовые и настраиваемые вместе), события, стратегии, исследования;
- у каждого события `len(CenterPoint) == len(BasicParameters.Weights) == 1 + число всех атрибутов`. Вектор состояния движка — это деньги и затем все атрибуты по возрастанию id;
- все ссылки (`StartingEvents`, `*Unlocked`, `AttributeChanges`, `ParameterChanges`) указывают на существующие id.

> Текущий `scenarios/demo.json` эту проверку не проходит (у события вектор длины 1, а нужно 2) и для разработки бесполезен: в нём нет слайдеров, стратегий, исследований и доступных событий. Для тестов и ручного запуска делаем свой сценарий `internal/enginebridge/testdata/full.json` (2 ключевых атрибута, 2 слайдера, 2 стратегии, 2 исследования, 4–5 событий, часть из которых открывает стратегии и исследования и меняет бюджет). Локальный запуск: `SCENARIOS_DIR=internal/enginebridge/testdata`. Файлы в `scenarios/` не правим: это зона движка.

### 3.2. Интерфейс моста

```go
type Catalog struct{ /* id → *Scenario */ }
func LoadCatalog(dir string) (*Catalog, error)
func (c *Catalog) IDs() []string                 // по алфавиту
func (c *Catalog) Get(id string) (*Scenario, bool)
func ParseScenario(id string, raw []byte) (*Scenario, error) // для восстановления из БД

func (s *Scenario) Raw() []byte
func (s *Scenario) Opening() *api.Event          // nil, если OpeningEvent нет среди событий
func (s *Scenario) NewInstance() *Instance

func (i *Instance) State() api.TeamGameState     // History пустой, его заполняет game
func (i *Instance) Bankrupt() bool
func (i *Instance) Submit(d api.Decision) (api.Cost, error)
func (i *Instance) DefaultDecision() api.Decision
func (i *Instance) Calculate() ([]api.Prediction, error)
func (i *Instance) SkipRound() ([]api.Prediction, error) // раунд без решения, для банкрота
func (i *Instance) Fire(ids []int) ([]api.Event, error)
```

`Instance` не потокобезопасен, его защищает мьютекс `game.Manager`. Наружу из пакета не выходит ни один тип `gameengine`.

Ошибки решения — тип `*enginebridge.DecisionError{Code, Message}` с кодами `invalid_decision`, `insufficient_money`, `insufficient_qp`. `Message` на русском, с названием сущности («Не хватает денег: нужно 120, есть 100», «Стратегия „Агрессивный рост“ пока недоступна»).

### 3.3. Что мост обязан делать сам

Все пункты проверены на текущем движке. Движок этого **не** ловит, поэтому ловит мост.

| № | Поведение движка | Что делает мост |
|---|---|---|
| 1 | `PassAttributes` принимает id любого атрибута, в том числе ключевого: команда могла бы напрямую выставить «Доверие» | `Submit` требует, чтобы в решении были ровно все настраиваемые атрибуты, по одному разу, значения в `[min, max]` и не NaN/Inf |
| 2 | Если в раунде не вызвать `PassAttributes`, слайдеры остаются на прежних уровнях **бесплатно** | Перед каждым `Calculate` в движок передан полный набор слайдеров. Решение по умолчанию тоже передаёт их явно |
| 3 | Использованное исследование можно купить повторно, QP спишутся | `Submit` принимает только исследования со статусом Unlocked |
| 4 | Повтор id в стратегиях или исследованиях списывает стоимость дважды | `Submit` отклоняет повторы |
| 5 | `FireEvents([1, 1])` применяет событие дважды | `Fire` отклоняет повторы и id не из последнего прогноза |
| 6 | `FireEvents(nil)` запускает случайный выбор | `Fire` в самом начале заменяет `nil` на `[]int{}`. Это единственное место вызова `FireEvents` |
| 7 | Нехватку денег и QP движок видит только в `CalculateState` | `Submit` считает стоимость сам: `gameengine.CycleBaseCost + state.AttributeCost(changes) + Σ Strategy.Cost`, QP = `Σ Hint.Cost`. Для слайдеров обязательно вызываем `AttributeCost` движка, формулу не дублируем |
| 8 | Тексты ошибок движка непригодны для показа (`%!w(<nil>), Invalid Strategy Id: 7`) | Мост проверяет всё сам до вызова `Pass*` и формирует свои сообщения. Если `Pass*` всё же вернул ошибку — `invalid_decision` с общим текстом, а текст движка идёт в лог |
| 9 | `GetPredicts` обходит map (порядок случайный) и при ошибке возвращает частичный результат | Сортируем по убыванию вероятности, при равенстве по id. Ошибку пишем в лог и работаем с тем, что вернулось |
| 10 | При деньгах меньше `CycleBaseCost` `CalculateState` не проходит никогда | Банкротство, раздел 3.4 |

Порядок внутри `Submit`: полная проверка → расчёт стоимости → `PassAttributes`, `PassStrategies`, `PassHints`. Если любой `Pass*` вернул ошибку, мост заново передаёт в движок предыдущее принятое решение (он его хранит), чтобы Pending-поля не остались наполовину новыми. `Submit` состояние игры не меняет, только Pending.

`Calculate` = `CalculateState()` → `GetPredicts()` → розыгрыш подсказки: `state.PickEvents(probs)`, выпавшие события помечаются `Suggested`. Ошибку `PickEvents` игнорируем (подсказок просто нет), элементы с `Attractor == nil` пропускаем. `Calculate` запоминает id событий прогноза для проверки в `Fire`.

`Fire(ids)` вызывается для каждой команды в каждом раунде, **даже с пустым списком**: движок в `FireEvents` переводит прошлые события из Fired в Extinguished. Ошибку `FireEvents` после успешной проверки id пишем в лог и продолжаем: это несогласованность сценария, а состояние уже изменено.

Преобразование в DTO:
- `attributes` — базовые атрибуты, `value = FinalValue`; `configurable` — настраиваемые, `value = BaseValue`;
- `strategies`, `hints` — только со статусом не Locked; `active` = Using; `used` = Used; `secret` отдаём только при `used`;
- все списки сортируем по id и создаём через `make`, чтобы в JSON был `[]`, а не `null`;
- `imageUrl`: пустую строку и адреса с `http://`/`https://` отдаём как есть, остальное приводим к виду `/images/<путь без ведущих "/" и "images/">`.

### 3.4. Банкротство

`Bankrupt()` = `money < CycleBaseCost + Σ round(min × CostPerUnit)` по всем слайдерам, то есть команда не может оплатить даже самый дешёвый раунд. Флаг не хранится, а вычисляется из состояния; `game` проверяет его в момент открытия каждого раунда (при старте и после применения событий) и запоминает на раунд.

Банкрот пропускает раунд, но не выбывает: автор движка планирует события вроде кредита, которые возвращают команду в игру. Поэтому прогнозы и события у банкрота есть.

При закрытии раунда вместо `Calculate()` для банкрота вызывается `SkipRound()`. `CalculateState` здесь вызвать нельзя (он всегда вернёт нехватку бюджета), поэтому мост делает то же самое без списания, публичными методами движка:
1. обнуляет `PendingAttributes`, `PendingStrategies`, `PendingHints`;
2. `UseStrategies(nil)` — отключает активные стратегии, раз за них не заплачено;
3. `CalculateFinals()` — пересчитывает итоговое поле (без этого события, открытые в прошлом раунде, не попадут в прогноз);
4. `GetPredicts()` и розыгрыш подсказки, как в `Calculate`.

Слайдеры остаются на прежних уровнях бесплатно: это один раунд, и сбрасывать их на минимум значило бы наказывать команду дважды. Дальше всё как у остальных: `Fire(ids)`, потом пересчёт флага на следующий раунд.

Автор движка собирается менять движок под настоящий сценарий. Если в нём появится штатная обработка нехватки денег, `SkipRound` заменяется на неё; снаружи моста ничего не меняется.

### 3.5. Решение по умолчанию

`DefaultDecision()`: слайдеры на текущих `BaseValue`, без стратегий и исследований, пустой отчёт. Если стоимость больше денег команды, все слайдеры ставятся на `min` (не банкрот может это оплатить по определению). Применяется при `force` и сохраняется в БД как обычное решение с `auto = 1`.

## 4. `game.Manager` (#7, #9, #10)

Один объект на процесс. Один `sync.Mutex` на всё. Каждый публичный метод соответствует одному эндпоинту и устроен одинаково:

```
lock → проверить фазу и входные данные → изменить состояние в памяти
     → записать в БД одной транзакцией → отправить WS-сообщения → unlock → вернуть DTO
```

- Проверки, которые могут отклонить запрос, выполняются **до** первого изменения в памяти.
- Запись в БД и отправка в хаб идут под мьютексом: так порядок сообщений совпадает с порядком изменений. Хаб никогда не блокируется (раздел 6).
- **Единое правило отката.** Если после начала изменений что-то пошло не так (ошибка БД, неожиданная ошибка движка), Manager выбрасывает состояние в памяти, заново строит его из БД тем же кодом, что при старте (раздел 8), и возвращает `500 internal`. Точечных откатов не пишем.

Состояние в памяти:

```go
type activeGame struct {
    id        int64
    phase     api.Phase
    round     int
    scenario  *enginebridge.Scenario // nil в лобби
    createdAt time.Time
    teams     []*team                // по возрастанию id
    byToken   map[string]*team
}
type team struct {
    id int64; name string; members []string; token string; ready bool
    inst        *enginebridge.Instance // nil в лобби
    bankrupt    bool                   // на текущий раунд
    decision    *api.Decision          // текущего раунда
    cost        *api.Cost
    submitted   bool                   // команда отправила сама
    predictions []api.Prediction       // только в round_closed
    history     []api.RoundEvents
}
```

Переходы:

| Метод | Из фазы | Что делает |
|---|---|---|
| `CreateGame(confirm)` | любая | Если идёт `round_open`/`round_closed` и нет `confirm` → `game_in_progress`. Старую игру помечает `active = 0`, создаёт новую в `lobby`. WS: `phase_changed` всем, затем закрыть соединения команд старой игры |
| `RegisterTeam` | `lobby` | Проверка лимитов и уникальности имени (`strings.ToLower(strings.TrimSpace(name))`), токен, `ready = true`. WS: `lobby_updated` |
| `SetReady` | `lobby` | WS: `lobby_updated` |
| `DeleteTeam` | `lobby` | Удаляет из БД и памяти. WS: закрыть соединения команды, `lobby_updated` |
| `Start(scenarioId)` | `lobby` | Нет команд или не все готовы → `teams_not_ready`. Создаёт экземпляры, сохраняет сырой JSON сценария в игру, `round = 1`, считает банкротов. WS: `phase_changed` |
| `SubmitDecision` | `round_open` | `round` не тот → `wrong_phase`; банкрот → `team_bankrupt`; нормализация (обрезка строк, `nil` → `[]`), `inst.Submit`, upsert в БД. WS: `submission_updated` преподу и команде |
| `CloseRound(round, force)` | `round_open` | См. ниже |
| `ApplyEvents(round, selections, finish)` | `round_closed` | См. ниже |
| `Finish` | `round_open` | `round = round − 1`, фаза `finished`. Решения незакрытого раунда остаются в БД, но игнорируются. WS: `phase_changed` |

`CloseRound`:
1. Проверки: фаза, номер раунда; без `force` все не банкроты должны быть `submitted`, иначе `not_all_submitted`.
2. Для не сдавших: `DefaultDecision()` → `inst.Submit` (с `auto`).
3. Для каждой команды: `inst.Calculate()`, а для банкрота `inst.SkipRound()` → прогнозы, снимок `inst.State()`.
4. Одна транзакция: авто-решения, по строке `round_results` на команду, `games.phase = round_closed`.
5. WS: `phase_changed`.

`ApplyEvents`:
1. Проверки **для всех команд до первого `Fire`**: фаза, номер раунда, каждая команда указана ровно один раз (`not_all_teams` / `bad_request`), id событий без повторов и из прогноза команды (`invalid_event`).
2. Для каждой команды: `inst.Fire(ids)`, запись в `history`, снимок состояния.
3. Если не `finish`: `round++`, сброс `decision/cost/submitted/predictions`, пересчёт банкротов. Иначе фаза `finished`, `round` не меняется.
4. Одна транзакция: `fired_events` и `state_after_events` в `round_results`, новая фаза и раунд в `games`.
5. WS: каждой команде `team_state`, затем всем `phase_changed`.

Чтение: `TeamState(token)`, `AdminState()`, `GameInfo()` собирают DTO из памяти под тем же мьютексом. `online` берётся из хаба. История (`Games`, `GameDetail`) и оценки (`SetGrades`) читаются и пишутся напрямую в БД через `store`, для активной игры тоже; `SetGrades` под мьютексом проверяет, что игра не «активная и не завершённая».

Токены: 16 байт из `crypto/rand` в hex. Токен препода сравниваем через `subtle.ConstantTimeCompare`. В логи токены не пишем.

## 5. HTTP-слой (#9, #10, #11)

- Хендлер делает только: разобрать JSON → вызвать метод `Manager` → записать ответ. Логики фаз и валидации игры в хендлерах нет.
- Ошибки: `game` возвращает `*game.Error{Status int, Code string, Message string}` (ошибки моста он оборачивает в `422`). Одна функция `writeError` превращает её в `ErrorResponse`; любая другая ошибка → лог + `500 internal` с общим текстом. Невалидный JSON, превышение размера тела → `400 bad_request`. Неизвестный путь под `/api/` → `404 not_found` в том же формате.
- Неизвестные поля в JSON игнорируем (`DisallowUnknownFields` не включаем).
- Авторизация: `Authorization: Bearer <token>`. Для `/api/admin/*` сравнение с `ADMIN_TOKEN`; для `/api/team/*` поиск команды активной игры по токену; иначе `401 unauthorized`.
- Все ответы `Content-Type: application/json; charset=utf-8`. Для `/api/*` ставим `Cache-Control: no-store`.

## 6. WebSocket-хаб (#8)

```go
// объявлен в internal/game, реализован в internal/ws
type Notifier interface {
    ToAll(msg api.WSMessage)                 // препод, команды, гости
    ToAdmin(msg api.WSMessage)
    ToTeams(msg api.WSMessage)               // все команды
    ToTeam(teamID int64, msg api.WSMessage)
    Online(teamID int64) bool
    DropTeam(teamID int64)                   // закрыть соединения команды
    DropAllTeams()
}
```

- Подключение: `/ws?token=`. Пустой токен → гость; токен препода → admin; токен команды активной игры → team; иначе `401` до апгрейда. Роль определяет функция, которую хабу передаёт `cmd/server` (она спрашивает `Manager`).
- У соединения буферизованный канал на 16 сообщений и одна пишущая горутина. Отправка неблокирующая: если буфер полон, соединение закрывается (клиент переподключится и сделает `GET`). Таймаут записи 5 секунд.
- Сервер шлёт ping каждые 30 секунд; без ответа соединение закрывается. Читающая горутина нужна только чтобы заметить закрытие, входящие сообщения игнорируются (лимит чтения 1 КБ).
- Сообщение сериализуется один раз и рассылается байтами.
- У команды может быть несколько соединений. `online` = есть хотя бы одно. Хаб сообщает об изменении (первое открылось / последнее закрылось) через колбэк `OnPresence(teamID, online)`, который вызывает **не под своим мьютексом**; колбэк берёт мьютекс `Manager` и шлёт `lobby_updated` (в лобби) или `team_online` (в раундах). Порядок блокировок всегда `Manager → хаб`, обратного не бывает.
- Проверку `Origin` оставляем стандартную (тот же хост): фронт в проде на том же origin, в разработке ходит через прокси dev-сервера.

## 7. База данных (#5)

Открытие: один `*sql.DB`, `SetMaxOpenConns(1)` (все записи и так сериализованы мьютексом игры, зато нет `SQLITE_BUSY`). DSN:

```
file:<DB_PATH>?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)
```

Миграции: файлы `migrations/NNNN_name.sql`, встроены через `embed` (`migrations/embed.go`), применяются по порядку имён, каждая в транзакции; номер последней применённой хранится в `PRAGMA user_version`. Сторонних библиотек миграций не берём. Уже слитую миграцию не редактируем, а добавляем новую.

Схема (`0001_init.sql`). Сложные значения храним JSON-текстом в виде DTO из `api`: это журнал, запросов по содержимому нет.

```sql
CREATE TABLE games (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  phase         TEXT    NOT NULL,              -- lobby | round_open | round_closed | finished
  round         INTEGER NOT NULL DEFAULT 0,
  active        INTEGER NOT NULL DEFAULT 1,
  scenario_id   TEXT    NOT NULL DEFAULT '',
  scenario_json TEXT,                          -- копия файла сценария на момент старта
  created_at    TEXT    NOT NULL,
  started_at    TEXT,
  finished_at   TEXT
);
CREATE UNIQUE INDEX games_one_active ON games(active) WHERE active = 1;

CREATE TABLE teams (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  game_id    INTEGER NOT NULL REFERENCES games(id),
  name       TEXT    NOT NULL,
  name_key   TEXT    NOT NULL,                 -- lower(trim(name)), считает Go
  members    TEXT    NOT NULL,                 -- JSON ["Аня","Петя"]
  token      TEXT    NOT NULL UNIQUE,
  ready      INTEGER NOT NULL DEFAULT 1,
  created_at TEXT    NOT NULL,
  UNIQUE (game_id, name_key)
);

CREATE TABLE decisions (                       -- последнее решение команды за раунд
  team_id    INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  round      INTEGER NOT NULL,
  decision   TEXT    NOT NULL,                 -- JSON api.Decision
  cost_money INTEGER NOT NULL,
  cost_qp    INTEGER NOT NULL,
  auto       INTEGER NOT NULL DEFAULT 0,       -- решение по умолчанию при force
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (team_id, round)
);

CREATE TABLE round_results (                   -- строка появляется при закрытии раунда
  team_id            INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  round              INTEGER NOT NULL,
  predictions        TEXT    NOT NULL,         -- JSON []api.Prediction
  state_after_calc   TEXT    NOT NULL,         -- JSON api.TeamGameState
  fired_events       TEXT,                     -- JSON [4,7] в порядке применения; NULL, пока препод не выбрал
  state_after_events TEXT,                     -- JSON api.TeamGameState
  closed_at          TEXT    NOT NULL,
  events_at          TEXT,
  PRIMARY KEY (team_id, round)
);

CREATE TABLE grades (
  team_id      INTEGER NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  skill      TEXT    NOT NULL,                 -- код навыка из api.Skills
  score      INTEGER NOT NULL,
  comment    TEXT    NOT NULL DEFAULT '',
  updated_at TEXT    NOT NULL,
  PRIMARY KEY (team_id, skill)
);
```

Отличия от списка таблиц в issue #5, сделаны намеренно: участники лежат JSON-полем в `teams`; `rounds`, `predictions`, `fired_events`, `state_snapshots` слиты в одну `round_results`. У банкрота за пропущенный раунд есть строка в `round_results`, но нет строки в `decisions`: по этому признаку восстановление понимает, что раунд пропущен.

Время — `time.Now().UTC().Format(time.RFC3339)`. `SetGrades` — в одной транзакции удалить оценки команды и вставить новые. Удаление команды возможно только в лобби, когда ссылок на неё ещё нет.

## 8. Восстановление после перезапуска

При старте `Manager` берёт игру с `active = 1` (если её нет, фаза `none`) и строит состояние в памяти:

1. Команды из `teams`. Если фаза `lobby` — на этом всё.
2. Сценарий — `ParseScenario(scenario_id, scenario_json)` **из БД**, а не из папки: файл могли поменять после старта игры.
3. Для каждой команды новый экземпляр и прогон журнала по раундам `r = 1, 2, …`:
   - если есть `round_results[r]`: при наличии `decisions[r]` — `Submit(decisions[r])` → `Calculate()`, без него — `SkipRound()` (команда была банкротом); если `fired_events` не `NULL` — `Fire(fired_events)` и запись в `history`;
   - иначе, если `r == games.round`, фаза `round_open` и есть `decisions[r]`: только `Submit` (команда уже отправила решение в открытом раунде).
4. В фазе `round_closed` прогнозы команды берём из `round_results.predictions` (там сохранены пометки `suggested`), а не из результата `Calculate`.
5. Сверка: состояние после прогона сравниваем с последним сохранённым снимком команды (как JSON). Расхождение — `slog.Warn` с id команды, работу продолжаем на результате прогона.

Если прогон упал с ошибкой — лог и остановка сервера: молча начинать с пустого состояния поверх чужой игры нельзя.

Тот же код вызывается при откате из раздела 4. Отдельная функция `rebuild(gameID)`.

Все WS-соединения после перезапуска оборваны; клиенты переподключаются и делают `GET` состояния, поэтому досылать ничего не надо.

## 9. Тесты

Обязательный минимум, без него PR не принимается:

- **`enginebridge`**: на `testdata/full.json` — каждая строка таблицы из 3.3 (по тесту), стоимость решения совпадает с фактическим списанием после `Calculate`, банкротство, решение по умолчанию в обоих вариантах, проверка сценариев (валидный и по одному сломанному на каждое правило).
- **`store`**: миграции на пустой файл в `t.TempDir()`, повторный запуск миграций ничего не ломает, запись и чтение каждой таблицы.
- **`game`**: с настоящим `store` во временном файле и фальшивым `Notifier`, который копит сообщения:
  - полный сценарий занятия: создать → 2 команды → старт → 3 раунда (решения, `force` для одной команды, события, пустой выбор) → `finish: true`;
  - каждая ошибка из таблицы контракта хотя бы один раз;
  - **восстановление**: после каждого шага сценария создать второй `Manager` на той же БД и сравнить `AdminState()` обоих как JSON;
  - порядок и адресаты WS-сообщений на каждом переходе.
- **`httpapi`**: через `httptest` — авторизация (нет токена, чужой, препода, команды прошлой игры), формат ошибок, один сквозной прогон; проверка, что ни в одном ответе нет `null` на месте массива.
- **`ws`**: подключение по каждой роли, доставка адресату, `online` при двух соединениях одной команды, отключение медленного клиента.

Гонки: `go test -race ./...` должен проходить.

## 10. Деплой (#12)

- Сборка: `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o game-server ./cmd/server`.
- На сервере: `/opt/game/` с бинарником, `scenarios/`, `web/dist/`, `game.db`; отдельный пользователь `game`.
- systemd: `WorkingDirectory=/opt/game`, `EnvironmentFile=/opt/game/.env` (права `600`), `Restart=always`, `RestartSec=2`. Перезапуск посреди занятия безопасен благодаря разделу 8.
- Caddy: весь сайт — `reverse_proxy 127.0.0.1:8080`. Статику отдаёт сам Go-сервер, WebSocket Caddy проксирует без настроек.
- Бэкап: cron раз в сутки, `sqlite3 /opt/game/game.db ".backup '/opt/game/backup/game-$(date +%F).db'"`, хранить 14 дней.

## 11. Порядок работ

Каждая задача — отдельная ветка и PR, проверки из AGENTS.md перед PR.

1. **#3** контракт: PR с `docs/api.md`, `internal/api`, этим документом. Согласовать с фронтом.
2. **#4** скелет: конфиг, логи, `/healthz`, статика, остановка. Сценарии подключаются в шаге 4.
3. **#5** `store` и миграции.
4. **#6** `enginebridge` и тестовый сценарий.
5. **#8** хаб (не зависит от 3–4, можно параллельно).
6. **#7 + #9 + #10** `game.Manager` и хендлеры: сначала лобби и старт, потом раунд. Восстановление пишется вместе с переходами, а не после.
7. **#11** история и оценки.
8. **#12** деплой.

## 12. Что сознательно не делаем

- Несколько игр одновременно, роли кроме «препод» и «команда», аккаунты.
- Отмена закрытия раунда и выбора событий. Журнал это позволяет (удалить последние записи и прогнать заново), но в первую версию не входит.
- Вход команды после старта игры, переименование команды, удаление команды посреди игры (не сдавшую команду закрывает `force`).
- История переотправок решения: храним только последнее.
- CORS, rate limit, метрики, выгрузка в CSV (фронт строит таблицу из `GET /api/admin/games/{gameId}`).

## 13. Открытые вопросы к команде

По каждому есть решение по умолчанию, работа из-за них не останавливается.

| Вопрос | Кому | По умолчанию |
|---|---|---|
| Команда без денег | решено 14.11: пропускает раунд, но получает события (например, кредит) и может вернуться (3.4) | — |
| Где лежат картинки событий? | движок, фронт | `scenarios/images/`, отдаются по `/images/` |
| Шкала оценок по шести навыкам из ТЗ | решено 14.11: целое 0–10 | — |
| Настоящий сценарий: автор движка делает и, возможно, поменяет движок | движок | работаем на `testdata/full.json`; все вызовы движка только в мосте |
| Команды видят числовые значения ключевых атрибутов | решено 14.11: да | — |

Замечания по движку, которые стоит передать автору (мост их обходит, чинить не блокирует): строки 1–5 и 8–10 таблицы 3.3; `CalculateFinals` обходит стратегии в случайном порядке, и при обрезке значений по границам результат может зависеть от порядка; `LoadGames` не принимает путь и не отдаёт список id.
