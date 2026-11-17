package gameengine

import (
	"errors"
	"fmt"
	"math"
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

func (c *BudgetChange) Apply(g *GameState) {
	money := min(g.Budget.BudgetData.MaxMoney, max(0, g.Budget.Money+c.MoneyShift))
	g.Budget = BudgetState{
		BudgetData: g.Budget.BudgetData,
		Money:      money,
		QP:         max(0, g.Budget.QP+c.QPShift),
	}
}

func (e *EventState) Fire(g *GameState) error {
	if e == nil || e.EventData == nil {
		return ErrInvalidEventId
	}
	if !g.eventCanOccur(e.EventData) || e.EventData.Iteration != g.Iteration || e.Status != EventNeutral {
		return ErrEventNotAvailable
	}
	for _, change := range e.EventData.AttributeChanges {
		if err := change.ApplyBase(g); err != nil {
			return err
		}
	}
	e.EventData.BudgetChange.Apply(g)
	e.Status = EventFiredStatus
	eventState, ok := g.Events[e.EventData.ID]
	if ok {
		eventState.Status = EventFiredStatus
		g.Events[e.EventData.ID] = eventState
	}
	return nil
}

func (ac *AttributeChange) ApplyBase(g *GameState) error {
	attribute, ok := g.Attributes[ac.AttributeID]
	if !ok {
		return fmt.Errorf("%w: %d", ErrInvalidAttributeId, ac.AttributeID)
	}
	attribute.BaseValue = clamp(attribute.BaseValue+ac.AttributeShift, attribute.AttributeData.MinValue, attribute.AttributeData.MaxValue)
	attribute.FinalValue = attribute.BaseValue
	g.Attributes[ac.AttributeID] = attribute
	return nil
}

func (ac *AttributeChange) ApplyFinal(g *GameState) error {
	attribute, ok := g.Attributes[ac.AttributeID]
	if !ok {
		return fmt.Errorf("%w: %d", ErrInvalidAttributeId, ac.AttributeID)
	}
	attribute.FinalValue = clamp(attribute.FinalValue+ac.AttributeShift, attribute.AttributeData.MinValue, attribute.AttributeData.MaxValue)
	g.Attributes[ac.AttributeID] = attribute
	return nil
}

func (g *GameState) ValidateAttributes(changes []AttributeChange) error {
	var err error
	for _, change := range changes {
		attribute, ok := g.Attributes[change.AttributeID]
		if !ok {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrInvalidAttributeId, change.AttributeID))
			continue
		}
		if math.IsNaN(change.AttributeShift) || math.IsInf(change.AttributeShift, 0) ||
			change.AttributeShift < attribute.AttributeData.MinValue ||
			change.AttributeShift > attribute.AttributeData.MaxValue {
			err = errors.Join(err, fmt.Errorf("invalid value %.3f for attribute %d", change.AttributeShift, change.AttributeID))
		}
	}
	return err
}

func (g *GameState) AttributeCost(changes []AttributeChange) int {
	cost := 0
	for _, change := range changes {
		for i := range g.GameData.ConfigurableAttributes {
			attribute := &g.GameData.ConfigurableAttributes[i]
			if attribute.ID == change.AttributeID {
				cost += int(math.Round(change.AttributeShift * attribute.CostPerUnit))
			}
		}
	}
	return cost
}

func (g *GameState) eventCanOccur(event *GameEvent) bool {
	if event == nil || (event.Iteration != g.Iteration && event.Iteration != 0) {
		return false
	}
	if !g.prerequisitesMet(event.Prerequisites) || !g.thresholdsMet(event.Thresholds) {
		return false
	}
	return true
}

func (g *GameState) prerequisitesMet(prerequisites Prerequisite) bool {
	hasStrategie := false
	hasEvent := false
	if len(prerequisites.Events) == 0 {
		hasEvent = true
	}
	for _, eventID := range prerequisites.Events {
		event, ok := g.Events[eventID]
		if ok && (event.Status == EventFiredStatus || event.Status == EventExtinguishedStatus) {
			hasEvent = true
		}
	}
	if len(prerequisites.Strategies) == 0 {
		hasStrategie = true
	}
	for _, strategyID := range prerequisites.Strategies {
		strategy, ok := g.Strategies[strategyID]
		if ok && strategy.Status == StrategyUsingStatus {
			hasStrategie = true
		}
	}
	return (hasStrategie && hasEvent)
}

func (g *GameState) thresholdsMet(thresholds []Threshold) bool {
	for _, threshold := range thresholds {
		attribute, ok := g.Attributes[threshold.AttributeID]
		if !ok {
			return false
		}
		if attribute.FinalValue < threshold.Min || attribute.FinalValue > threshold.Max {
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
	var err error
	for _, id := range ids {
		strategy, ok := g.Strategies[id]
		if !ok {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrInvalidStrategyId, id))
			continue
		}
		if !g.strategyAvailable(strategy.StrategyData) {
			err = errors.Join(err, fmt.Errorf("%w: %d", ErrStrategyAccessNotAllowed, id))
		}
	}
	return err
}

func (g *GameState) ValidateHints(ids []HintId) error {
	var err error
	for _, id := range ids {
		hint, ok := g.Hints[id]
		if !ok {
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
	for _, change := range changes {
		attribute := g.Attributes[change.AttributeID]
		attribute.BaseValue = change.AttributeShift
		attribute.FinalValue = change.AttributeShift
		g.Attributes[change.AttributeID] = attribute
	}
	return nil
}

func (g *GameState) UseStrategies(ids []StrategyId) error {
	if err := g.ValidateStrategies(ids); err != nil {
		return err
	}
	for id, strategy := range g.Strategies {
		strategy.Status = StrategyNeutralStatus
		g.Strategies[id] = strategy
	}
	for _, id := range ids {
		strategy := g.Strategies[id]
		strategy.Status = StrategyUsingStatus
		g.Strategies[id] = strategy
	}
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

func (g *GameState) CalculateFinals() error {
	for id, attribute := range g.Attributes {
		attribute.FinalValue = attribute.BaseValue
		g.Attributes[id] = attribute
	}
	for _, strategy := range g.Strategies {
		if strategy.Status != StrategyUsingStatus {
			continue
		}
		for _, change := range strategy.StrategyData.AttributeChanges {
			if err := change.ApplyFinal(g); err != nil {
				return err
			}
		}
	}
	return nil
}
