package gameengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	gameFactoriesMu sync.RWMutex
	gameFactories   = make(map[string]func() Game)
)

// RegisterGame registers a scenario factory under id.
// Registering the same ID again replaces the previous factory.
// Empty IDs and nil factories are ignored.
func RegisterGame(id string, factory func() Game) {
	id = strings.TrimSpace(id)
	if id == "" || factory == nil {
		return
	}

	gameFactoriesMu.Lock()
	gameFactories[id] = factory
	gameFactoriesMu.Unlock()
}

// GetGameInstance creates a new, independent state for a registered game.
// The scenario definition is copied before CreateInstance is called so that
// mutable slices and nested scenario data are not shared between sessions.
func GetGameInstance(id string) (GameState, error) {
	gameFactoriesMu.RLock()
	factory, ok := gameFactories[id]
	gameFactoriesMu.RUnlock()
	if !ok || factory == nil {
		return GameState{}, fmt.Errorf("unknown game id: %s", id)
	}

	game := cloneGameDefinition(factory())
	state := game.CreateInstance()
	state.ID = id
	return state, nil
}

// GetAvailableStrategies returns strategies whose prerequisites are met.
func (g *GameState) GetAvailableStrategies() []StrategyState {
	if g == nil {
		return nil
	}

	result := make([]StrategyState, 0, len(g.Strategies))
	for _, strategy := range g.Strategies {
		if strategy.StrategyData != nil &&
			g.strategyAvailable(strategy.StrategyData) {
			result = append(result, strategy)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].StrategyData.ID < result[j].StrategyData.ID
	})

	return result
}

// GetAvailableHints returns unused hints whose prerequisites are met.
func (g *GameState) GetAvailableHints() []HintState {
	if g == nil {
		return nil
	}

	result := make([]HintState, 0, len(g.Hints))
	for _, hint := range g.Hints {
		if hint.Status != HintUsedStatus &&
			hint.HintData != nil &&
			g.hintAvailable(hint.HintData) {
			result = append(result, hint)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].HintData.ID < result[j].HintData.ID
	})

	return result
}

// LoadGames loads every JSON scenario from ./scenarios. The filename without
// its extension becomes the registered game ID.
func LoadGames() error {
	return LoadGamesFromDir("scenarios")
}

// LoadGamesFromDir loads scenarios from dir. All files are parsed before the
// registry is modified, so a malformed file cannot leave a partially loaded
// set of scenarios.
func LoadGamesFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read scenarios directory %q: %w", dir, err)
	}

	type registration struct {
		id   string
		game Game
	}
	registrations := make([]registration, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		game, err := CreateGameJSON(path)
		if err != nil {
			return fmt.Errorf("load scenario %q: %w", path, err)
		}

		id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("duplicate scenario ID %q in directory %q", id, dir)
		}
		seen[id] = struct{}{}
		registrations = append(registrations, registration{id: id, game: game})
	}

	gameFactoriesMu.Lock()
	for _, item := range registrations {
		registeredGame := item.game
		gameFactories[item.id] = func() Game { return registeredGame }
	}
	gameFactoriesMu.Unlock()
	return nil
}

// PassAttributes stores absolute values selected for configurable attributes.
// Values are applied only after CalculateState succeeds.
func (g *GameState) PassAttributes(ids []int, values []float64) error {
	if g == nil {
		return errors.New("game state is nil")
	}
	if len(ids) != len(values) {
		return ErrDimensionsNotMatch
	}

	changes := make([]AttributeChange, len(ids))
	for i, id := range ids {
		changes[i] = AttributeChange{
			AttributeID:    AttributeId(id),
			AttributeShift: values[i],
		}
	}
	if err := g.ValidateAttributes(changes); err != nil {
		return err
	}
	g.PendingAttributes = changes
	return nil
}

