package gameengine

// Состояние доступности подсказки.
type HintStatus int

const (
	HintLockedStatus   HintStatus = 0
	HintUnlockedStatus HintStatus = 1
	HintUsedStatus     HintStatus = 2
)

type HintState struct {
	HintData *Hint
	Status   HintStatus
}

// Состояние доступности стратегии.
type StrategyStatus int

const (
	StrategyLockedStatus   StrategyStatus = 0
	StrategyUnlockedStatus StrategyStatus = 1
	StrategyUsingStatus    StrategyStatus = 2
)

type StrategyState struct {
	StrategyData *Strategy
	Status       StrategyStatus
}

// Поле влияния события: веса координат и форма распределения.
type Field struct {
	Weights Vector
	Power   Power
}

// Состояние доступности или завершённости события.
type EventStatus int

const (
	EventLockedStatus       EventStatus = 0
	EventAvailableStatus    EventStatus = 1
	EventFiredStatus        EventStatus = 2
	EventExtinguishedStatus EventStatus = 3
)

type EventState struct {
	EventData   *GameEvent
	BaseField   Field
	FinalField  Field
	BaseStatus  EventStatus
	FinalStatus EventStatus
}

// Направление изменения атрибута.
type AttributeStatus int

const (
	AttributeNeutral AttributeStatus = 0
	AttributeUp      AttributeStatus = 1
	AttributeDown    AttributeStatus = 2
)

type AttributeState struct {
	AttributeData *GameAttribute
	BaseValue     float64
	FinalValue    float64
	Status        AttributeStatus
}

// Текущее значение бюджета и ссылка на его правила.
type BudgetState struct {
	BudgetData *Budget
	Money      int
	QP         int
}

// Изменяемое состояние партии.
// Pending-поля хранят выбор игрока между стадиями API.
type GameState struct {
	ID                string
	GameData          *Game
	Attributes        map[AttributeId]AttributeState
	Events            map[EventId]EventState
	Hints             map[HintId]HintState
	Strategies        map[StrategyId]StrategyState
	Budget            BudgetState
	Iteration         int
	PendingAttributes []AttributeChange
	PendingStrategies []StrategyId
	PendingHints      []HintId
}
