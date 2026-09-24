package game

import (
	"encoding/json"
	"io"
	"math/rand"
	"runtime"
	"sync"
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

	infectedIndices    []int
	infectedHeads      []int
	nextInfected       []int
	closeCounts        []int
	farCounts          []int
	candidateWaitGroup sync.WaitGroup
	candidateOffsets   []candidateOffset
	candidateRadius    int
}

func (populationMap *PopulationMap) Step(config ContaminationConfig, source *rand.Rand) {
	populationMap.ensureScratch()
	populationMap.ensureCandidateOffsets(config.FarRadius)
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

	populationMap.countInfectionCandidates(config)
	for index, person := range populationMap.People {
		if person.Infected || person.Dead || person.Immune {
			continue
		}
		if shouldInfectFromCounts(populationMap.closeCounts[index], populationMap.farCounts[index], config, source) {
			populationMap.People[index].Infected = true
		}
	}
}

func (populationMap *PopulationMap) shouldInfect(index int, config ContaminationConfig, source *rand.Rand) bool {
	closeCandidates, farCandidates := populationMap.infectionCandidateCounts(index, config)
	if closeCandidates > 0 {
		closeCandidates++
		farCandidates = 0
	} else {
		closeCandidates = farCandidates
	}
	return shouldInfectFromCounts(closeCandidates, farCandidates, config, source)
}

func (populationMap *PopulationMap) infectionCandidateCounts(index int, config ContaminationConfig) (int, int) {
	closeCandidates := 0
	farCandidates := 0
	target := populationMap.People[index]
	closeRadiusSquared := config.CloseRadius * config.CloseRadius
	farRadiusSquared := config.FarRadius * config.FarRadius
	for _, offset := range populationMap.candidateOffsets {
		candidateX := target.X + offset.deltaColumn
		candidateY := target.Y + offset.deltaRow
		if candidateX < 0 || candidateX >= populationMap.Width || candidateY < 0 || candidateY >= populationMap.Height {
			continue
		}
		infectedIndex := populationMap.infectedHeads[populationMap.index(candidateX, candidateY)]
		for infectedIndex != -1 {
			if offset.distanceSq <= closeRadiusSquared {
				closeCandidates++
			} else if offset.distanceSq <= farRadiusSquared {
				farCandidates++
			}
			infectedIndex = populationMap.nextInfected[infectedIndex]
		}
	}
	return closeCandidates, farCandidates
}

func (populationMap *PopulationMap) ensureCandidateOffsets(radius int) {
	if populationMap.candidateRadius == radius && populationMap.candidateOffsets != nil {
		return
	}
	populationMap.candidateOffsets = buildCandidateOffsets(radius)
	populationMap.candidateRadius = radius
}

func (populationMap *PopulationMap) countInfectionCandidates(config ContaminationConfig) {
	workerCount := min(runtime.GOMAXPROCS(0), (len(populationMap.People)+1023)/1024)
	if workerCount < 2 {
		for index, person := range populationMap.People {
			if person.Infected || person.Dead || person.Immune {
				continue
			}
			populationMap.closeCounts[index], populationMap.farCounts[index] = populationMap.infectionCandidateCounts(index, config)
		}
		return
	}

	candidatePoolOnce.Do(startCandidatePool)
	chunkSize := (len(populationMap.People) + workerCount - 1) / workerCount
	populationMap.candidateWaitGroup.Add(workerCount)
	for worker := 0; worker < workerCount; worker++ {
		start := worker * chunkSize
		end := min(len(populationMap.People), start+chunkSize)
		candidateJobs <- candidateJob{populationMap: populationMap, config: config, start: start, end: end, waitGroup: &populationMap.candidateWaitGroup}
	}
	populationMap.candidateWaitGroup.Wait()
}

func (populationMap *PopulationMap) ensureScratch() {
	peopleCount := len(populationMap.People)
	cellCount := populationMap.Width * populationMap.Height
	if cap(populationMap.infectedIndices) == peopleCount && len(populationMap.infectedHeads) == cellCount && len(populationMap.nextInfected) == peopleCount && len(populationMap.closeCounts) == peopleCount && len(populationMap.farCounts) == peopleCount {
		return
	}
	populationMap.infectedIndices = make([]int, 0, peopleCount)
	populationMap.infectedHeads = make([]int, cellCount)
	populationMap.nextInfected = make([]int, peopleCount)
	populationMap.closeCounts = make([]int, peopleCount)
	populationMap.farCounts = make([]int, peopleCount)
	for index := range populationMap.infectedHeads {
		populationMap.infectedHeads[index] = -1
	}
}

func (populationMap *PopulationMap) index(x, y int) int {
	return y*populationMap.Width + x
}

func GeneratePopulationMap(width, height int, seed int64) *PopulationMap {
	candidatePoolOnce.Do(startCandidatePool)
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

	populationMap := &PopulationMap{
		Width:           width,
		Height:          height,
		Seed:            seed,
		Settlements:     settlements,
		People:          people,
		infectedIndices: make([]int, 0, len(people)),
		infectedHeads:   newInfectedHeads(width * height),
		nextInfected:    make([]int, len(people)),
	}
	populationMap.ensureScratch()
	return populationMap
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