// PassStrategies stores the strategy set selected for the next cycle.
func (g *GameState) PassStrategies(ids []int) error {
	if g == nil {
		return errors.New("game state is nil")
	}

	strategies := make([]StrategyId, len(ids))
	for i, id := range ids {
		strategies[i] = StrategyId(id)
	}
	if err := g.ValidateStrategies(strategies); err != nil {
		return err
	}
	g.PendingStrategies = strategies
	return nil
}

// PassHints stores hints selected for the next cycle.
func (g *GameState) PassHints(ids []int) error {
	if g == nil {
		return errors.New("game state is nil")
	}

	hints := make([]HintId, len(ids))
	for i, id := range ids {
		hints[i] = HintId(id)
	}
	if err := g.ValidateHints(hints); err != nil {
		return err
	}
	g.PendingHints = hints
	return nil
}

// CalculateState applies all pending player choices as one transaction.
// If validation, resource checks, or recalculation fails, the original state
// and its pending selections remain unchanged.
func (g *GameState) CalculateState() error {
	if g == nil {
		return errors.New("game state is nil")
	}

	next := *g
	next.Attributes = cloneAttributeStates(g.Attributes)
	next.Strategies = cloneStrategyStates(g.Strategies)
	next.Hints = cloneHintStates(g.Hints)

	if err := next.UseAttributes(next.PendingAttributes); err != nil {
		return err
	}
	if err := next.UseStrategies(next.PendingStrategies); err != nil {
		return err
	}
	if err := next.UseHints(next.PendingHints); err != nil {
		return err
	}

	moneyCost := saturatingAddInt(CycleBaseCost, next.AttributeCost(next.PendingAttributes))
	for _, strategyID := range next.PendingStrategies {
		strategy, ok := next.Strategies[strategyID]
		if !ok || strategy.StrategyData == nil {
			return fmt.Errorf("%w: %d", ErrInvalidStrategyId, strategyID)
		}
		moneyCost = saturatingAddInt(moneyCost, strategy.StrategyData.Cost)
	}

	qpCost := 0
	for _, hintID := range next.PendingHints {
		hint, ok := next.Hints[hintID]
		if !ok || hint.HintData == nil {
			return fmt.Errorf("%w: %d", ErrInvalidHintId, hintID)
		}
		qpCost = saturatingAddInt(qpCost, hint.HintData.Cost)
	}

	if moneyCost > next.Budget.Money {
		return ErrInsufficientBudget
	}
	if qpCost > next.Budget.QP {
		return ErrInsufficientQP
	}

	cost := BudgetChange{
		MoneyShift: -moneyCost,
		QPShift:    -qpCost,
	}
	cost.Apply(&next)

	if err := next.CalculateFinals(); err != nil {
		return err
	}

	next.PendingAttributes = nil
	next.PendingStrategies = nil
	next.PendingHints = nil
	*g = next
	return nil
}

// GetPredicts returns the raw event selection weights for all available,
// neutral events. Values are intentionally not normalized: normalization is
// performed separately inside each branch by PickEventFromBranch.
func (g *GameState) GetPredicts() ([]ProbResult, error) {
	return g.GetProbabilitiesForEvents()
}

