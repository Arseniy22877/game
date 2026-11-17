package gameengine

type HintStatus int

const (
	HintNeutralStatus HintStatus = iota
	HintUsedStatus
)

const HintUnlockedStatus HintStatus = HintNeutralStatus

type HintState struct {
	HintData *Hint
	Status   HintStatus
}

type StrategyStatus int

const (
	StrategyNeutralStatus StrategyStatus = iota
	StrategyUsingStatus
)

type StrategyState struct {
	StrategyData *Strategy
	Status       StrategyStatus
}

type EventStatus int

const (
	EventNeutral EventStatus = iota
	EventFiredStatus
	EventExtinguishedStatus
)

type EventState struct {
	EventData *GameEvent
	Status    EventStatus
}

type AttributeStatus int

const (
	AttributeNeutral AttributeStatus = iota
	AttributeUp
	AttributeDown
)

type AttributeState struct {
	AttributeData *GameAttribute
	BaseValue     float64
	FinalValue    float64
	Status        AttributeStatus
}

type BudgetState struct {
	BudgetData *Budget
	Money      int
	QP         int
}

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
