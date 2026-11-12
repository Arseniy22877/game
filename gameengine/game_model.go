package gameengine

// Идентификатор события.
type EventId int

// Идентификатор стратегии.
type StrategyId int

// Идентификатор подсказки.
type HintId int

// Идентификатор атрибута.
type AttributeId int

// Вектор координат игрового состояния или параметров события.
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
}

// Стратегия с временными изменениями состояния и стоимостью поддержки.
type Strategy struct {
	// Параметры, отображаемые игроку.
	ID          StrategyId
	Name        string
	Description string
	Cost        int

	// Изменения, применяемые при активации стратегии.
	ParameterChanges []EventParametersChange
	AttributeChanges []AttributeChange
	EventsUnlocked   []EventId
}

// Правила и текущие пределы игрового бюджета.
type Budget struct {
	InitialMoney int
	MaxMoney     int
	InitialQP    int
}

// Амплитуда и плотность влияния события.
type Power struct {
	Amplitude float64
	Density   float64
}

// Параметры, по которым событие влияет на пространство состояний.
type EventParameters struct {
	Power   Power
	Weights Vector
}

// Описание события и его последствия после срабатывания.
type GameEvent struct {
	// Параметры, отображаемые игроку.
	ID          EventId
	Name        string
	Description string
	IsPositive  bool

	// Положение события и его базовые параметры в пространстве состояний.
	BasicParameters EventParameters
	CenterPoint     Vector

	// Изменения состояния, выполняемые после события.
	ParameterChanges   []EventParametersChange
	AttributeChanges   []AttributeChange
	HintsUnlocked      []HintId
	StrategiesUnlocked []StrategyId
	EventsUnlocked     []EventId
	BudgetChange       BudgetChange
}

// Изменение параметров поля конкретного события.

type EventParametersChange struct {
	EventID EventId
	Shift   EventParameters
}

type AttributeChange struct {
	AttributeID    AttributeId
	AttributeShift float64
}

// Изменение денег и очков качества.
type BudgetChange struct {
	MoneyShift int
	QPShift    int
}

// Полное описание игры, из которого создаётся GameState.
type Game struct {
	BasicAttributes        []GameAttribute
	ConfigurableAttributes []ConfigurableAttribute
	Events                 []GameEvent
	Hints                  []Hint
	Strategies             []Strategy
	StartingBudget         Budget
	StartingEvents         []EventId
	OpeningEvent           EventId
	ProbabilityStrictness  float64
	PowerRestoreRate       float64
	MaxEventsPerTurn       int
	// Минимальная вероятность плохого события, при которой оно гарантируется.
	Softness float64
}
