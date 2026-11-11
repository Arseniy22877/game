package main

type HintStatus int

const (
	HintLockedStatus   HintStatus = 0
	HintUnlockedStatus HintStatus = 1
	HintUsedStatus     HintStatus = 2
)

type HintState struct {
	_hintData *Hint
	status    HintStatus
}

type StrategyStatus int

const (
	StrategyLockedStatus   StrategyStatus = 0
	StrategyUnlockedStatus StrategyStatus = 1
	StrategyUsingStatus    StrategyStatus = 2
)

type StrategyState struct {
	_strategyData *Strategy
	status        StrategyStatus
}

type Field struct {
	weights Vector
	power   Power
}

type EventStatus int

const (
	EventLockedStatus       EventStatus = 0
	EventAvailableStatus    EventStatus = 1
	EventFiredStatus        EventStatus = 2
	EventExtinguishedStatus EventStatus = 3
)

type EventState struct {
	_eventData  *GameEvent
	baseField   Field
	finalField  Field
	baseStatus  EventStatus
	finalStatus EventStatus
}

type AttributeStatus int

const (
	AttributeNeutral AttributeStatus = 0
	AttributeUp      AttributeStatus = 1
	AttributeDown    AttributeStatus = 2
)

type AttributeState struct {
	_attributeData *GameAttribute
	baseValue      float64
	finalValue     float64
	status         AttributeStatus
}

type BudgetState struct {
	_budgetData *Budget
	money       int
	qp          int
}

type GameState struct {
	_gameData  *Game
	attributes map[AttributeId]AttributeState
	events     map[EventId]EventState
	hints      map[HintId]HintState
	strategies map[StrategyId]StrategyState
	budget     BudgetState
	iteration  int
}