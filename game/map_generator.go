package game

import (
	"encoding/json"
	"io"
	"math/rand"
)

const (
	GeneratedMapWidth               = 850
	GeneratedMapHeight              = 850
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
	Dead     bool  `json:"dead"`
	Immune   bool  `json:"immune"`
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

	infectedIndices []int
	infectedHeads   []int
	nextInfected    []int
}

func (populationMap *PopulationMap) Step(config ContaminationConfig, source *rand.Rand) {
	populationMap.ensureScratch()
	infected := populationMap.infectedIndices[:0]
	for index, person := range populationMap.People {
		if !person.Infected || person.Dead {
			continue
		}
		if source.Float64() < config.DeathChance {
			populationMap.People[index].Infected = false
			populationMap.People[index].Dead = true
			continue
		}
		if source.Float64() < config.RecoveryChance {
			populationMap.People[index].Infected = false
			populationMap.People[index].Immune = source.Float64() < config.ImmunityChance
			continue
		}
		infected = append(infected, index)
	}
	for index := range populationMap.infectedHeads {
		populationMap.infectedHeads[index] = -1
	}
	for _, infectedIndex := range infected {
		person := populationMap.People[infectedIndex]
		bucket := populationMap.index(person.X, person.Y)
		populationMap.nextInfected[infectedIndex] = populationMap.infectedHeads[bucket]
		populationMap.infectedHeads[bucket] = infectedIndex
	}

	for index, person := range populationMap.People {
		if person.Infected || person.Dead || person.Immune {
			continue
		}
		if populationMap.shouldInfect(index, config, source) {
			populationMap.People[index].Infected = true
		}
	}
}

func (populationMap *PopulationMap) shouldInfect(index int, config ContaminationConfig, source *rand.Rand) bool {
	closeCandidates := 0
	farCandidates := 0
	target := populationMap.People[index]
	closeRadiusSquared := config.CloseRadius * config.CloseRadius
	farRadiusSquared := config.FarRadius * config.FarRadius
	minX := max(0, target.X-config.FarRadius)
	maxX := min(populationMap.Width-1, target.X+config.FarRadius)
	minY := max(0, target.Y-config.FarRadius)
	maxY := min(populationMap.Height-1, target.Y+config.FarRadius)
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			infectedIndex := populationMap.infectedHeads[populationMap.index(x, y)]
			for infectedIndex != -1 {
				infectedPerson := populationMap.People[infectedIndex]
				distance := squaredDistance(target.X, target.Y, infectedPerson.X, infectedPerson.Y)
				if distance <= closeRadiusSquared {
					closeCandidates++
				} else if distance <= farRadiusSquared {
					farCandidates++
				}
				infectedIndex = populationMap.nextInfected[infectedIndex]
			}
		}
	}
	chance := config.FarChance
	if closeCandidates > 0 {
		chance = config.CloseChance
		closeCandidates += 1
	} else {
		closeCandidates = farCandidates
	}
	for range closeCandidates {
		if source.Float64() < chance {
			return true
		}
	}
	return false
}

func (populationMap *PopulationMap) ensureScratch() {
	peopleCount := len(populationMap.People)
	cellCount := populationMap.Width * populationMap.Height
	if cap(populationMap.infectedIndices) == peopleCount && len(populationMap.infectedHeads) == cellCount && len(populationMap.nextInfected) == peopleCount {
		return
	}
	populationMap.infectedIndices = make([]int, 0, peopleCount)
	populationMap.infectedHeads = make([]int, cellCount)
	populationMap.nextInfected = make([]int, peopleCount)
	for index := range populationMap.infectedHeads {
		populationMap.infectedHeads[index] = -1
	}
}

func (populationMap *PopulationMap) index(x, y int) int {
	return y*populationMap.Width + x
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
		Width:           width,
		Height:          height,
		Seed:            seed,
		Settlements:     settlements,
		People:          people,
		infectedIndices: make([]int, 0, len(people)),
		infectedHeads:   newInfectedHeads(width * height),
		nextInfected:    make([]int, len(people)),
	}
}

func newInfectedHeads(size int) []int {
	head := make([]int, size)
	for index := range head {
		head[index] = -1
	}
	return head
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
