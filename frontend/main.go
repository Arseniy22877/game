package main

import (
	"fmt"

	"github.com/Arseniy22877/game/gameengine"
)

func main() {
	if !runUnitTests(CreateTestGame()) {
		fmt.Println("Unit tests failed; frontend skipped.")
		return
	}
	state, err := gameengine.GetGameInstance("first")
	if err != nil {
		fmt.Printf("Game startup failed: %v\n", err)
		return
	}
	game := *state.GameData
	printGameCharacteristics(game)
	if err := serveFrontend(":8080", state); err != nil {
		fmt.Printf("Frontend stopped: %v\n", err)
	}
}
