package main

type EventId int
type StrategyId int
type HintId int
type AttributeId int
type Vector []float64

type GameAttribute struct {
	id           AttributeId
	defaultValue float64
	maxValue     float64
	minValue     float64
	name         string
	description  string
}

type ConfigurableAttribute struct {
	GameAttribute
	costPerUnit float64
}

type Hint struct {
	id          HintId
	reliability string
	cost        int
	name        string
	description string
	secret      string
}

type Strategy struct {
	// Visible params
	id          StrategyId
	name        string
	description string
	cost        int

	// System change params
	parameterChanges []EventParametersChange
	attributeChanges []AttributeChange
	eventsUnlocked   []EventId
}

type Budget struct {
	initialMoney int
	maxMoney     int
	initialQp    int
}

type Power struct {
	amplitude float64
	density   float64
}

type EventParameters struct {
	power   Power
	weights Vector
}

type GameEvent struct {
	// Visible Params
	id          EventId
	name        string
	description string
	isPositive  bool

	// Dimensinal params
	basicParameters EventParameters
	centerPoint     Vector

	// System change params
	parameterChanges   []EventParametersChange
	attributeChanges   []AttributeChange
	hintsUnlocked      []HintId
	strategiesUnlocked []StrategyId
	eventsUnlocked     []EventId
	budgetChange       BudgetChange
}

// Changes

type EventParametersChange struct {
	eventId EventId
	shift   EventParameters
}

type AttributeChange struct {
	attributeId    AttributeId
	attributeShift float64
}

type BudgetChange struct {
	moneyShift int
	qpShift    int
}

// Main struct

type Game struct {
	basicAttributes        []GameAttribute
	configurableAttributes []ConfigurableAttribute
	events                 []GameEvent
	hints                  []Hint
	strategies             []Strategy
	startingBudget         Budget
	startingEvents         []EventId
	openingEvent           EventId
	probabilityStrictness  float64
	powerRestoreRate       float64
	maxEventsPerTurn       int
	softness               float64 // minimum probability of bad event to guarantee it
}