// FireEvents fires the explicitly selected events and automatically picks one
// event for every unselected branch. Passing nil selects automatically in all
// branches. Every selected event is checked against the same pre-event state;
// the event effects are then applied together and committed atomically.
func (g *GameState) FireEvents(ids []int) error {
	if g == nil || g.GameData == nil {
		return errors.New("game data is nil")
	}

	probabilities, err := g.GetPredicts()
	if err != nil {
		return err
	}

	selectedIDs := make([]EventId, 0, len(g.GameData.Branches))
	selectedBranches := make(map[Branch]struct{}, len(g.GameData.Branches))

	if ids == nil {
		picked, err := g.PickEvents(probabilities)
		if err != nil {
			return err
		}
		for _, probability := range picked {
			if probability.Event == nil || probability.Event.EventData == nil {
				return ErrInvalidEventId
			}
			id := probability.Event.EventData.ID
			branch := probability.Event.EventData.Branch
			selectedIDs = append(selectedIDs, id)
			selectedBranches[branch] = struct{}{}
		}
	} else {
		for _, rawID := range ids {
			id := EventId(rawID)
			event, ok := g.Events[id]
			if !ok || event.EventData == nil {
				return fmt.Errorf("%w: %d", ErrInvalidEventId, id)
			}
			if event.Status != EventNeutral || !g.eventCanOccur(event.EventData) {
				return fmt.Errorf("%w: %d", ErrEventNotAvailable, id)
			}

			branch := event.EventData.Branch
			if _, exists := selectedBranches[branch]; exists {
				return fmt.Errorf("more than one event selected for branch %q", branch)
			}
			selectedBranches[branch] = struct{}{}
			selectedIDs = append(selectedIDs, id)
		}

		for _, branch := range g.GameData.Branches {
			if _, alreadySelected := selectedBranches[branch]; alreadySelected {
				continue
			}
			probability, pickErr := g.PickEventFromBranch(probabilities, branch)
			if pickErr != nil {
				continue
			}
			if probability.Event == nil || probability.Event.EventData == nil {
				return ErrInvalidEventId
			}
			id := probability.Event.EventData.ID
			selectedIDs = append(selectedIDs, id)
			selectedBranches[branch] = struct{}{}
		}
	}

	selectedEvents := make([]*GameEvent, 0, len(selectedIDs))
	seenIDs := make(map[EventId]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		if _, duplicate := seenIDs[id]; duplicate {
			return fmt.Errorf("event %d selected more than once", id)
		}
		seenIDs[id] = struct{}{}

		event, ok := g.Events[id]
		if !ok || event.EventData == nil || event.Status != EventNeutral || !g.eventCanOccur(event.EventData) {
			return fmt.Errorf("%w: %d", ErrEventNotAvailable, id)
		}
		selectedEvents = append(selectedEvents, event.EventData)
	}

	// applyEventEffects itself calculates on a copy and commits only on success.
	if err := g.applyEventEffects(selectedEvents); err != nil {
		return err
	}

	// Update statuses only after all effects have been applied successfully.
	for id, event := range g.Events {
		if event.Status == EventFiredStatus {
			event.Status = EventExtinguishedStatus
			g.Events[id] = event
		}
	}
	for _, id := range selectedIDs {
		event := g.Events[id]
		event.Status = EventFiredStatus
		g.Events[id] = event
	}
	return nil
}

func cloneGameDefinition(source Game) Game {
	result := source
	result.BasicAttributes = append([]GameAttribute(nil), source.BasicAttributes...)
	result.ConfigurableAttributes = append([]ConfigurableAttribute(nil), source.ConfigurableAttributes...)
	result.Branches = append([]Branch(nil), source.Branches...)

	result.Events = make([]GameEvent, len(source.Events))
	for i, event := range source.Events {
		result.Events[i] = event
		result.Events[i].Weights = append(Vector(nil), event.Weights...)
		result.Events[i].Prerequisites = clonePrerequisite(event.Prerequisites)
		result.Events[i].Thresholds = append([]Threshold(nil), event.Thresholds...)
		result.Events[i].AttributeChanges = append([]AttributeChange(nil), event.AttributeChanges...)
	}

	result.Hints = make([]Hint, len(source.Hints))
	for i, hint := range source.Hints {
		result.Hints[i] = hint
		result.Hints[i].Prerequisites = clonePrerequisite(hint.Prerequisites)
	}

	result.Strategies = make([]Strategy, len(source.Strategies))
	for i, strategy := range source.Strategies {
		result.Strategies[i] = strategy
		result.Strategies[i].Prerequisites = clonePrerequisite(strategy.Prerequisites)
		result.Strategies[i].AttributeChanges = append([]AttributeChange(nil), strategy.AttributeChanges...)
	}
	return result
}

func clonePrerequisite(source Prerequisite) Prerequisite {
	return Prerequisite{
		Events:     append([]EventId(nil), source.Events...),
		Strategies: append([]StrategyId(nil), source.Strategies...),
	}
}
