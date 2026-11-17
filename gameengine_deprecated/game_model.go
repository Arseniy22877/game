package gameengine

// Идентификатор события.
type EventId int

// Идентификатор стратегии.
type StrategyId int

// Идентификатор подсказки.
type HintId int

// Идентификатор атрибута.
type AttributeId int

type Branch string

type Vector []float64

// Атрибут, который описывает измерение состояния игры.
type GameAttribute struct {
	ID           AttributeId
	DefaultValue float64
	MaxValue     float64
	MinValue     float64
	Name         string
	Description  string
}

// Атрибут, значение которого игрок может менять в начале цикла.
type ConfigurableAttribute struct {
	GameAttribute
	CostPerUnit float64
}

// Подсказка, которую можно открыть и использовать в цикле.
type Hint struct {
	ID          HintId
	Reliability string
	Cost        int
	Name        string
	Description string
	Secret      string

	Prerequisites Prerequisite
}

// Стратегия с временными изменениями состояния и стоимостью поддержки.
type Strategy struct {
	// Параметры, отображаемые игроку.
	ID          StrategyId
	Name        string
	Description string
	Cost        int

	Prerequisites Prerequisite
	// Изменения, применяемые при активации стратегии.
	AttributeChanges []AttributeChange
}

// Правила и текущие пределы игрового бюджета.
type Budget struct {
	InitialMoney int
	MaxMoney     int
	InitialQP    int
}

type Prerequisite struct {
	Events     []EventId
	Strategies []StrategyId
}

type Threshold struct {
	AttributeID AttributeId
	Value       float64
	Min         float64
	Max         float64
}

// Описание события и его последствия после срабатывания.
type GameEvent struct {
	// Параметры, отображаемые игроку.
	ID          EventId
	Name        string
	Description string
	ImageURL    string
	IsPositive  bool

	// Положение события и его базовые параметры в пространстве состояний.
	Weights          Vector
	BasicPower       float64
	Branch           Branch
	Iteration        int
	Prerequisites    Prerequisite
	Thresholds       []Threshold
	AttributeChanges []AttributeChange
	BudgetChange     BudgetChange
}

// Изменение параметров поля конкретного события.

type AttributeChange struct {
	AttributeID    AttributeId
	AttributeShift float64
}

// Изменение денег и очков качества.
type BudgetChange struct {
	MoneyShift int
	QPShift    int
}

// Полное описание игры, из которого создаётся GameState. Это один сценарий.
type Game struct {
	BasicAttributes        []GameAttribute
	ConfigurableAttributes []ConfigurableAttribute
	Branches               []Branch
	Events                 []GameEvent
	Hints                  []Hint
	Strategies             []Strategy
	StartingBudget         Budget
	OpeningEvent           EventId
	MaxEventsPerTurn       int
}
