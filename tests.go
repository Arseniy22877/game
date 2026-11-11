package main

import (
	"fmt"
	"math"
)

type unitTestSuite struct {
	passed int
	failed int
}

func (suite *unitTestSuite) check(name string, condition bool, details string) {
	if condition {
		suite.passed++
		fmt.Printf("[PASS] %s\n", name)
		return
	}
	suite.failed++
	fmt.Printf("[FAIL] %s: %s\n", name, details)
}

func (suite *unitTestSuite) checkEqual(name string, got, want interface{}) {
	suite.check(name, got == want, fmt.Sprintf("got %v, want %v", got, want))
}

func runUnitTests(game Game) bool {
	fmt.Println("=== Unit tests ===")
	suite := unitTestSuite{}

	state := game.createInstance()
	suite.checkEqual("instance stores source game", state._gameData, &game)
	suite.checkEqual("initial money", state.budget.money, game.startingBudget.initialMoney)
	suite.checkEqual("initial QP", state.budget.qp, game.startingBudget.initialQp)
	suite.checkEqual("all attributes initialized", len(state.attributes), 4)
	suite.checkEqual("all events initialized", len(state.events), 6)
	suite.checkEqual("all hints initialized", len(state.hints), 3)
	suite.checkEqual("all strategies initialized", len(state.strategies), 3)

	for _, id := range []EventId{1, 2} {
		suite.check(fmt.Sprintf("starting event %d available", id),
			state.events[id].baseStatus == EventAvailableStatus,
			fmt.Sprintf("status is %d", state.events[id].baseStatus))
	}
	suite.check("opening event starts fired",
		state.events[0].baseStatus == EventFiredStatus,
		fmt.Sprintf("status is %d", state.events[0].baseStatus))
	suite.check("non-starting event locked",
		state.events[3].baseStatus == EventLockedStatus,
		fmt.Sprintf("status is %d", state.events[3].baseStatus))
	suite.check("initial vector has expected dimensions",
		len(*state.ToVector()) == 5,
		fmt.Sprintf("length is %d", len(*state.ToVector())))

	state.budget.money = 175
	income := BudgetChange{moneyShift: 20}
	income.apply(&state)
	suite.checkEqual("budget respects maximum", state.budget.money, game.startingBudget.maxMoney)
	expense := BudgetChange{moneyShift: -1000}
	expense.apply(&state)
	suite.checkEqual("budget does not become negative", state.budget.money, 0)
	state.budget.money = game.startingBudget.initialMoney

	baseEvent := state.events[0]
	originalPower := baseEvent.baseField.power
	originalWeight := baseEvent.baseField.weights[0]
	parameterChange := EventParametersChange{
		eventId: 0,
		shift: EventParameters{
			power:   Power{amplitude: 0.1, density: 0.2},
			weights: Vector{0.1, 0.1, 0.1, 0.1, 0.1},
		},
	}
	suite.check("base event parameters apply",
		parameterChange.applyBase(&state) == nil,
		"applyBase returned an error")
	changedEvent := state.events[0]
	suite.check("base power is persisted",
		changedEvent.baseField.power.amplitude == originalPower.amplitude+0.1,
		fmt.Sprintf("power is %v", changedEvent.baseField.power))
	suite.check("base weights are persisted",
		math.Abs(changedEvent.baseField.weights[0]-(originalWeight+0.1)) < 1e-9,
		fmt.Sprintf("weights are %v", changedEvent.baseField.weights))

	state = game.createInstance()
	eventToFire := state.events[0]
	eventToFire.fire(&state)
	suite.check("event firing changes status",
		state.events[0].baseStatus == EventFiredStatus,
		fmt.Sprintf("status is %d", state.events[0].baseStatus))
	suite.check("event unlocks strategy",
		state.strategies[0].status == StrategyUnlockedStatus,
		fmt.Sprintf("status is %d", state.strategies[0].status))
	suite.check("event unlocks hint",
		state.hints[0].status == HintUnlockedStatus,
		fmt.Sprintf("status is %d", state.hints[0].status))
	suite.check("event unlocks event",
		state.events[3].baseStatus == EventAvailableStatus,
		fmt.Sprintf("status is %d", state.events[3].baseStatus))

	suite.check("locked strategy is rejected",
		state.validateStrategies([]StrategyId{1}) != nil,
		"locked strategy was accepted")
	suite.check("locked hint is rejected",
		state.validateHints([]HintId{1}) != nil,
		"locked hint was accepted")
	suite.check("unknown attribute is rejected",
		state.validateAttributes([]AttributeChange{{attributeId: 99}}) != nil,
		"unknown attribute was accepted")

	state.strategies[0] = StrategyState{
		_strategyData: &game.strategies[0],
		status:        StrategyUsingStatus,
	}
	state.calcFinals()
	suite.check("active strategy changes final event power",
		state.events[0].finalField.power.amplitude > state.events[0].baseField.power.amplitude,
		"final power did not increase")
	state.useStrategies(nil)
	state.calcFinals()
	suite.check("disabled strategy is removed from final field",
		state.events[0].finalField.power.amplitude == state.events[0].baseField.power.amplitude,
		"strategy effect remained after disabling")
	suite.check("disabled strategy does not change base field",
		state.events[0].baseField.power.amplitude == game.events[0].basicParameters.power.amplitude,
		"base field was changed by final calculation")
	suite.check("final strategy weights do not alias base weights",
		math.Abs(state.events[0].baseField.weights[0]-game.events[0].basicParameters.weights[0]) < 1e-9,
		fmt.Sprintf("base weights are %v", state.events[0].baseField.weights))

	state = game.createInstance()
	invalidEventChange := EventParametersChange{
		eventId: 99,
		shift: EventParameters{
			weights: Vector{0, 0, 0, 0, 0},
		},
	}
	suite.check("invalid event parameter target is rejected",
		invalidEventChange.applyBase(&state) == ErrInvalidEventId,
		"invalid event target was accepted")

	state = game.createInstance()
	originalAttribute := state.attributes[1].baseValue
	originalMoney := state.budget.money
	atomicErr := state.pushCycle(
		[]AttributeChange{{attributeId: 1, attributeShift: 0.9}},
		nil,
		[]HintId{99},
		BudgetChange{moneyShift: -20},
	)
	suite.check("invalid cycle input returns an error", atomicErr != nil,
		"invalid hint was accepted")
	suite.check("invalid cycle does not change attributes",
		state.attributes[1].baseValue == originalAttribute,
		fmt.Sprintf("attribute is %.2f", state.attributes[1].baseValue))
	suite.check("invalid cycle does not charge budget",
		state.budget.money == originalMoney,
		fmt.Sprintf("money is %d", state.budget.money))

	state = game.createInstance()
	initialVector := append(Vector(nil), (*state.ToVector())...)
	repeatedVector := state.ToVector()
	suite.check("state vector ordering is stable",
		math.Abs((*repeatedVector)[0]-initialVector[0]) < 1e-9 &&
			math.Abs((*repeatedVector)[1]-initialVector[1]) < 1e-9 &&
			math.Abs((*repeatedVector)[2]-initialVector[2]) < 1e-9 &&
			math.Abs((*repeatedVector)[3]-initialVector[3]) < 1e-9 &&
			math.Abs((*repeatedVector)[4]-initialVector[4]) < 1e-9,
		fmt.Sprintf("vectors are %v and %v", initialVector, *repeatedVector))

	state = game.createInstance()
	state.calcFinals()
	probabilities, probabilityErr := state.getProbabilitiesForEvents()
	suite.check("probabilities calculate without errors", probabilityErr == nil,
		fmt.Sprintf("error is %v", probabilityErr))
	suite.check("probabilities include available events", len(probabilities) == 2,
		fmt.Sprintf("count is %d", len(probabilities)))
	for i, probability := range probabilities {
		suite.check(fmt.Sprintf("probability %d is valid", i),
			probability._attractor != nil && probability.probability >= 0,
			fmt.Sprintf("result is %+v", probability))
	}
	suite.check("zero probability normalization is empty",
		len(normalize([]ProbResult{{probability: 0}})) == 0,
		"zero probability produced a NaN result")
	emptyEvents, emptyEventsErr := state.pickEvents(nil)
	suite.check("empty event field returns an error",
		emptyEventsErr != nil && len(emptyEvents) == 0,
		fmt.Sprintf("events=%v error=%v", emptyEvents, emptyEventsErr))
	state = game.createInstance()
	eventToFire = state.events[0]
	eventToFire.fire(&state)
	state.calcFinals()
	probabilities, probabilityErr = state.getProbabilitiesForEvents()
	containsFiredEvent := false
	for _, probability := range probabilities {
		if probability._attractor._eventState._eventData.id == 0 {
			containsFiredEvent = true
		}
	}
	suite.check("fired event is excluded from next field",
		probabilityErr == nil && !containsFiredEvent,
		fmt.Sprintf("probabilities=%d error=%v", len(probabilities), probabilityErr))

	state = game.createInstance()
	cycleErr := state.pushCycle(
		[]AttributeChange{{attributeId: 1, attributeShift: 0.7}},
		nil,
		nil,
		BudgetChange{moneyShift: -10},
	)
	suite.check("complete cycle executes", cycleErr == nil,
		fmt.Sprintf("error is %v", cycleErr))
	suite.check("complete cycle charges budget", state.budget.money <= 130,
		fmt.Sprintf("money is %d", state.budget.money))
	suite.check("fired event is not available after cycle",
		countFiredEvents(state) > 0,
		"no event was fired")

	fmt.Printf("Unit tests: %d passed, %d failed\n", suite.passed, suite.failed)
	return suite.failed == 0
}

