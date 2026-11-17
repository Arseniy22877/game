package gameengine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// NewEmptyGame returns an editable game with initialized collections.
func NewEmptyGame() Game {
	return Game{
		BasicAttributes:        []GameAttribute{},
		ConfigurableAttributes: []ConfigurableAttribute{},
		Branches:               []Branch{},
		Events:                 []GameEvent{},
		Hints:                  []Hint{},
		Strategies:             []Strategy{},
		StartingBudget:         Budget{},
	}
}

// CreateGameJSON reads and parses a game JSON file.
// An empty or whitespace-only file represents an empty game.
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

// ParseGameJSON parses game data. Empty input is treated as an empty game.
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
		return Game{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Game{}, err
	}

	if err := migrateLegacyThresholds(trimmed, &game); err != nil {
		return Game{}, fmt.Errorf("migrate legacy thresholds: %w", err)
	}

	normalizeGame(&game)
	return game, nil
}

// PrettyGameJSON serializes a game as indented JSON.
func PrettyGameJSON(game Game) ([]byte, error) {
	return json.MarshalIndent(game, "", "  ")
}

// SaveGameJSON validates and writes a game JSON file.
func SaveGameJSON(path string, game Game) error {
	if err := ValidateGame(&game); err != nil {
		return fmt.Errorf("validate game: %w", err)
	}

	normalizeGame(&game)
	data, err := PrettyGameJSON(game)
	if err != nil {
		return fmt.Errorf("encode game JSON: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write game JSON %q: %w", path, err)
	}
	return nil
}

// ValidateGame checks IDs and references used by a game.
func ValidateGame(game *Game) error {
	if game == nil {
		return fmt.Errorf("game is nil")
	}

	attributeIDs := make(map[AttributeId]struct{}, len(game.BasicAttributes)+len(game.ConfigurableAttributes))
	for _, attribute := range game.BasicAttributes {
		if _, exists := attributeIDs[attribute.ID]; exists {
			return fmt.Errorf("duplicate attribute ID %d", attribute.ID)
		}
		attributeIDs[attribute.ID] = struct{}{}
	}
	for _, attribute := range game.ConfigurableAttributes {
		if _, exists := attributeIDs[attribute.ID]; exists {
			return fmt.Errorf("duplicate attribute ID %d", attribute.ID)
		}
		attributeIDs[attribute.ID] = struct{}{}
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
	for _, event := range game.Events {
		owner := fmt.Sprintf("event %d", event.ID)
		if err := validatePrerequisites(event.Prerequisites, eventIDs, strategyIDs, owner); err != nil {
			return err
		}
		for thresholdIndex, threshold := range event.Thresholds {
			if _, exists := attributeIDs[threshold.AttributeID]; !exists {
				return fmt.Errorf("event %d threshold %d uses unknown attribute ID %d", event.ID, thresholdIndex, threshold.AttributeID)
			}
			if threshold.Min > threshold.Max {
				return fmt.Errorf("event %d threshold %d has Min greater than Max", event.ID, thresholdIndex)
			}
		}
		for _, change := range event.AttributeChanges {
			if _, exists := attributeIDs[change.AttributeID]; !exists {
				return fmt.Errorf("event %d uses unknown attribute ID %d", event.ID, change.AttributeID)
			}
		}
	}

	for _, strategy := range game.Strategies {
		owner := fmt.Sprintf("strategy %d", strategy.ID)
		if err := validatePrerequisites(strategy.Prerequisites, eventIDs, strategyIDs, owner); err != nil {
			return err
		}
		for _, change := range strategy.AttributeChanges {
			if _, exists := attributeIDs[change.AttributeID]; !exists {
				return fmt.Errorf("strategy %d uses unknown attribute ID %d", strategy.ID, change.AttributeID)
			}
		}
	}

	for _, hint := range game.Hints {
		owner := fmt.Sprintf("hint %d", hint.ID)
		if err := validatePrerequisites(hint.Prerequisites, eventIDs, strategyIDs, owner); err != nil {
			return err
		}
	}

	return nil
}

// migrateLegacyThresholds converts the previous {Value, Higher} representation
// into an inclusive [Min, Max] range. Value is retained in the new model.
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
				attributeLimit.min = threshold.Value
				attributeLimit.max = threshold.Value
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

func unmarshalJSONField(fields map[string]json.RawMessage, name string, target interface{}) error {
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
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
