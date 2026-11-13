package gameengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	gameFactoriesMu sync.RWMutex
	gameFactories   = make(map[string]func() Game)
)

// RegisterGame adds a named game factory to the instance registry.
func RegisterGame(id string, factory func() Game) {
	gameFactoriesMu.Lock()
	defer gameFactoriesMu.Unlock()
	gameFactories[id] = factory
}

// GetGameInstance creates a fresh state for a registered game ID.
func GetGameInstance(id string) (GameState, error) {
	gameFactoriesMu.RLock()
	factory, ok := gameFactories[id]
	gameFactoriesMu.RUnlock()
	if !ok {
		return GameState{}, fmt.Errorf("unknown game id: %s", id)
	}
	game := factory()
	state := game.CreateInstance()
	state.ID = id
	return state, nil
}

// LoadGames загружает все JSON-сценарии из папки scenarios и регистрирует их
// по имени файла без расширения.
func LoadGames() error {
	entries, err := os.ReadDir("scenarios")
	if err != nil {
		return fmt.Errorf("read scenarios directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}

		path := filepath.Join("scenarios", entry.Name())
		game, err := CreateGameJSON(path)
		if err != nil {
			return err
		}
		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		registeredGame := game
		RegisterGame(id, func() Game {
			return registeredGame
		})
	}
	return nil
}

// Передаёт значения настраиваемых атрибутов в текущий выбор цикла.
// Значения проверяются, но применяются к состоянию только в CalculateState.
func (g *GameState) PassAtributes(ids []int, values []float64) error {
	if len(ids) != len(values) {
		return ErrDimensionsNotMatch
	}
	changes := make([]AttributeChange, len(ids))
	for i := range ids {
		changes[i] = AttributeChange{AttributeID: AttributeId(ids[i]), AttributeShift: values[i]}
	}
	if err := g.ValidateAttributes(changes); err != nil {
		return err
	}
	g.PendingAttributes = changes
	return nil
}

// Правильное написание-синоним для внешнего кода.
func (g *GameState) PassAttributes(ids []int, values []float64) error {
	return g.PassAtributes(ids, values)
}

// Передаёт выбранные стратегии в текущий выбор цикла.
func (g *GameState) PassStrategies(ids []int) error {
	Strategies := make([]StrategyId, len(ids))
	for i, ID := range ids {
		Strategies[i] = StrategyId(ID)
	}
	if err := g.ValidateStrategies(Strategies); err != nil {
		return err
	}
	g.PendingStrategies = Strategies
	return nil
}

// Передаёт выбранные подсказки в текущий выбор цикла.
func (g *GameState) PassHints(ids []int) error {
	Hints := make([]HintId, len(ids))
	for i, ID := range ids {
		Hints[i] = HintId(ID)
	}
	if err := g.ValidateHints(Hints); err != nil {
		return err
	}
	g.PendingHints = Hints
	return nil
}

// Применяет выбор цикла, списывает его стоимость и пересчитывает итоговое поле.
func (g *GameState) CalculateState() error {
	moneyCost := CycleBaseCost + g.AttributeCost(g.PendingAttributes)
	for _, strategyID := range g.PendingStrategies {
		moneyCost += g.Strategies[strategyID].StrategyData.Cost
	}
	qpCost := 0
	for _, hintID := range g.PendingHints {
		qpCost += g.Hints[hintID].HintData.Cost
	}
	if moneyCost > g.Budget.Money {
		return ErrInsufficientBudget
	}
	if qpCost > g.Budget.QP {
		return ErrInsufficientQP
	}

	if err := g.UseAttributes(g.PendingAttributes); err != nil {
		return err
	}
	if err := g.UseStrategies(g.PendingStrategies); err != nil {
		return err
	}
	if err := g.UseHints(g.PendingHints); err != nil {
		return err
	}
	Cost := BudgetChange{MoneyShift: -moneyCost, QPShift: -qpCost}
	Cost.Apply(g)
	err := g.CalculateFinals()
	g.PendingAttributes = nil
	g.PendingStrategies = nil
	g.PendingHints = nil
	return err
}

// Возвращает прогнозы вероятностей доступных событий.
func (g *GameState) GetPredicts() ([]ProbResult, error) {
	return g.GetProbabilitiesForEvents()
}

// Срабатывает явно выбранный набор событий или запускает автоматический picker
// при передаче nil.
func (g *GameState) FireEvents(ids []int) error {
	var Events []ProbResult
	if ids == nil {
		probabilities, err := g.GetPredicts()
		if err != nil {
			return err
		}
		var pickErr error
		Events, pickErr = g.PickEvents(probabilities)
		if pickErr != nil {
			return pickErr
		}
	} else {
		Events = make([]ProbResult, 0, len(ids))
		for _, ID := range ids {
			event, ok := g.Events[EventId(ID)]
			if !ok || event.FinalStatus != EventAvailableStatus {
				return fmt.Errorf("%w: %d", ErrInvalidEventId, ID)
			}
			Events = append(Events, ProbResult{
				Attractor: &Attractor{EventState: &event},
			})
		}
	}

	for ID, event := range g.Events {
		if event.BaseStatus == EventFiredStatus {
			event.BaseStatus = EventExtinguishedStatus
			g.Events[ID] = event
		}
	}
	for _, probability := range Events {
		if err := probability.Attractor.EventState.Fire(g); err != nil {
			return err
		}
	}
	return nil
}
