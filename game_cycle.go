package main

import (
	"errors"
	"fmt"
	"math"
)

const cycleBaseCost = 5

var (
	ErrInvalidStrategyId        = errors.New("Invalid Strategy Id")
	ErrStrategyAccessNotAllowed = errors.New("Trying to access locked strategy")
	ErrInvalidAttributeId       = errors.New("Invalid Attribute Id")
	ErrInvalidEventId           = errors.New("Invalid Event Id")
	ErrInvalidHintId            = errors.New("Invalid Hint Id")
	ErrHintAccessNotAllowed     = errors.New("Trying to access locked hint")
	ErrInsufficientBudget       = errors.New("Insufficient Budget")
)

func (ac *AttributeChange) applyBase(g *GameState) error {
	if attr, e := g.attributes[ac.attributeId]; e {
		attr.baseValue = clamp(attr.baseValue+ac.attributeShift, attr._attributeData.minValue, attr._attributeData.maxValue)
		g.attributes[ac.attributeId] = attr
	} else {
		return ErrInvalidAttributeId
	}
	return nil
}

func (ac *AttributeChange) applyFinal(g *GameState) error {
	if attr, e := g.attributes[ac.attributeId]; e {
		attr.finalValue = clamp(attr.finalValue+ac.attributeShift, attr._attributeData.minValue, attr._attributeData.maxValue)
		g.attributes[ac.attributeId] = attr
	} else {
		return ErrInvalidAttributeId
	}
	return nil
}

func (c *EventParametersChange) applyBase(g *GameState) error {
	if state, e := g.events[c.eventId]; e {
		state.baseField.power.amplitude += c.shift.power.amplitude
		state.baseField.power.density += c.shift.power.density
		if len(state.baseField.weights) != len(c.shift.weights) {
			return ErrDimensionsNotMatch
		}
		for i := range c.shift.weights {
			state.baseField.weights[i] += c.shift.weights[i]
		}
		g.events[c.eventId] = state
	} else {
		return ErrInvalidEventId
	}
	return nil
}

func (c *EventParametersChange) applyFinal(g *GameState) error {
	if state, e := g.events[c.eventId]; e {
		state.finalField.power.amplitude += c.shift.power.amplitude
		state.finalField.power.density += c.shift.power.density
		if len(state.finalField.weights) != len(c.shift.weights) {
			return ErrDimensionsNotMatch
		}
		for i := range c.shift.weights {
			state.finalField.weights[i] += c.shift.weights[i]
		}
		g.events[c.eventId] = state
	} else {
		return ErrInvalidEventId
	}
	return nil
}

func (c *BudgetChange) apply(g *GameState) {
	money := g.budget.money + c.moneyShift
	money = min(g.budget._budgetData.maxMoney, max(0, money))
	g.budget = BudgetState{
		_budgetData: g.budget._budgetData,
		money:       money,
		qp:          max(0, g.budget.qp+c.qpShift),
	}
}

func (e *EventState) fire(g *GameState) (err error) {
	var er error
	for _, change := range e._eventData.attributeChanges {
		er = change.applyBase(g)
		if er != nil {
			err = fmt.Errorf("%w, %w", err, er)
		}
	}

	for _, change := range e._eventData.parameterChanges {
		er = change.applyBase(g)
		if er != nil {
			err = fmt.Errorf("%w, %w", err, er)
		}
	}

	er = g.unlockStrategies(e._eventData.strategiesUnlocked)
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}
	er = g.unlockHints(e._eventData.hintsUnlocked)
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}
	er = g.unlockEventsBase(e._eventData.eventsUnlocked)
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}

	e._eventData.budgetChange.apply(g)

	if state, ok := g.events[e._eventData.id]; ok {
		state.baseStatus = EventFiredStatus
		state.finalStatus = EventFiredStatus
		g.events[e._eventData.id] = state
	} else {
		err = fmt.Errorf("%w, %w: %d", err, ErrInvalidEventId, e._eventData.id)
	}

	return err
}

func (g *GameState) unlockStrategies(toUse []StrategyId) (err error) {
	for _, strat := range toUse {
		if s, e := g.strategies[strat]; e {
			if s.status == StrategyLockedStatus {
				s.status = StrategyUnlockedStatus
				g.strategies[strat] = s
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidStrategyId, strat)
			continue
		}

	}

	return err
}

func (g *GameState) useStrategies(toUse []StrategyId) (err error) {
	if err = g.validateStrategies(toUse); err != nil {
		return err
	}

	for k, strat := range g.strategies {
		if strat.status == StrategyUsingStatus {
			strat.status = StrategyUnlockedStatus
		}
		g.strategies[k] = strat
	}

	for _, strat := range toUse {
		s := g.strategies[strat]
		s.status = StrategyUsingStatus
		g.strategies[strat] = s
	}
	return nil
}

func (g *GameState) validateStrategies(toUse []StrategyId) (err error) {
	for _, id := range toUse {
		strat, ok := g.strategies[id]
		if !ok {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidStrategyId, id)
			continue
		}
		if strat.status == StrategyLockedStatus {
			err = fmt.Errorf("%w, %w: %d", err, ErrStrategyAccessNotAllowed, id)
		}
	}
	return err
}

func (g *GameState) unlockHints(toUse []HintId) (err error) {
	for _, hint := range toUse {
		if h, e := g.hints[hint]; e {
			if h.status == HintLockedStatus {
				h.status = HintUnlockedStatus
				g.hints[hint] = h
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidHintId, hint)
			continue
		}

	}

	return err
}

