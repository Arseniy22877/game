package main

import "fmt"

func main() {
	if !runUnitTests(createTestGame()) {
		fmt.Println("Unit tests failed; frontend skipped.")
		return
	}
	game := createFirstGame()
	printGameCharacteristics(game)
	if err := serveFrontend(":8080", game); err != nil {
		fmt.Printf("Frontend stopped: %v\n", err)
	}
}
