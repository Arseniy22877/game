package gameengine

import (
	"encoding/json"
	"fmt"
	"os"
)

// CreateGameJSON загружает описание игры из JSON-файла.
//
// Путь передаётся непосредственно в os.ReadFile, поэтому относительные пути
// вычисляются относительно текущей рабочей директории приложения.
// Ошибки чтения и разбора возвращаются вызывающему коду.
func CreateGameJSON(path string) (Game, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Game{}, fmt.Errorf("read game JSON %q: %w", path, err)
	}

	var game Game
	if err := json.Unmarshal(data, &game); err != nil {
		return Game{}, fmt.Errorf("parse game JSON %q: %w", path, err)
	}
	return game, nil
}
