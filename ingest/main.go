package main

import (
	"log"
	"net/http"
	"os"

	"github.com/mauroza/gamestats/ingest/internal/games"
)

func main() {
	gamesDir := getenv("GAMES_DIR", "../games")
	addr := getenv("ADDR", ":8080")

	reg, err := games.Load(gamesDir)
	if err != nil {
		log.Fatalf("cargando juegos: %v", err)
	}
	log.Printf("juegos registrados: %d", len(reg))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	log.Printf("ingest escuchando en %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