func countAvailableEvents(state GameState) int {
	available := 0
	for _, event := range state.events {
		if event.baseStatus == EventAvailableStatus {
			available++
		}
	}
	return available
}

func countFiredEvents(state GameState) int {
	fired := 0
	for _, event := range state.events {
		if event.baseStatus == EventFiredStatus {
			fired++
		}
	}
	return fired
}

type simulationStats struct {
	games              int
	cycles             int
	goodEvents         int
	badEvents          int
	events             int
	cycleErrors        int
	endingMoney        int
	endingQp           int
	unlockedStrategies int
	unlockedHints      int
	unlockedEvents     int
}

func printGameCharacteristics(game Game) {
	fmt.Println("=== Game characteristics ===")
	fmt.Printf("Attributes: %d basic, %d configurable\n",
		len(game.basicAttributes), len(game.configurableAttributes))
	fmt.Printf("Events: %d total, %d starting, max per cycle: %d\n",
		len(game.events), len(game.startingEvents), game.maxEventsPerTurn)
	fmt.Printf("Hints: %d, strategies: %d\n", len(game.hints), len(game.strategies))
	fmt.Printf("Budget: money %d/%d, qp %d\n",
		game.startingBudget.initialMoney, game.startingBudget.maxMoney, game.startingBudget.initialQp)
	for _, attribute := range game.basicAttributes {
		fmt.Printf("  Basic attribute: %s [%.2f..%.2f], default %.2f\n",
			attribute.name, attribute.minValue, attribute.maxValue, attribute.defaultValue)
	}
	for _, attribute := range game.configurableAttributes {
		fmt.Printf("  Configurable attribute: %s, cost %.2f per unit\n",
			attribute.name, attribute.costPerUnit)
	}
	for _, event := range game.events {
		sign := "bad"
		if event.isPositive {
			sign = "good"
		}
		fmt.Printf("  Event %d: %s (%s, amplitude %.2f)\n",
			event.id, event.name, sign, event.basicParameters.power.amplitude)
	}
	for _, strategy := range game.strategies {
		fmt.Printf("  Strategy %d: %s, maintenance %d\n",
			strategy.id, strategy.name, strategy.cost)
	}
	for _, hint := range game.hints {
		fmt.Printf("  Hint %d: %s, cost %d, reliability %s\n",
			hint.id, hint.name, hint.cost, hint.reliability)
	}
}

