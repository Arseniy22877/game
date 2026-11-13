package gameengine

import (
	"errors"
	"fmt"
	"math"
)

const CycleBaseCost = 5

var (
	ErrInvalidStrategyId        = errors.New("Invalid Strategy Id")
	ErrStrategyAccessNotAllowed = errors.New("Trying to access locked strategy")
	ErrInvalidAttributeId       = errors.New("Invalid Attribute Id")
	ErrInvalidEventId           = errors.New("Invalid Event Id")
	ErrInvalidHintId            = errors.New("Invalid Hint Id")
	ErrHintAccessNotAllowed     = errors.New("Trying to access locked hint")
	ErrInsufficientBudget       = errors.New("Insufficient Budget")
	ErrInsufficientQP           = errors.New("Insufficient QP")
)

// Применяет изменение атрибута к постоянной части состояния.
func (ac *AttributeChange) ApplyBase(g *GameState) error {
	if attr, e := g.Attributes[ac.AttributeID]; e {
		attr.BaseValue = clamp(attr.BaseValue+ac.AttributeShift, attr.AttributeData.MinValue, attr.AttributeData.MaxValue)
		g.Attributes[ac.AttributeID] = attr
	} else {
		return ErrInvalidAttributeId
	}
	return nil
}

// Применяет изменение атрибута к итоговой части текущего цикла.
func (ac *AttributeChange) ApplyFinal(g *GameState) error {
	if attr, e := g.Attributes[ac.AttributeID]; e {
		attr.FinalValue = clamp(attr.FinalValue+ac.AttributeShift, attr.AttributeData.MinValue, attr.AttributeData.MaxValue)
		g.Attributes[ac.AttributeID] = attr
	} else {
		return ErrInvalidAttributeId
	}
	return nil
}

// Применяет изменение параметров события к базовому полю.
func (c *EventParametersChange) ApplyBase(g *GameState) error {
	if state, e := g.Events[c.EventID]; e {
		state.BaseField.Power.Amplitude += c.Shift.Power.Amplitude
		state.BaseField.Power.Density += c.Shift.Power.Density
		if len(state.BaseField.Weights) != len(c.Shift.Weights) {
			return ErrDimensionsNotMatch
		}
		for i := range c.Shift.Weights {
			state.BaseField.Weights[i] += c.Shift.Weights[i]
		}
		g.Events[c.EventID] = state
	} else {
		return ErrInvalidEventId
	}
	return nil
}

// Применяет временное изменение параметров события к итоговому полю.
func (c *EventParametersChange) ApplyFinal(g *GameState) error {
	if state, e := g.Events[c.EventID]; e {
		state.FinalField.Power.Amplitude += c.Shift.Power.Amplitude
		state.FinalField.Power.Density += c.Shift.Power.Density
		if len(state.FinalField.Weights) != len(c.Shift.Weights) {
			return ErrDimensionsNotMatch
		}
		for i := range c.Shift.Weights {
			state.FinalField.Weights[i] += c.Shift.Weights[i]
		}
		g.Events[c.EventID] = state
	} else {
		return ErrInvalidEventId
	}
	return nil
}

// Изменяет бюджет с ограничением денег диапазоном от нуля до максимума.
func (c *BudgetChange) Apply(g *GameState) {
	Money := g.Budget.Money + c.MoneyShift
	Money = min(g.Budget.BudgetData.MaxMoney, max(0, Money))
	g.Budget = BudgetState{
		BudgetData: g.Budget.BudgetData,
		Money:      Money,
		QP:         max(0, g.Budget.QP+c.QPShift),
	}
}

// Выполняет последствия события и переводит его в состояние Fired.
func (e *EventState) Fire(g *GameState) (err error) {
	var er error
	for _, change := range e.EventData.AttributeChanges {
		er = change.ApplyBase(g)
		if er != nil {
			err = fmt.Errorf("%w, %w", err, er)
		}
	}

	for _, change := range e.EventData.ParameterChanges {
		er = change.ApplyBase(g)
		if er != nil {
			err = fmt.Errorf("%w, %w", err, er)
		}
	}

	er = g.UnlockStrategies(e.EventData.StrategiesUnlocked)
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}
	er = g.UnlockHints(e.EventData.HintsUnlocked)
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}
	er = g.UnlockEventsBase(e.EventData.EventsUnlocked)
	if er != nil {
		err = fmt.Errorf("%w, %w", err, er)
	}

	e.EventData.BudgetChange.Apply(g)

	if state, ok := g.Events[e.EventData.ID]; ok {
		state.BaseStatus = EventFiredStatus
		state.FinalStatus = EventFiredStatus
		g.Events[e.EventData.ID] = state
	} else {
		err = fmt.Errorf("%w, %w: %d", err, ErrInvalidEventId, e.EventData.ID)
	}

	return err
}

// Открывает стратегии, перечисленные в последствиях события.
func (g *GameState) UnlockStrategies(toUse []StrategyId) (err error) {
	for _, strat := range toUse {
		if s, e := g.Strategies[strat]; e {
			if s.Status == StrategyLockedStatus {
				s.Status = StrategyUnlockedStatus
				g.Strategies[strat] = s
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidStrategyId, strat)
			continue
		}

	}

	return err
}

// Заменяет активные стратегии выбранными стратегиями текущего цикла.
func (g *GameState) UseStrategies(toUse []StrategyId) (err error) {
	if err = g.ValidateStrategies(toUse); err != nil {
		return err
	}

	for k, strat := range g.Strategies {
		if strat.Status == StrategyUsingStatus {
			strat.Status = StrategyUnlockedStatus
		}
		g.Strategies[k] = strat
	}

	for _, strat := range toUse {
		s := g.Strategies[strat]
		s.Status = StrategyUsingStatus
		g.Strategies[strat] = s
	}
	return nil
}

