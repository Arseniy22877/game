package gameengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateGameJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "game.json")
	const content = `{
		"BasicAttributes": [{"ID": 1, "DefaultValue": 0.5, "MinValue": 0, "MaxValue": 1, "Name": "Trust"}],
		"Events": [{"ID": 7, "Name": "Launch", "ImageURL": "images/launch.png"}],
		"StartingBudget": {"InitialMoney": 100, "MaxMoney": 150, "InitialQP": 3},
		"StartingEvents": [7],
		"OpeningEvent": 7,
		"ProbabilityStrictness": 0.25,
		"PowerRestoreRate": 0.5,
		"MaxEventsPerTurn": 2,
		"Softness": 0.1
	}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	game, err := CreateGameJSON(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(game.BasicAttributes) != 1 || game.BasicAttributes[0].ID != 1 {
		t.Fatalf("unexpected basic attributes: %#v", game.BasicAttributes)
	}
	if game.StartingBudget.InitialMoney != 100 || game.StartingBudget.MaxMoney != 150 {
		t.Fatalf("unexpected budget: %#v", game.StartingBudget)
	}
	if game.OpeningEvent != 7 || game.MaxEventsPerTurn != 2 {
		t.Fatalf("unexpected game settings: %#v", game)
	}
	if game.Events[0].ImageURL != "images/launch.png" {
		t.Fatalf("unexpected event image URL: %q", game.Events[0].ImageURL)
	}
}

func TestCreateGameJSONReturnsErrorOnInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"StartingBudget":`), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateGameJSON(path); err == nil {
		t.Fatal("CreateGameJSON returned nil error for invalid JSON")
	}
}
