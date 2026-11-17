package gameengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validScenarioJSON() []byte {
	return []byte(`{
		"BasicAttributes": [
			{"ID": 1, "DefaultValue": 50, "MaxValue": 100, "MinValue": 0, "Name": "approval"}
		],
		"ConfigurableAttributes": [
			{"ID": 2, "DefaultValue": 20, "MaxValue": 100, "MinValue": 0, "Name": "investment", "CostPerUnit": 0.5}
		],
		"Branches": ["economy"],
		"Events": [
			{
				"ID": 10,
				"Name": "Opening",
				"Weights": [0.2, 0.4],
				"BasicPower": 10,
				"Branch": "economy",
				"Iteration": 0,
				"Thresholds": [{"AttributeID": 1, "Value": 60, "Higher": true}],
				"AttributeChanges": []
			}
		],
		"Hints": [{"ID": 3, "Cost": 2, "Name": "Hint"}],
		"Strategies": [{"ID": 4, "Cost": 5, "Name": "Strategy", "AttributeChanges": []}],
		"StartingBudget": {"InitialMoney": 100, "MaxMoney": 200, "InitialQP": 5},
		"OpeningEvent": 10,
		"MaxEventsPerTurn": 1
	}`)
}

func TestParseGameJSONMigratesLegacyThreshold(t *testing.T) {
	game, err := ParseGameJSON(validScenarioJSON())
	if err != nil {
		t.Fatalf("ParseGameJSON() error = %v", err)
	}

	got := game.Events[0].Thresholds[0]
	if got.Min != 60 || got.Max != 100 {
		t.Fatalf("legacy threshold migrated to [%v, %v], want [60, 100]", got.Min, got.Max)
	}
	if game.Events[0].Weights == nil || game.Events[0].AttributeChanges == nil {
		t.Fatal("event collections were not normalized")
	}
	if game.Hints[0].Prerequisites.Events == nil || game.Strategies[0].Prerequisites.Strategies == nil {
		t.Fatal("prerequisite collections were not normalized")
	}
}

func TestParseGameJSONRejectsWeightDimensionMismatch(t *testing.T) {
	data := strings.Replace(string(validScenarioJSON()), `"Weights": [0.2, 0.4]`, `"Weights": [0.2]`, 1)
	_, err := ParseGameJSON([]byte(data))
	if err == nil || !strings.Contains(err.Error(), "expected 2") {
		t.Fatalf("ParseGameJSON() error = %v, want weight dimension error", err)
	}
}

func TestParseGameJSONRejectsTrailingJSON(t *testing.T) {
	data := append(validScenarioJSON(), []byte(` {}`)...)
	_, err := ParseGameJSON(data)
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("ParseGameJSON() error = %v, want multiple JSON values error", err)
	}
}

func TestCreateInstanceInitializesStateAndPointers(t *testing.T) {
	game, err := ParseGameJSON(validScenarioJSON())
	if err != nil {
		t.Fatalf("ParseGameJSON() error = %v", err)
	}

	state := game.CreateInstance()
	if state.GameData != &game {
		t.Fatal("GameData does not point to source game")
	}
	if state.Budget.Money != 100 || state.Budget.QP != 5 || state.Budget.BudgetData != &game.StartingBudget {
		t.Fatalf("unexpected initial budget: %+v", state.Budget)
	}
	if got := state.Attributes[1].AttributeData; got != &game.BasicAttributes[0] {
		t.Fatal("basic attribute state does not point to source attribute")
	}
	if got := state.Attributes[2].AttributeData; got != &game.ConfigurableAttributes[0].GameAttribute {
		t.Fatal("configurable attribute state does not point to source attribute")
	}
	if got := state.Events[10].EventData; got != &game.Events[0] {
		t.Fatal("event state does not point to source event")
	}
	if state.Events[10].Status != EventFiredStatus {
		t.Fatal("opening event was not marked as fired")
	}
	if state.Hints[3].Status != HintNeutralStatus || state.Strategies[4].Status != StrategyNeutralStatus {
		t.Fatal("hints or strategies were not initialized as neutral")
	}
}

func TestGetGameInstanceCreatesIndependentScenarioCopies(t *testing.T) {
	game, err := ParseGameJSON(validScenarioJSON())
	if err != nil {
		t.Fatalf("ParseGameJSON() error = %v", err)
	}

	const id = "parser_create_independence_test"
	RegisterGame(id, func() Game { return game })
	t.Cleanup(func() {
		gameFactoriesMu.Lock()
		delete(gameFactories, id)
		gameFactoriesMu.Unlock()
	})

	first, err := GetGameInstance(id)
	if err != nil {
		t.Fatalf("GetGameInstance(first) error = %v", err)
	}
	second, err := GetGameInstance(id)
	if err != nil {
		t.Fatalf("GetGameInstance(second) error = %v", err)
	}

	first.Events[10].EventData.Name = "changed"
	if second.Events[10].EventData.Name == "changed" {
		t.Fatal("two game instances share mutable scenario data")
	}
}

func TestSaveAndLoadGameJSON(t *testing.T) {
	game, err := ParseGameJSON(validScenarioJSON())
	if err != nil {
		t.Fatalf("ParseGameJSON() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "nested", "scenario.json")
	if err := SaveGameJSON(path, game); err != nil {
		t.Fatalf("SaveGameJSON() error = %v", err)
	}
	loaded, err := CreateGameJSON(path)
	if err != nil {
		t.Fatalf("CreateGameJSON() error = %v", err)
	}
	if len(loaded.Events) != 1 || loaded.Events[0].ID != 10 {
		t.Fatalf("unexpected loaded events: %+v", loaded.Events)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved file does not exist: %v", err)
	}
}

func TestParseEmptyInputReturnsEmptyGame(t *testing.T) {
	game, err := ParseGameJSON([]byte(" \n\t"))
	if err != nil {
		t.Fatalf("ParseGameJSON() error = %v", err)
	}
	if game.Events == nil || game.BasicAttributes == nil || game.Branches == nil {
		t.Fatal("empty game collections should be initialized")
	}
}
