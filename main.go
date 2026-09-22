package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"jeu-de-la-vie/game"
)

const (
	boardWidth  = 40
	boardHeight = 25
)

type server struct {
	mu    sync.RWMutex
	board *game.Board
	rng   *rand.Rand
	rules game.ContaminationConfig
}

func main() {
	state := &server{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
		rules: game.ContaminationConfig{
			CloseRadius: 2,
			CloseChance: 0.5,
			FarRadius:   15,
			FarChance:   0.15,
		},
	}
	state.board = game.RandomBoard(boardWidth, boardHeight, 0.25, state.rng)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", state.handleState)
	mux.HandleFunc("/api/map", handleMap)
	mux.HandleFunc("/api/tick", state.handleTick)
	mux.HandleFunc("/api/reset", state.handleReset)
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(".", "frontend"))))

	log.Println("Jeu de contamination disponible sur http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func handleMap(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, err := os.Open("generated_map.json")
	if err != nil {
		http.Error(response, "carte introuvable", http.StatusNotFound)
		return
	}
	defer file.Close()

	populationMap, err := game.ReadPopulationMap(file)
	if err != nil {
		http.Error(response, "carte JSON invalide", http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(populationMap); err != nil {
		log.Printf("erreur d'encodage de la carte: %v", err)
	}
}

func (server *server) handleState(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	server.writeBoard(response)
}

func (server *server) handleTick(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	server.mu.Lock()
	server.board.Step(server.rules, server.rng)
	server.mu.Unlock()
	server.writeBoard(response)
}

func (server *server) handleReset(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	server.mu.Lock()
	server.board = game.RandomBoard(boardWidth, boardHeight, 0.25, server.rng)
	server.mu.Unlock()
	server.writeBoard(response)
}

func (server *server) writeBoard(response http.ResponseWriter) {
	server.mu.RLock()
	defer server.mu.RUnlock()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(server.board); err != nil {
		log.Printf("erreur d'encodage de l'état: %v", err)
	}
}