// Проверяет существование и доступность стратегий.
func (g *GameState) ValidateStrategies(toUse []StrategyId) (err error) {
	for _, ID := range toUse {
		strat, ok := g.Strategies[ID]
		if !ok {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidStrategyId, ID)
			continue
		}
		if strat.Status == StrategyLockedStatus {
			err = fmt.Errorf("%w, %w: %d", err, ErrStrategyAccessNotAllowed, ID)
		}
	}
	return err
}

// Открывает подсказки, перечисленные в последствиях события.
func (g *GameState) UnlockHints(toUse []HintId) (err error) {
	for _, hint := range toUse {
		if h, e := g.Hints[hint]; e {
			if h.Status == HintLockedStatus {
				h.Status = HintUnlockedStatus
				g.Hints[hint] = h
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidHintId, hint)
			continue
		}

	}

	return err
}

// Помечает выбранные подсказки как использованные.
func (g *GameState) UseHints(toUse []HintId) (err error) {
	if err = g.ValidateHints(toUse); err != nil {
		return err
	}

	for _, hint := range toUse {
		h := g.Hints[hint]
		h.Status = HintUsedStatus
		g.Hints[hint] = h
	}
	return nil
}

// Проверяет существование и доступность подсказок.
func (g *GameState) ValidateHints(toUse []HintId) (err error) {
	for _, hint := range toUse {
		h, ok := g.Hints[hint]
		if !ok {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidHintId, hint)
		} else if h.Status == HintLockedStatus {
			err = fmt.Errorf("%w, %w: %d", err, ErrHintAccessNotAllowed, hint)
		}
	}
	return err
}

// Открывает события в базовой ветке.
func (g *GameState) UnlockEventsBase(toUse []EventId) (err error) {
	for _, event := range toUse {
		if ev, e := g.Events[event]; e {
			if ev.BaseStatus == EventLockedStatus {
				ev.BaseStatus = EventAvailableStatus
				g.Events[event] = ev
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidEventId, event)
			continue
		}

	}

	return err
}

// Открывает события только в итоговом поле активной стратегии.
func (g *GameState) UnlockEventsFinal(toUse []EventId) (err error) {
	for _, event := range toUse {
		if ev, e := g.Events[event]; e {
			if ev.FinalStatus == EventLockedStatus {
				ev.FinalStatus = EventAvailableStatus
				g.Events[event] = ev
			}
		} else {
			err = fmt.Errorf("%w, %w: %d", err, ErrInvalidEventId, event)
			continue
		}

	}

	return err
}

// Устанавливает выбранные игроком базовые значения атрибутов.
func (g *GameState) UseAttributes(toUse []AttributeChange) (err error) {
	if err = g.ValidateAttributes(toUse); err != nil {
		return err
	}

	for _, ac := range toUse {
		attr := g.Attributes[ac.AttributeID]
		attr.BaseValue = ac.AttributeShift
		g.Attributes[ac.AttributeID] = attr
	}
	return nil
}

// Проверяет значения атрибутов и их идентификаторы.
func (g *GameState) ValidateAttributes(toUse []AttributeChange) (err error) {
	for _, ac := range toUse {
		attr, ex := g.Attributes[ac.AttributeID]
		if !ex {
			err = fmt.Errorf("%w: %d", ErrInvalidAttributeId, ac.AttributeID)
		} else if math.IsNaN(ac.AttributeShift) || math.IsInf(ac.AttributeShift, 0) ||
			ac.AttributeShift < attr.AttributeData.MinValue || ac.AttributeShift > attr.AttributeData.MaxValue {
			err = fmt.Errorf("%w: value %.3f for attribute %d", err, ac.AttributeShift, ac.AttributeID)
		}
	}
	return err
}

// Считает стоимость выбранных настраиваемых атрибутов.
func (g *GameState) AttributeCost(toUse []AttributeChange) int {
	Cost := 0
	for _, change := range toUse {
		_, ok := g.Attributes[change.AttributeID]
		if !ok {
			continue
		}
		for i := range g.GameData.ConfigurableAttributes {
			configurable := &g.GameData.ConfigurableAttributes[i]
			if configurable.ID == change.AttributeID {
				Cost += int(math.Round(change.AttributeShift * configurable.CostPerUnit))
				break
			}
		}
	}
	return Cost
}

// Создаёт независимую копию вектора.
func copyVector(source Vector) Vector {
	return append(Vector(nil), source...)
}

// Строит итоговое состояние: базовые значения плюс активные стратегии.
func (g *GameState) CalculateFinals() (err error) {
	for k, attr := range g.Attributes {
		attr.FinalValue = attr.BaseValue
		g.Attributes[k] = attr
	}

	for k, e := range g.Events {
		e.FinalField = Field{
			Weights: copyVector(e.BaseField.Weights),
			Power:   e.BaseField.Power,
		}
		e.FinalStatus = e.BaseStatus
		g.Events[k] = e
	}

	for _, strat := range g.Strategies {
		if strat.Status == StrategyUsingStatus {
			for _, change := range strat.StrategyData.AttributeChanges {
				if applyErr := change.ApplyFinal(g); applyErr != nil {
					err = fmt.Errorf("%w, %w", err, applyErr)
				}
			}
			for _, change := range strat.StrategyData.ParameterChanges {
				if applyErr := change.ApplyFinal(g); applyErr != nil {
					err = fmt.Errorf("%w, %w", err, applyErr)
				}
			}

			if unlockErr := g.UnlockEventsFinal(strat.StrategyData.EventsUnlocked); unlockErr != nil {
				err = fmt.Errorf("%w, %w", err, unlockErr)
			}
		}
	}
	return err
}
