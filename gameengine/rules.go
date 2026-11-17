package gameengine

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

const CycleBaseCost = 5

var (
	ErrInvalidStrategyId        = errors.New("invalid strategy id")
	ErrStrategyAccessNotAllowed = errors.New("strategy prerequisites are not met")
	ErrInvalidAttributeId       = errors.New("invalid attribute id")
	ErrInvalidEventId           = errors.New("invalid event id")
	ErrInvalidHintId            = errors.New("invalid hint id")
	ErrHintAccessNotAllowed     = errors.New("hint prerequisites are not met")
	ErrInsufficientBudget       = errors.New("insufficient budget")
	ErrInsufficientQP           = errors.New("insufficient QP")
	ErrConflictingStrategies    = errors.New("conflicting strategies")
	ErrEventNotAvailable        = errors.New("event is not available")
)

func clampValue(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func saturatingAddInt(a, b int) int {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	if b > 0 && a > maxInt-b {
		return maxInt
	}
	if b < 0 && a < minInt-b {
		return minInt
	}
	return a + b
}

func (c *BudgetChange) Apply(g *GameState) {
	if c == nil || g == nil {
		return
	}
	maxMoney := int(^uint(0) >> 1)
	if g.Budget.BudgetData != nil {
		maxMoney = max(0, g.Budget.BudgetData.MaxMoney)
	}
	money := clampInt(saturatingAddInt(g.Budget.Money, c.MoneyShift), 0, maxMoney)
	g.Budget = BudgetState{
		BudgetData: g.Budget.BudgetData,
		Money:      money,
		QP:         max(0, saturatingAddInt(g.Budget.QP, c.QPShift)),
	}
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (e *EventState) Fire(g *GameState) error {
	if e == nil || e.EventData == nil || g == nil {
		return ErrInvalidEventId
	}

	id := e.EventData.ID
	current, exists := g.Events[id]
	if !exists {
		return fmt.Errorf("%w: %d", ErrInvalidEventId, id)
	}
	if current.Status != EventNeutral || !g.eventCanOccur(current.EventData) {
		return fmt.Errorf("%w: %d", ErrEventNotAvailable, id)
	}
	if err := g.applyEventEffects([]*GameEvent{current.EventData}); err != nil {
		return err
	}

	current.Status = EventFiredStatus
	g.Events[id] = current
	e.Status = EventFiredStatus
	return nil
}

// ApplyBase changes an attribute's persistent value, then recalculates its
// final value so active strategy modifiers remain in effect.
func (ac *AttributeChange) ApplyBase(g *GameState) error {
	if ac == nil || g == nil {
		return ErrInvalidAttributeId
	}
	attribute, ok := g.Attributes[ac.AttributeID]
	if !ok || attribute.AttributeData == nil {
		return fmt.Errorf("%w: %d", ErrInvalidAttributeId, ac.AttributeID)
	}
	if math.IsNaN(ac.AttributeShift) || math.IsInf(ac.AttributeShift, 0) {
		return fmt.Errorf("invalid change for attribute %d", ac.AttributeID)
	}

	next := *g
	next.Attributes = cloneAttributeStates(g.Attributes)
	baseValue := attribute.BaseValue + ac.AttributeShift
	if math.IsNaN(baseValue) || math.IsInf(baseValue, 0) {
		return fmt.Errorf("invalid resulting base value for attribute %d", ac.AttributeID)
	}
	attribute.BaseValue = clampValue(
		baseValue,
		attribute.AttributeData.MinValue,
		attribute.AttributeData.MaxValue,
	)
	next.Attributes[ac.AttributeID] = attribute
	if err := next.CalculateFinals(); err != nil {
		return err
	}
	g.Attributes = next.Attributes
	return nil
}

func (ac *AttributeChange) ApplyFinal(g *GameState) error {
	if ac == nil || g == nil {
		return ErrInvalidAttributeId
	}
	attribute, ok := g.Attributes[ac.AttributeID]
	if !ok || attribute.AttributeData == nil {
		return fmt.Errorf("%w: %d", ErrInvalidAttributeId, ac.AttributeID)
	}
	if math.IsNaN(ac.AttributeShift) || math.IsInf(ac.AttributeShift, 0) {
		return fmt.Errorf("invalid change for attribute %d", ac.AttributeID)
	}
	attribute.FinalValue = clampValue(
		attribute.FinalValue+ac.AttributeShift,
		attribute.AttributeData.MinValue,
		attribute.AttributeData.MaxValue,
	)
	g.Attributes[ac.AttributeID] = attribute
	return nil
}

func (g *GameState) ValidateAttributes(changes []AttributeChange) error {
	if g == nil || g.GameData == nil {
		return errors.New("game data is nil")
	}
	var err error
	seen := make(map[AttributeId]struct{}, len(changes))
	configurable := make(map[AttributeId]struct{}, len(g.GameData.ConfigurableAttributes))
	for _, attribute := range g.GameData.ConfigurableAttributes {
		configurable[attribute.ID] = struct{}{}
	}

	for _, change := range changes {
		attribute, ok := g.Attributes[change.AttributeID]
		if !ok || attribute.AttributeData == nil {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrInvalidAttributeId, change.AttributeID))
			continue
		}
		if _, allowed := configurable[change.AttributeID]; !allowed {
			err = errors.Join(err, fmt.Errorf("attribute %d is not configurable", change.AttributeID))
		}
		if _, duplicate := seen[change.AttributeID]; duplicate {
			err = errors.Join(err, fmt.Errorf("attribute %d is selected more than once", change.AttributeID))
		}
		seen[change.AttributeID] = struct{}{}

		if math.IsNaN(change.AttributeShift) || math.IsInf(change.AttributeShift, 0) ||
			change.AttributeShift < attribute.AttributeData.MinValue ||
			change.AttributeShift > attribute.AttributeData.MaxValue {
			err = errors.Join(err, fmt.Errorf("invalid value %.3f for attribute %d", change.AttributeShift, change.AttributeID))
		}
	}
	return err
}

