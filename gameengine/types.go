package gameengine

// Identifiers used by a scenario.
type EventId int
type StrategyId int
type HintId int
type AttributeId int

type Branch string

type Vector []float64

// GameAttribute describes one coordinate of the game state.
type GameAttribute struct {
	ID           AttributeId
	DefaultValue float64
	MaxValue     float64
	MinValue     float64
	Name         string
	Description  string
}

// ConfigurableAttribute is an attribute the player can set at the beginning
// of a cycle.
type ConfigurableAttribute struct {
	GameAttribute
	CostPerUnit float64
}

// Hint is information that can be unlocked and used during a cycle.
type Hint struct {
	ID          HintId
	Reliability string
	Cost        int
	Name        string
	Description string
	Secret      string

	Prerequisites Prerequisite
}

// Strategy applies temporary attribute changes and has a recurring cost.
type Strategy struct {
	ID          StrategyId
	Name        string
	Description string
	Cost        int

	Prerequisites    Prerequisite
	AttributeChanges []AttributeChange
}

// Budget contains the initial values and the money limit for a scenario.
type Budget struct {
	InitialMoney int
	MaxMoney     int
	InitialQP    int
}

type Prerequisite struct {
	// Events are alternatives: at least one listed event must have happened.
	Events []EventId
	// Strategies are alternatives: at least one listed strategy must be active.
	Strategies []StrategyId
}

// Threshold requires Min <= attribute value <= Max.
type Threshold struct {
	AttributeID AttributeId
	// Value is retained for compatibility with older scenario structs.
	// The current threshold logic uses Min and Max.
	Value float64
	Min   float64
	Max   float64
}

// GameEvent describes an event and its effects.
type GameEvent struct {
	ID          EventId
	Name        string
	Description string
	ImageURL    string
	IsPositive  bool

	// Weights follow the order: BasicAttributes, then ConfigurableAttributes.
	Weights       Vector
	BasicPower    float64
	Branch        Branch
	Iteration     int // 0 means that the event is available in every iteration.
	Prerequisites Prerequisite
	Thresholds    []Threshold

	AttributeChanges []AttributeChange
	BudgetChange     BudgetChange
}

type AttributeChange struct {
	AttributeID    AttributeId
	AttributeShift float64
}

type BudgetChange struct {
	MoneyShift int
	QPShift    int
}

// Game is the immutable scenario definition used to create GameStates.
type Game struct {
	BasicAttributes        []GameAttribute
	ConfigurableAttributes []ConfigurableAttribute
	Branches               []Branch
	Events                 []GameEvent
	Hints                  []Hint
	Strategies             []Strategy
	StartingBudget         Budget
	OpeningEvent           EventId
	MinProbabilitySum      float64
}

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
	ID         string
	GameData   *Game
	Attributes map[AttributeId]AttributeState
	Events     map[EventId]EventState
	Hints      map[HintId]HintState
	Strategies map[StrategyId]StrategyState
	Budget     BudgetState
	Iteration  int

	PendingAttributes []AttributeChange
	PendingStrategies []StrategyId
	PendingHints      []HintId
}
