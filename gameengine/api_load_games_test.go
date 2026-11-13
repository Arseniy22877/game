package gameengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadGames(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(workingDirectory); err != nil {
			t.Fatal(err)
		}
	}()

	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "scenarios"), 0700); err != nil {
		t.Fatal(err)
	}
	const scenario = `{
		"StartingBudget": {"InitialMoney": 20, "MaxMoney": 30, "InitialQP": 4},
		"BasicAttributes": [{"ID": 0, "DefaultValue": 0.5, "MinValue": 0, "MaxValue": 1}],
		"Events": []
	}`
	if err := os.WriteFile(filepath.Join(directory, "scenarios", "sample.json"), []byte(scenario), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(directory); err != nil {
		t.Fatal(err)
	}

	if err := LoadGames(); err != nil {
		t.Fatal(err)
	}
	state, err := GetGameInstance("sample")
	if err != nil {
		t.Fatal(err)
	}
	if state.Budget.Money != 20 || state.Budget.QP != 4 {
		t.Fatalf("unexpected loaded budget: money=%d QP=%d", state.Budget.Money, state.Budget.QP)
	}
}