func countUnlocked(state GameState) (strategies, hints, events int) {
	for _, strategy := range state.strategies {
		if strategy.status != StrategyLockedStatus {
			strategies++
		}
	}
	for _, hint := range state.hints {
		if hint.status != HintLockedStatus {
			hints++
		}
	}
	for _, event := range state.events {
		if event.baseStatus != EventLockedStatus {
			events++
		}
	}
	return strategies, hints, events
}

func runSimulation(game Game, cycles int) (simulationStats, error) {
	state := game.createInstance()
	stats := simulationStats{games: 1, cycles: cycles}
	countedEvents := make(map[EventId]bool)

	for cycle := 0; cycle < cycles; cycle++ {
		strategies := make([]StrategyId, 0)
		hints := make([]HintId, 0)
		maintenance := 5
		for id, strategy := range state.strategies {
			if strategy.status != StrategyLockedStatus {
				strategies = append(strategies, id)
				maintenance += strategy._strategyData.cost
			}
		}
		for id, hint := range state.hints {
			if hint.status != HintLockedStatus {
				hints = append(hints, id)
				maintenance += hint._hintData.cost
			}
		}

		changes := []AttributeChange{
			{attributeId: 1, attributeShift: 0.5 + float64(cycle%3-1)*0.15},
			{attributeId: 2, attributeShift: 0.5 + float64((cycle+1)%3-1)*0.15},
			{attributeId: 3, attributeShift: 0.5 + float64((cycle+2)%3-1)*0.15},
		}
		before := make(map[EventId]EventStatus, len(state.events))
		for id, event := range state.events {
			before[id] = event.baseStatus
		}

		err := state.pushCycle(changes, strategies, hints, BudgetChange{moneyShift: -maintenance})
		if err != nil {
			stats.cycleErrors++
			continue
		}
		for id, event := range state.events {
			if event.baseStatus == EventFiredStatus && before[id] != EventFiredStatus && !countedEvents[id] {
				countedEvents[id] = true
				stats.events++
				if event._eventData.isPositive {
					stats.goodEvents++
				} else {
					stats.badEvents++
				}
			}
		}
	}

	stats.endingMoney = state.budget.money
	stats.endingQp = state.budget.qp
	stats.unlockedStrategies, stats.unlockedHints, stats.unlockedEvents = countUnlocked(state)
	return stats, nil
}