func (g *GameState) useHints(toUse []HintId) (err error) {
	if err = g.validateHints(toUse); err != nil {
		return err
	}

	for _, hint := range toUse {
		h := g.hints[hint]
		h.status = HintUsedStatus
		g.hints[hint] = h
	}
	return nil
}

func (g *GameState) validateHints(toUse []HintId) (err error) {
	for _, hint := range toUse {
		h, ok := g.hints[hint]
		if !ok {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidHintId, hint)
		} else if h.status == HintLockedStatus {
			err = fmt.Errorf("%w, %w: %d", err, ErrHintAccessNotAllowed, hint)
		}
	}
	return err
}

func (g *GameState) unlockEventsBase(toUse []EventId) (err error) {
	for _, event := range toUse {
		if ev, e := g.events[event]; e {
			if ev.baseStatus == EventLockedStatus {
				ev.baseStatus = EventAvailableStatus
				g.events[event] = ev
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidEventId, event)
			continue
		}

	}

	return err
}

func (g *GameState) unlockEventsFinal(toUse []EventId) (err error) {
	for _, event := range toUse {
		if ev, e := g.events[event]; e {
			if ev.finalStatus == EventLockedStatus {
				ev.finalStatus = EventAvailableStatus
				g.events[event] = ev
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidEventId, event)
			continue
		}

	}

	return err
}

func (g *GameState) useAttributes(toUse []AttributeChange) (err error) {
	if err = g.validateAttributes(toUse); err != nil {
		return err
	}

	for _, ac := range toUse {
		attr := g.attributes[ac.attributeId]
		attr.baseValue = ac.attributeShift
		g.attributes[ac.attributeId] = attr
	}
	return nil
}

func (g *GameState) validateAttributes(toUse []AttributeChange) (err error) {
	for _, ac := range toUse {
		attr, ex := g.attributes[ac.attributeId]
		if !ex {
			err = fmt.Errorf("%w: %d", ErrInvalidAttributeId, ac.attributeId)
		} else if math.IsNaN(ac.attributeShift) || math.IsInf(ac.attributeShift, 0) ||
			ac.attributeShift < attr._attributeData.minValue || ac.attributeShift > attr._attributeData.maxValue {
			err = fmt.Errorf("%w: value %.3f for attribute %d", err, ac.attributeShift, ac.attributeId)
		}
	}
	return err
}

func (g *GameState) attributeCost(toUse []AttributeChange) int {
	cost := 0
	for _, change := range toUse {
		_, ok := g.attributes[change.attributeId]
		if !ok {
			continue
		}
		for i := range g._gameData.configurableAttributes {
			configurable := &g._gameData.configurableAttributes[i]
			if configurable.id == change.attributeId {
				cost += int(math.Round(change.attributeShift * configurable.costPerUnit))
				break
			}
		}
	}
	return cost
}

func copyVector(source Vector) Vector {
	return append(Vector(nil), source...)
}

func (g *GameState) calcFinals() (err error) {
	for k, attr := range g.attributes {
		attr.finalValue = attr.baseValue
		g.attributes[k] = attr
	}

	for k, e := range g.events {
		e.finalField = Field{
			weights: copyVector(e.baseField.weights),
			power:   e.baseField.power,
		}
		e.finalStatus = e.baseStatus
		g.events[k] = e
	}

	for _, strat := range g.strategies {
		if strat.status == StrategyUsingStatus {
			for _, change := range strat._strategyData.attributeChanges {
				if applyErr := change.applyFinal(g); applyErr != nil {
					err = fmt.Errorf("%w, %w", err, applyErr)
				}
			}
			for _, change := range strat._strategyData.parameterChanges {
				if applyErr := change.applyFinal(g); applyErr != nil {
					err = fmt.Errorf("%w, %w", err, applyErr)
				}
			}

			if unlockErr := g.unlockEventsFinal(strat._strategyData.eventsUnlocked); unlockErr != nil {
				err = fmt.Errorf("%w, %w", err, unlockErr)
			}
		}
	}
	return err
}

func (g *GameState) pushCycle(configurableAttributes []AttributeChange, chosenStrategies []StrategyId, chosenHints []HintId, totalCost BudgetChange) (err error) {
	if err = g.validateAttributes(configurableAttributes); err != nil {
		return err
	}
	if err = g.validateStrategies(chosenStrategies); err != nil {
		return err
	}
	if err = g.validateHints(chosenHints); err != nil {
		return err
	}

	calculatedCost := cycleBaseCost + g.attributeCost(configurableAttributes)
	for _, strategyID := range chosenStrategies {
		calculatedCost += g.strategies[strategyID]._strategyData.cost
	}
	for _, hintID := range chosenHints {
		calculatedCost += g.hints[hintID]._hintData.cost
	}
	totalCost.moneyShift = -calculatedCost
	if totalCost.moneyShift > g.budget.money || totalCost.qpShift > g.budget.qp {
		err = ErrInsufficientBudget
		return err
	}

	if err = g.useAttributes(configurableAttributes); err != nil {
		return err
	}
	if err = g.useStrategies(chosenStrategies); err != nil {
		return err
	}
	if err = g.useHints(chosenHints); err != nil {
		return err
	}

	totalCost.apply(g)

	if err = g.calcFinals(); err != nil {
		return err
	}

	probs, er := g.getProbabilitiesForEvents()
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}

	events, er := g.pickEvents(probs)

	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}

	for k, event := range g.events {
		if event.baseStatus == EventFiredStatus {
			event.baseStatus = EventExtinguishedStatus
			g.events[k] = event
		}
	}

	for _, prob := range events {
		er = prob._attractor._eventState.fire(g)
		if er != nil {
			err = fmt.Errorf("%w, %w", err, er)
		}
	}
	return err
}