func (g *GameState) AttributeCost(changes []AttributeChange) int {
	if g == nil || g.GameData == nil {
		return 0
	}
	cost := 0
	for _, change := range changes {
		for i := range g.GameData.ConfigurableAttributes {
			attribute := &g.GameData.ConfigurableAttributes[i]
			if attribute.ID == change.AttributeID {
				cost = saturatingAddInt(cost, int(math.Round(change.AttributeShift*attribute.CostPerUnit)))
				break
			}
		}
	}
	return cost
}

func (g *GameState) eventCanOccur(event *GameEvent) bool {
	if event == nil || g == nil {
		return false
	}
	if event.Iteration != 0 && event.Iteration != g.Iteration {
		return false
	}
	return g.prerequisitesMet(event.Prerequisites) && g.thresholdsMet(event.Thresholds)
}

func (g *GameState) prerequisitesMet(prerequisites Prerequisite) bool {
	if !g.eventPrerequisitesMet(prerequisites.Events) {
		return false
	}
	if len(prerequisites.Strategies) == 0 {
		return true
	}
	for _, strategyID := range prerequisites.Strategies {
		strategy, ok := g.Strategies[strategyID]
		if ok && strategy.Status == StrategyUsingStatus {
			return true
		}
	}
	return false
}

func (g *GameState) eventPrerequisitesMet(eventIDs []EventId) bool {
	if len(eventIDs) == 0 {
		return true
	}
	for _, eventID := range eventIDs {
		event, ok := g.Events[eventID]
		if ok && (event.Status == EventFiredStatus || event.Status == EventExtinguishedStatus) {
			return true
		}
	}
	return false
}

// prerequisitesMetForStrategySet evaluates strategy prerequisites against the
// complete set that the player is about to activate, not the previous cycle's
// active strategies. Event prerequisites still use the current event history.
func (g *GameState) prerequisitesMetForStrategySet(
	prerequisites Prerequisite,
	selected map[StrategyId]struct{},
) bool {
	if !g.eventPrerequisitesMet(prerequisites.Events) {
		return false
	}
	if len(prerequisites.Strategies) == 0 {
		return true
	}
	for _, strategyID := range prerequisites.Strategies {
		if _, active := selected[strategyID]; active {
			return true
		}
	}
	return false
}

func (g *GameState) thresholdsMet(thresholds []Threshold) bool {
	for _, threshold := range thresholds {
		attribute, ok := g.Attributes[threshold.AttributeID]
		if !ok || math.IsNaN(attribute.FinalValue) || math.IsInf(attribute.FinalValue, 0) ||
			attribute.FinalValue < threshold.Min || attribute.FinalValue > threshold.Max {
			return false
		}
	}
	return true
}

