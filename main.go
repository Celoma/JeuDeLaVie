package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"jeu-de-la-vie/game"
)

const (
	boardWidth  = 40
	boardHeight = 25
)

type server struct {
	mu         sync.RWMutex
	simulation *game.Simulation
	population *game.PopulationMap
	mapRNG     *rand.Rand
	mapSeed    int64
}

func main() {
	address := flag.String("addr", ":8080", "adresse d'écoute HTTP")
	seed := flag.Int64("seed", game.GeneratedMapSeed, "seed de la simulation")
	width := flag.Int("width", boardWidth, "largeur du plateau")
	height := flag.Int("height", boardHeight, "hauteur du plateau")
	density := flag.Float64("density", 0.25, "densité initiale de personnes saines")
	flag.Parse()
	if *width <= 0 || *height <= 0 {
		log.Fatal("width et height doivent être strictement positifs")
	}
	if *density < 0 || *density > 1 {
		log.Fatal("density doit être comprise entre 0 et 1")
	}

	rules := game.ContaminationConfig{
		CloseRadius:    2,
		CloseChance:    0.5,
		FarRadius:      15,
		FarChance:      0.15,
		DeathChance:    0.02,
		RecoveryChance: 0.05,
		ImmunityChance: 0.8,
	}
	state := &server{
		simulation: game.NewSimulation(*width, *height, *density, *seed, rules),
		population: game.GeneratePopulationMap(game.GeneratedMapWidth, game.GeneratedMapHeight, *seed),
		mapRNG:     rand.New(rand.NewSource(*seed)),
		mapSeed:    *seed,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", state.handleState)
	mux.HandleFunc("/api/simulation", state.handleSimulation)
	mux.HandleFunc("/api/rules", state.handleRules)
	mux.HandleFunc("/api/map", state.handleMap)
	mux.HandleFunc("/api/benchmarks", handleBenchmarks)
	mux.HandleFunc("/api/tick", state.handleTick)
	mux.HandleFunc("/api/reset", state.handleReset)
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(".", "frontend"))))

	log.Printf("Jeu de contamination disponible sur http://localhost%s (seed %d)", *address, *seed)
	log.Fatal(http.ListenAndServe(*address, mux))
}

func handleBenchmarks(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, err := os.Open(filepath.Join("benchmarks", "latest.json"))
	if err != nil {
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write([]byte(`{"results":[],"configuration":{"runs":0,"warmup":0}}`))
		return
	}
	defer file.Close()

	response.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(response, file); err != nil {
		log.Printf("erreur de lecture des benchmarks: %v", err)
	}
}

func (server *server) handleMap(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	server.mu.RLock()
	defer server.mu.RUnlock()
	server.writeJSON(response, server.population)
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
	server.simulation.Step()
	server.population.Step(server.simulation.Rules, server.mapRNG)
	server.mu.Unlock()
	server.writeMap(response)
}

func (server *server) handleReset(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var resetRequest struct {
		Seed *int64 `json:"seed"`
	}
	if err := json.NewDecoder(request.Body).Decode(&resetRequest); err != nil && err != io.EOF {
		http.Error(response, "JSON invalide", http.StatusBadRequest)
		return
	}
	server.mu.Lock()
	if resetRequest.Seed == nil {
		server.simulation.Reset()
	} else {
		server.simulation.ResetWithSeed(*resetRequest.Seed)
		server.mapSeed = *resetRequest.Seed
	}
	server.mapRNG = rand.New(rand.NewSource(server.mapSeed))
	server.population = game.GeneratePopulationMap(game.GeneratedMapWidth, game.GeneratedMapHeight, server.mapSeed)
	server.mu.Unlock()
	server.writeMap(response)
}

func (server *server) handleSimulation(response http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	server.mu.RLock()
	defer server.mu.RUnlock()
	server.writeJSON(response, struct {
		Board *game.Board              `json:"board"`
		Rules game.ContaminationConfig `json:"rules"`
		Tick  uint64                   `json:"tick"`
		Seed  int64                    `json:"seed"`
		Map   *game.PopulationMap      `json:"map"`
	}{server.simulation.Board, server.simulation.Rules, server.simulation.Tick, server.simulation.Seed(), server.population})
}

func (server *server) handleRules(response http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		server.mu.RLock()
		rules := server.simulation.Rules
		server.mu.RUnlock()
		server.writeJSON(response, rules)
	case http.MethodPut:
		var rules game.ContaminationConfig
		if err := json.NewDecoder(request.Body).Decode(&rules); err != nil {
			http.Error(response, "JSON invalide", http.StatusBadRequest)
			return
		}
		if err := rules.Validate(); err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		server.mu.Lock()
		server.simulation.Rules = rules
		server.mu.Unlock()
		server.writeJSON(response, rules)
	default:
		http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (server *server) writeBoard(response http.ResponseWriter) {
	server.mu.RLock()
	defer server.mu.RUnlock()
	server.writeJSON(response, server.simulation.Board)
}

func (server *server) writeMap(response http.ResponseWriter) {
	server.mu.RLock()
	defer server.mu.RUnlock()
	server.writeJSON(response, server.population)
}

func (server *server) writeJSON(response http.ResponseWriter, value any) {
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		log.Printf("erreur d'encodage de l'état: %v", err)
	}
}
