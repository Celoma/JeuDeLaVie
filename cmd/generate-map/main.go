package main

import (
	"log"
	"os"

	"jeu-de-la-vie/game"
)

const outputFile = "generated_map.json"

func main() {
	populationMap := game.GeneratePopulationMap(
		game.GeneratedMapWidth,
		game.GeneratedMapHeight,
		game.GeneratedMapSeed,
	)

	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	if err := game.WritePopulationMap(file, populationMap); err != nil {
		log.Fatal(err)
	}

	log.Printf("carte generee dans %s (%d personnes)", outputFile, len(populationMap.People))
}