func (g *GameState) strategyAvailable(strategy *Strategy) bool {
	return strategy != nil && g.prerequisitesMet(strategy.Prerequisites)
}

func (g *GameState) hintAvailable(hint *Hint) bool {
	return hint != nil && g.prerequisitesMet(hint.Prerequisites)
}

func (g *GameState) ValidateStrategies(ids []StrategyId) error {
	if g == nil {
		return errors.New("game state is nil")
	}
	selected := make(map[StrategyId]struct{}, len(ids))
	var err error

	for _, id := range ids {
		if _, duplicate := selected[id]; duplicate {
			err = errors.Join(err, fmt.Errorf("strategy %d is selected more than once", id))
			continue
		}
		if _, ok := g.Strategies[id]; !ok {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrInvalidStrategyId, id))
			continue
		}
		selected[id] = struct{}{}
	}

	checked := make(map[StrategyId]struct{}, len(selected))
	for _, id := range ids {
		if _, valid := selected[id]; !valid {
			continue
		}
		if _, alreadyChecked := checked[id]; alreadyChecked {
			continue
		}
		checked[id] = struct{}{}
		strategy := g.Strategies[id]
		if strategy.StrategyData == nil {
			err = errors.Join(err, fmt.Errorf("%w: %d (nil strategy data)", ErrInvalidStrategyId, id))
			continue
		}
		if !g.prerequisitesMetForStrategySet(strategy.StrategyData.Prerequisites, selected) {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrStrategyAccessNotAllowed, id))
		}
	}
	return err
}

func (g *GameState) ValidateHints(ids []HintId) error {
	if g == nil {
		return errors.New("game state is nil")
	}
	var err error
	seen := make(map[HintId]struct{}, len(ids))
	for _, id := range ids {
		if _, duplicate := seen[id]; duplicate {
			err = errors.Join(err, fmt.Errorf("hint %d is selected more than once", id))
			continue
		}
		seen[id] = struct{}{}

		hint, ok := g.Hints[id]
		if !ok || hint.HintData == nil {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrInvalidHintId, id))
			continue
		}
		if hint.Status == HintUsedStatus || !g.hintAvailable(hint.HintData) {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrHintAccessNotAllowed, id))
		}
	}
	return err
}

func (g *GameState) UseAttributes(changes []AttributeChange) error {
	if err := g.ValidateAttributes(changes); err != nil {
		return err
	}

	next := *g
	next.Attributes = cloneAttributeStates(g.Attributes)
	for _, change := range changes {
		attribute := next.Attributes[change.AttributeID]
		attribute.BaseValue = change.AttributeShift
		next.Attributes[change.AttributeID] = attribute
	}
	if err := next.CalculateFinals(); err != nil {
		return err
	}
	g.Attributes = next.Attributes
	return nil
}

func (g *GameState) UseStrategies(ids []StrategyId) error {
	if err := g.ValidateStrategies(ids); err != nil {
		return err
	}

	next := *g
	next.Attributes = cloneAttributeStates(g.Attributes)
	next.Strategies = cloneStrategyStates(g.Strategies)
	for id, strategy := range next.Strategies {
		strategy.Status = StrategyNeutralStatus
		next.Strategies[id] = strategy
	}
	for _, id := range ids {
		strategy := next.Strategies[id]
		strategy.Status = StrategyUsingStatus
		next.Strategies[id] = strategy
	}
	if err := next.CalculateFinals(); err != nil {
		return err
	}
	g.Strategies = next.Strategies
	g.Attributes = next.Attributes
	return nil
}

func (g *GameState) UseHints(ids []HintId) error {
	if err := g.ValidateHints(ids); err != nil {
		return err
	}
	for _, id := range ids {
		hint := g.Hints[id]
		hint.Status = HintUsedStatus
		g.Hints[id] = hint
	}
	return nil
}

