package main

import (
	"flag"
	"log"
	"os"

	"jeu-de-la-vie/game"
)

const outputFile = "generated_map.json"

func main() {
	seed := flag.Int64("seed", game.GeneratedMapSeed, "graine utilisée pour générer la carte")
	flag.Parse()

	populationMap := game.GeneratePopulationMap(
		game.GeneratedMapWidth,
		game.GeneratedMapHeight,
		*seed,
	)

	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if err := game.WritePopulationMap(file, populationMap); err != nil {
		log.Fatal(err)
	}

	log.Printf("carte generee dans %s avec la seed %d (%d personnes)", outputFile, *seed, len(populationMap.People))
}
