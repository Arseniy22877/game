package gameengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// NewEmptyGame returns an editable game with all collections initialized.
func NewEmptyGame() Game {
	return Game{
		BasicAttributes:        []GameAttribute{},
		ConfigurableAttributes: []ConfigurableAttribute{},
		Branches:               []Branch{},
		Events:                 []GameEvent{},
		Hints:                  []Hint{},
		Strategies:             []Strategy{},
	}
}

// CreateGameJSON reads, parses, migrates and validates a game JSON file.
// Empty or whitespace-only files are treated as an empty game.
func CreateGameJSON(path string) (Game, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Game{}, fmt.Errorf("read game JSON %q: %w", path, err)
	}

	game, err := ParseGameJSON(data)
	if err != nil {
		return Game{}, fmt.Errorf("parse game JSON %q: %w", path, err)
	}
	return game, nil
}

// ParseGameJSON parses, migrates and validates a game definition.
// JSON must contain one object. Empty input represents an empty game.
func ParseGameJSON(data []byte) (Game, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return NewEmptyGame(), nil
	}
	if trimmed[0] != '{' {
		return Game{}, fmt.Errorf("game JSON must contain an object")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	var game Game
	if err := decoder.Decode(&game); err != nil {
		return Game{}, fmt.Errorf("decode game: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Game{}, err
	}

	if err := migrateLegacyThresholds(trimmed, &game); err != nil {
		return Game{}, fmt.Errorf("migrate legacy thresholds: %w", err)
	}
	normalizeGame(&game)
	if err := ValidateGame(&game); err != nil {
		return Game{}, fmt.Errorf("validate game: %w", err)
	}
	return game, nil
}

// PrettyGameJSON serializes a game definition as indented JSON.
func PrettyGameJSON(game Game) ([]byte, error) {
	return json.MarshalIndent(game, "", "  ")
}

// SaveGameJSON validates and writes a game definition as formatted JSON.
// It normalizes a deep copy and does not mutate the caller's game.
func SaveGameJSON(path string, game Game) error {
	game = cloneGameDefinition(game)
	normalizeGame(&game)
	if err := ValidateGame(&game); err != nil {
		return fmt.Errorf("validate game: %w", err)
	}

	data, err := PrettyGameJSON(game)
	if err != nil {
		return fmt.Errorf("encode game JSON: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create game directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write game JSON %q: %w", path, err)
	}
	return nil
}

// ValidateGame checks IDs, references, dimensions and numeric values used by
// a game definition. Event weights must follow the order
// BasicAttributes, then ConfigurableAttributes.
func ValidateGame(game *Game) error {
	if game == nil {
		return fmt.Errorf("game is nil")
	}

	attributeIDs := make(map[AttributeId]struct{}, len(game.BasicAttributes)+len(game.ConfigurableAttributes))
	for i, attribute := range game.BasicAttributes {
		if err := validateGameAttribute(attribute, fmt.Sprintf("basic attribute %d", i)); err != nil {
			return err
		}
		if _, exists := attributeIDs[attribute.ID]; exists {
			return fmt.Errorf("duplicate attribute ID %d", attribute.ID)
		}
		attributeIDs[attribute.ID] = struct{}{}
	}
	for i, configurable := range game.ConfigurableAttributes {
		attribute := configurable.GameAttribute
		owner := fmt.Sprintf("configurable attribute %d", i)
		if err := validateGameAttribute(attribute, owner); err != nil {
			return err
		}
		if math.IsNaN(configurable.CostPerUnit) || math.IsInf(configurable.CostPerUnit, 0) || configurable.CostPerUnit < 0 {
			return fmt.Errorf("%s has invalid CostPerUnit %.6g", owner, configurable.CostPerUnit)
		}
		if _, exists := attributeIDs[attribute.ID]; exists {
			return fmt.Errorf("duplicate attribute ID %d", attribute.ID)
		}
		attributeIDs[attribute.ID] = struct{}{}
	}

	if game.StartingBudget.InitialMoney < 0 || game.StartingBudget.MaxMoney < 0 || game.StartingBudget.InitialQP < 0 {
		return fmt.Errorf("starting budget values cannot be negative")
	}
	if game.StartingBudget.InitialMoney > game.StartingBudget.MaxMoney {
		return fmt.Errorf("initial money %d exceeds maximum money %d", game.StartingBudget.InitialMoney, game.StartingBudget.MaxMoney)
	}
	if !isFinite(game.MinProbabilitySum) || game.MinProbabilitySum < 0 {
		return fmt.Errorf("MinProbabilitySum must be finite and non-negative")
	}

	branchIDs := make(map[Branch]struct{}, len(game.Branches))
	for _, branch := range game.Branches {
		if strings.TrimSpace(string(branch)) == "" {
			return fmt.Errorf("branch name cannot be empty")
		}
		if _, exists := branchIDs[branch]; exists {
			return fmt.Errorf("duplicate branch %q", branch)
		}
		branchIDs[branch] = struct{}{}
	}

	eventIDs := make(map[EventId]struct{}, len(game.Events))
	for _, event := range game.Events {
		if _, exists := eventIDs[event.ID]; exists {
			return fmt.Errorf("duplicate event ID %d", event.ID)
		}
		eventIDs[event.ID] = struct{}{}
	}
	strategyIDs := make(map[StrategyId]struct{}, len(game.Strategies))
	for _, strategy := range game.Strategies {
		if _, exists := strategyIDs[strategy.ID]; exists {
			return fmt.Errorf("duplicate strategy ID %d", strategy.ID)
		}
		strategyIDs[strategy.ID] = struct{}{}
	}
	hintIDs := make(map[HintId]struct{}, len(game.Hints))
	for _, hint := range game.Hints {
		if _, exists := hintIDs[hint.ID]; exists {
			return fmt.Errorf("duplicate hint ID %d", hint.ID)
		}
		hintIDs[hint.ID] = struct{}{}
	}

	if len(eventIDs) > 0 {
		if _, exists := eventIDs[game.OpeningEvent]; !exists {
			return fmt.Errorf("opening event ID %d does not exist", game.OpeningEvent)
		}
	}

	attributeCount := len(attributeIDs)
	for _, event := range game.Events {
		owner := fmt.Sprintf("event %d", event.ID)
		if _, exists := branchIDs[event.Branch]; !exists {
			return fmt.Errorf("%s uses unknown branch %q", owner, event.Branch)
		}
		if event.Iteration < 0 {
			return fmt.Errorf("%s has negative Iteration", owner)
		}
		if !isFinite(event.BasicPower) {
			return fmt.Errorf("%s has invalid BasicPower", owner)
		}
		if len(event.Weights) != attributeCount {
			return fmt.Errorf(
				"%s has %d weights, expected %d (basic attributes followed by configurable attributes)",
				owner, len(event.Weights), attributeCount,
			)
		}
		for weightIndex, weight := range event.Weights {
			if !isFinite(weight) {
				return fmt.Errorf("%s weight %d is not finite", owner, weightIndex)
			}
		}
		if err := validatePrerequisites(event.Prerequisites, eventIDs, strategyIDs, owner); err != nil {
			return err
		}
		for thresholdIndex, threshold := range event.Thresholds {
			if _, exists := attributeIDs[threshold.AttributeID]; !exists {
				return fmt.Errorf("%s threshold %d uses unknown attribute ID %d", owner, thresholdIndex, threshold.AttributeID)
			}
			if !isFinite(threshold.Min) || !isFinite(threshold.Max) || threshold.Min > threshold.Max {
				return fmt.Errorf("%s threshold %d has invalid range [%.6g, %.6g]", owner, thresholdIndex, threshold.Min, threshold.Max)
			}
		}
		for _, change := range event.AttributeChanges {
			if _, exists := attributeIDs[change.AttributeID]; !exists {
				return fmt.Errorf("%s uses unknown attribute ID %d", owner, change.AttributeID)
			}
			if !isFinite(change.AttributeShift) {
				return fmt.Errorf("%s has a non-finite change for attribute %d", owner, change.AttributeID)
			}
		}
	}

	for _, strategy := range game.Strategies {
		owner := fmt.Sprintf("strategy %d", strategy.ID)
		if strategy.Cost < 0 {
			return fmt.Errorf("%s has negative cost", owner)
		}
		if err := validatePrerequisites(strategy.Prerequisites, eventIDs, strategyIDs, owner); err != nil {
			return err
		}
		for _, change := range strategy.AttributeChanges {
			if _, exists := attributeIDs[change.AttributeID]; !exists {
				return fmt.Errorf("%s uses unknown attribute ID %d", owner, change.AttributeID)
			}
			if !isFinite(change.AttributeShift) {
				return fmt.Errorf("%s has a non-finite change for attribute %d", owner, change.AttributeID)
			}
		}
	}

	for _, hint := range game.Hints {
		owner := fmt.Sprintf("hint %d", hint.ID)
		if hint.Cost < 0 {
			return fmt.Errorf("%s has negative cost", owner)
		}
		if err := validatePrerequisites(hint.Prerequisites, eventIDs, strategyIDs, owner); err != nil {
			return err
		}
	}

	return nil
}

func validateGameAttribute(attribute GameAttribute, owner string) error {
	if !isFinite(attribute.MinValue) || !isFinite(attribute.MaxValue) || !isFinite(attribute.DefaultValue) {
		return fmt.Errorf("%s has non-finite values", owner)
	}
	if attribute.MinValue > attribute.MaxValue {
		return fmt.Errorf("%s has MinValue greater than MaxValue", owner)
	}
	if attribute.DefaultValue < attribute.MinValue || attribute.DefaultValue > attribute.MaxValue {
		return fmt.Errorf("%s DefaultValue is outside [MinValue, MaxValue]", owner)
	}
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// migrateLegacyThresholds converts the old {AttributeID, Value, Higher}
// threshold representation into an inclusive [Min, Max] range.
func migrateLegacyThresholds(data []byte, game *Game) error {
	var raw struct {
		Events []struct {
			Thresholds []map[string]json.RawMessage `json:"Thresholds"`
		} `json:"Events"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	limits := make(map[AttributeId]struct{ min, max float64 }, len(game.BasicAttributes)+len(game.ConfigurableAttributes))
	for _, attribute := range game.BasicAttributes {
		limits[attribute.ID] = struct{ min, max float64 }{attribute.MinValue, attribute.MaxValue}
	}
	for _, attribute := range game.ConfigurableAttributes {
		attribute := attribute.GameAttribute
		limits[attribute.ID] = struct{ min, max float64 }{attribute.MinValue, attribute.MaxValue}
	}

	for eventIndex := range game.Events {
		if eventIndex >= len(raw.Events) {
			break
		}
		for thresholdIndex := range game.Events[eventIndex].Thresholds {
			if thresholdIndex >= len(raw.Events[eventIndex].Thresholds) {
				break
			}

			fields := raw.Events[eventIndex].Thresholds[thresholdIndex]
			if hasJSONField(fields, "Min") || hasJSONField(fields, "Max") || !hasJSONField(fields, "Higher") {
				continue
			}

			var higher bool
			if err := unmarshalJSONField(fields, "Higher", &higher); err != nil {
				return fmt.Errorf("event %d threshold %d Higher: %w", game.Events[eventIndex].ID, thresholdIndex, err)
			}
			threshold := &game.Events[eventIndex].Thresholds[thresholdIndex]
			attributeLimit, found := limits[threshold.AttributeID]
			if !found {
				return fmt.Errorf("event %d threshold %d references unknown attribute %d", game.Events[eventIndex].ID, thresholdIndex, threshold.AttributeID)
			}
			if higher {
				threshold.Min = threshold.Value
				threshold.Max = attributeLimit.max
			} else {
				threshold.Min = attributeLimit.min
				threshold.Max = threshold.Value
			}
		}
	}
	return nil
}

func hasJSONField(fields map[string]json.RawMessage, name string) bool {
	for key := range fields {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func unmarshalJSONField(fields map[string]json.RawMessage, name string, target any) error {
	for key, value := range fields {
		if strings.EqualFold(key, name) {
			return json.Unmarshal(value, target)
		}
	}
	return fmt.Errorf("field %q is missing", name)
}

func validatePrerequisites(requirements Prerequisite, events map[EventId]struct{}, strategies map[StrategyId]struct{}, owner string) error {
	for _, id := range requirements.Events {
		if _, exists := events[id]; !exists {
			return fmt.Errorf("%s has prerequisite for unknown event ID %d", owner, id)
		}
	}
	for _, id := range requirements.Strategies {
		if _, exists := strategies[id]; !exists {
			return fmt.Errorf("%s has prerequisite for unknown strategy ID %d", owner, id)
		}
	}
	return nil
}

func normalizeGame(game *Game) {
	if game.BasicAttributes == nil {
		game.BasicAttributes = []GameAttribute{}
	}
	if game.ConfigurableAttributes == nil {
		game.ConfigurableAttributes = []ConfigurableAttribute{}
	}
	if game.Branches == nil {
		game.Branches = []Branch{}
	}
	if game.Events == nil {
		game.Events = []GameEvent{}
	}
	if game.Hints == nil {
		game.Hints = []Hint{}
	}
	if game.Strategies == nil {
		game.Strategies = []Strategy{}
	}

	for i := range game.Events {
		event := &game.Events[i]
		if event.Weights == nil {
			event.Weights = Vector{}
		}
		if event.Thresholds == nil {
			event.Thresholds = []Threshold{}
		}
		if event.AttributeChanges == nil {
			event.AttributeChanges = []AttributeChange{}
		}
		normalizePrerequisites(&event.Prerequisites)
	}
	for i := range game.Strategies {
		strategy := &game.Strategies[i]
		if strategy.AttributeChanges == nil {
			strategy.AttributeChanges = []AttributeChange{}
		}
		normalizePrerequisites(&strategy.Prerequisites)
	}
	for i := range game.Hints {
		normalizePrerequisites(&game.Hints[i].Prerequisites)
	}
}

func normalizePrerequisites(requirements *Prerequisite) {
	if requirements.Events == nil {
		requirements.Events = []EventId{}
	}
	if requirements.Strategies == nil {
		requirements.Strategies = []StrategyId{}
	}
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return fmt.Errorf("invalid trailing JSON data: %w", err)
	}
	return nil
}

// CreateInstance constructs a fresh runtime state from this scenario.
// The state references the scenario's data slices; callers that need isolated
// game sessions should clone the Game first (GetGameInstance does this).
func (game *Game) CreateInstance() GameState {
	state := GameState{
		GameData:   game,
		Attributes: make(map[AttributeId]AttributeState),
		Events:     make(map[EventId]EventState),
		Hints:      make(map[HintId]HintState),
		Strategies: make(map[StrategyId]StrategyState),
	}
	if game == nil {
		return state
	}

	state.Budget = BudgetState{
		BudgetData: &game.StartingBudget,
		Money:      game.StartingBudget.InitialMoney,
		QP:         game.StartingBudget.InitialQP,
	}

	for i := range game.BasicAttributes {
		attribute := &game.BasicAttributes[i]
		state.Attributes[attribute.ID] = AttributeState{
			AttributeData: attribute,
			BaseValue:     attribute.DefaultValue,
			FinalValue:    attribute.DefaultValue,
		}
	}
	for i := range game.ConfigurableAttributes {
		attribute := &game.ConfigurableAttributes[i].GameAttribute
		state.Attributes[attribute.ID] = AttributeState{
			AttributeData: attribute,
			BaseValue:     attribute.DefaultValue,
			FinalValue:    attribute.DefaultValue,
		}
	}
	for i := range game.Events {
		event := &game.Events[i]
		state.Events[event.ID] = EventState{EventData: event, Status: EventNeutral}
	}
	for i := range game.Hints {
		hint := &game.Hints[i]
		state.Hints[hint.ID] = HintState{HintData: hint, Status: HintNeutralStatus}
	}
	for i := range game.Strategies {
		strategy := &game.Strategies[i]
		state.Strategies[strategy.ID] = StrategyState{StrategyData: strategy, Status: StrategyNeutralStatus}
	}

	if event, ok := state.Events[game.OpeningEvent]; ok {
		event.Status = EventFiredStatus
		state.Events[game.OpeningEvent] = event
	}
	return state
}