// CalculateFinals applies the sum of all active strategy modifiers to each
// base value, then clamps once. Sorting IDs makes floating-point accumulation
// deterministic and independent of Go map iteration order.
func (g *GameState) CalculateFinals() error {
	if g == nil {
		return errors.New("game state is nil")
	}
	strategyIDs := make([]int, 0, len(g.Strategies))
	for id := range g.Strategies {
		strategyIDs = append(strategyIDs, int(id))
	}
	sort.Ints(strategyIDs)

	shifts := make(map[AttributeId]float64)
	for _, rawID := range strategyIDs {
		strategy := g.Strategies[StrategyId(rawID)]
		if strategy.Status != StrategyUsingStatus {
			continue
		}
		if strategy.StrategyData == nil {
			return fmt.Errorf("strategy %d has nil data", rawID)
		}
		for _, change := range strategy.StrategyData.AttributeChanges {
			if math.IsNaN(change.AttributeShift) || math.IsInf(change.AttributeShift, 0) {
				return fmt.Errorf("invalid change for attribute %d in strategy %d", change.AttributeID, rawID)
			}
			if _, ok := g.Attributes[change.AttributeID]; !ok {
				return fmt.Errorf("%w: %d", ErrInvalidAttributeId, change.AttributeID)
			}
			shifts[change.AttributeID] += change.AttributeShift
			if math.IsNaN(shifts[change.AttributeID]) || math.IsInf(shifts[change.AttributeID], 0) {
				return fmt.Errorf("combined strategy changes overflow for attribute %d", change.AttributeID)
			}
		}
	}

	finalValues := make(map[AttributeId]float64, len(g.Attributes))
	for id, attribute := range g.Attributes {
		if attribute.AttributeData == nil {
			return fmt.Errorf("attribute %d has nil data", id)
		}
		finalValue := attribute.BaseValue + shifts[id]
		if math.IsNaN(finalValue) || math.IsInf(finalValue, 0) {
			return fmt.Errorf("invalid final value for attribute %d", id)
		}
		finalValues[id] = clampValue(finalValue, attribute.AttributeData.MinValue, attribute.AttributeData.MaxValue)
	}

	for id, finalValue := range finalValues {
		attribute := g.Attributes[id]
		attribute.FinalValue = finalValue
		g.Attributes[id] = attribute
	}
	return nil
}

// applyEventEffects applies a group of events as one simultaneous update.
// Eligibility is checked by the caller against the pre-event state. Changes
// to the same attribute and budget field are summed and clamped only once.
func (g *GameState) applyEventEffects(events []*GameEvent) error {
	orderedEvents := append([]*GameEvent(nil), events...)
	sort.Slice(orderedEvents, func(i, j int) bool {
		if orderedEvents[i] == nil {
			return orderedEvents[j] != nil
		}
		if orderedEvents[j] == nil {
			return false
		}
		return orderedEvents[i].ID < orderedEvents[j].ID
	})

	attributeShifts := make(map[AttributeId]float64)
	var moneyShift, qpShift int

	for _, event := range orderedEvents {
		if event == nil {
			return ErrInvalidEventId
		}
		for _, change := range event.AttributeChanges {
			if math.IsNaN(change.AttributeShift) || math.IsInf(change.AttributeShift, 0) {
				return fmt.Errorf("invalid change for attribute %d in event %d", change.AttributeID, event.ID)
			}
			attributeShifts[change.AttributeID] += change.AttributeShift
			if math.IsNaN(attributeShifts[change.AttributeID]) || math.IsInf(attributeShifts[change.AttributeID], 0) {
				return fmt.Errorf("combined event changes overflow for attribute %d", change.AttributeID)
			}
		}
		moneyShift = saturatingAddInt(moneyShift, event.BudgetChange.MoneyShift)
		qpShift = saturatingAddInt(qpShift, event.BudgetChange.QPShift)
	}

	next := *g
	next.Attributes = cloneAttributeStates(g.Attributes)
	for id, shift := range attributeShifts {
		attribute, ok := next.Attributes[id]
		if !ok || attribute.AttributeData == nil {
			return fmt.Errorf("%w: %d", ErrInvalidAttributeId, id)
		}
		baseValue := attribute.BaseValue + shift
		if math.IsNaN(baseValue) || math.IsInf(baseValue, 0) {
			return fmt.Errorf("invalid resulting base value for attribute %d", id)
		}
		attribute.BaseValue = clampValue(
			baseValue,
			attribute.AttributeData.MinValue,
			attribute.AttributeData.MaxValue,
		)
		next.Attributes[id] = attribute
	}

	budgetChange := BudgetChange{MoneyShift: moneyShift, QPShift: qpShift}
	budgetChange.Apply(&next)
	if err := next.CalculateFinals(); err != nil {
		return err
	}

	g.Attributes = next.Attributes
	g.Budget = next.Budget
	return nil
}
