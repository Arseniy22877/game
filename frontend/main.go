package main

import (
	"fmt"

	"github.com/Arseniy22877/game/gameengine"
)

func main() {
	if !runUnitTests(gameengine.CreateTestGame()) {
		fmt.Println("Unit tests failed; frontend skipped.")
		return
	}
	game := gameengine.CreateFirstGame()
	printGameCharacteristics(game)
	if err := serveFrontend(":8080", game); err != nil {
		fmt.Printf("Frontend stopped: %v\n", err)
	}
}
