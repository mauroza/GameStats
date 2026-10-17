// Package games carga la configuración de cada juego (acciones válidas y
// campos requeridos en data). Agregar un juego = agregar un archivo JSON.
package games

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config describe un juego: acción -> campos obligatorios de data, y las
// acciones de juego que no pertenecen a un jugador (ej. ROUND_END).
type Config struct {
	GameID          string              `json:"gameId"`
	Actions         map[string][]string `json:"actions"`
	NoPlayerActions []string            `json:"noPlayerActions,omitempty"`
}

// Registry contiene las configuraciones indexadas por gameId.
type Registry map[string]Config

// Load lee todos los *.json de dir.
func Load(dir string) (Registry, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	reg := Registry{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Config
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if c.GameID == "" {
			return nil, fmt.Errorf("%s: gameId vacío", f)
		}
		reg[c.GameID] = c
	}
	return reg, nil
}
