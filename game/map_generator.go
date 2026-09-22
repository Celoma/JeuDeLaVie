package game

import (
	"encoding/json"
	"io"
	"math/rand"
)

const (
	GeneratedMapWidth               = 2000
	GeneratedMapHeight              = 2000
	GeneratedMapSeed          int64 = 42
	GeneratedMapFamilySpacing       = 18
	GeneratedMapFamilyRadius        = 3
	GeneratedMapFamilySize          = 8
)

type SettlementType string

const (
	SettlementIgloo SettlementType = "igloo"
	SettlementCity  SettlementType = "city"
)

type Person struct {
	X        int   `json:"x"`
	Y        int   `json:"y"`
	Age      uint8 `json:"age"`
	Infected bool  `json:"infected"`
}

type Settlement struct {
	ID   uint32         `json:"id"`
	Type SettlementType `json:"type"`
	X    int            `json:"x"`
	Y    int            `json:"y"`
}

type PopulationMap struct {
	Width       int          `json:"width"`
	Height      int          `json:"height"`
	Seed        int64        `json:"seed"`
	Settlements []Settlement `json:"settlements"`
	People      []Person     `json:"people"`
}

func GeneratePopulationMap(width, height int, seed int64) *PopulationMap {
	source := rand.New(rand.NewSource(seed))
	settlementCount := width*height/10000 + 1
	settlements := make([]Settlement, settlementCount)
	for index := range settlements {
		settlementType := SettlementIgloo
		if index%5 == 0 {
			settlementType = SettlementCity
		}
		settlements[index] = Settlement{
			ID:   uint32(index + 1),
			Type: settlementType,
			X:    source.Intn(width),
			Y:    source.Intn(height),
		}
	}

	people := make([]Person, 0, familyCount(width, height)*GeneratedMapFamilySize)
	for centerY := GeneratedMapFamilySpacing / 2; centerY < height; centerY += GeneratedMapFamilySpacing {
		for centerX := GeneratedMapFamilySpacing / 2; centerX < width; centerX += GeneratedMapFamilySpacing {
			familyX := clamp(centerX+source.Intn(9)-4, GeneratedMapFamilyRadius, width-GeneratedMapFamilyRadius-1)
			familyY := clamp(centerY+source.Intn(9)-4, GeneratedMapFamilyRadius, height-GeneratedMapFamilyRadius-1)
			addFamily(&people, familyX, familyY, source)
		}
	}
	people[source.Intn(len(people))].Infected = true

	return &PopulationMap{
		Width:       width,
		Height:      height,
		Seed:        seed,
		Settlements: settlements,
		People:      people,
	}
}

func familyCount(width, height int) int {
	return ((width-1)/GeneratedMapFamilySpacing + 1) * ((height-1)/GeneratedMapFamilySpacing + 1)
}

func addFamily(people *[]Person, centerX, centerY int, source *rand.Rand) {
	occupied := make(map[[2]int]bool, GeneratedMapFamilySize)
	for len(occupied) < GeneratedMapFamilySize {
		x := centerX + source.Intn(2*GeneratedMapFamilyRadius+1) - GeneratedMapFamilyRadius
		y := centerY + source.Intn(2*GeneratedMapFamilyRadius+1) - GeneratedMapFamilyRadius
		if occupied[[2]int{x, y}] {
			continue
		}
		occupied[[2]int{x, y}] = true
		age := uint8(source.Intn(46) + 25)
		if len(occupied) > 2 {
			age = uint8(source.Intn(26))
		}
		*people = append(*people, Person{X: x, Y: y, Age: age})
	}
}

func clamp(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func max(first, second int) int {
	if first > second {
		return first
	}
	return second
}

func min(first, second int) int {
	if first < second {
		return first
	}
	return second
}

func WritePopulationMap(writer io.Writer, populationMap *PopulationMap) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(populationMap)
}

func ReadPopulationMap(reader io.Reader) (*PopulationMap, error) {
	populationMap := &PopulationMap{}
	if err := json.NewDecoder(reader).Decode(populationMap); err != nil {
		return nil, err
	}
	return populationMap, nil
}
