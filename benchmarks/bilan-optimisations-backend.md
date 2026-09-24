# Bilan des optimisations backend

Date du bilan : 2026-09-24

Ce document recense les optimisations backend mises en place dans le moteur de
simulation et mesure ce qu'elles ont apporte. Les temps ci-dessous concernent
le calcul d'un tick Go, sans HTTP, rendu frontend, disque ni reseau.

## Resume executif

Sur le benchmark comparable d'une carte de 600 x 600, le temps moyen d'un tick
est passe de **632,765 ms** avant optimisation a **123,491 ms** apres la
premiere optimisation, puis a **4,594 ms** avec le comptage parallele present
dans le workspace.

| Etat | Temps moyen | Gain vs initial | Allocations | Debit theorique |
| --- | ---: | ---: | ---: | ---: |
| Avant optimisation | 632,765 ms | - | 24 allocs/op | 1,58 tick/s |
| Indexation spatiale | 123,491 ms | 5,1x plus rapide | 0 alloc/op | 8,10 tick/s |
| Indexation + multithreading | 4,594 ms | 137,7x plus rapide | 17 allocs/op | 217,7 tick/s |

Le dernier chiffre est une mesure de cinq executions avec deux warmups. Il
correspond au code actuel, dont la partie multithreading n'est pas encore
presente dans un commit historique. Il faut donc le considerer comme le
resultat de l'arbre de travail actuel, pas comme une mesure d'une release.

## Etat initial

Avant l'optimisation du 24 septembre, chaque cellule saine parcourait les
cellules infectees pour trouver ses voisins. Cette recherche repetait le meme
travail pour de nombreuses cellules et le tick allouait egalement de la
memoire.

Mesure de reference :

- Benchmark : `BenchmarkTick-16`
- Carte : 600 x 600, densite 0,25, seed 42
- 5 runs, 2 warmups
- Temps moyen : **632,765 ms**
- P95 : **645,060 ms**
- Memoire : **3 870 930 B/op**
- Allocations : **24 allocs/op**
- Source : `benchmark-20260923-123048289.md`

## Chronologie

### 21 septembre : moteur de contamination initial

Commit `81e834b` - `GFRE - Add pool + parameters`.

Le moteur de contamination et ses regles sont introduits. Le generateur
aleatoire est conserve dans l'etat de la simulation au lieu d'etre recree a
chaque tick. Aucun benchmark directement comparable n'est archive pour cette
etape.

### 23 septembre : reduction de la carte generee

Commit `cff1f72` - `GFRE - taille de la map perf`.

La carte generee passe de 1500 x 1500 a 850 x 850 :

- nombre de cellules : 2 250 000 -> 722 500 ;
- reduction du nombre de cellules : environ **67,9 %** ;
- carte actuelle : 17 672 personnes et 73 settlements avec la seed 42.

Cette modification reduit le cout de `PopulationMap`, mais elle change la
charge de travail. Elle ne doit donc pas etre comparee directement au
benchmark `BenchmarkTick` de 600 x 600.

### 23 septembre : benchmark reproductible

Commit `594a513` - `mise en place d'un pdf`.

Le benchmark principal est stabilise sur une carte 600 x 600, une densite de
0,25 et la seed 42. Le script PowerShell ajoute aussi les runs, warmups,
percentiles et mesures de memoire. Cette etape n'accelere pas le moteur, mais
rend les comparaisons suivantes fiables.

### 24 septembre, 10:29 : indexation spatiale de `Board`

Commit `948ceb5` - `premiere optimisation`.

Deux changements reduisent le travail du tick :

1. Les cellules infectees sont placees dans des buckets indexes par position.
2. Une cellule saine ne visite plus toute la carte : elle ne parcourt que la
   boite correspondant au `FarRadius`.
3. Les buffers `nextCells` et `bucketHeads` sont reutilises entre les ticks.

Resultat mesure dans `benchmark-20260924-083014515.md` :

- **632,765 ms -> 123,491 ms** ;
- gain de **5,1x** ;
- latence reduite de **80,5 %** ;
- **3 870 930 -> 0 B/op** ;
- **24 -> 0 allocations/op**.

Le gain principal vient de la reduction du nombre de voisins examines. La
reutilisation des buffers supprime en plus la pression du garbage collector.

### 24 septembre, 10:37 : indexation spatiale de `PopulationMap`

Commit `e056e3c` - `optimisation front, lenteur`.

Le meme principe est applique a la carte sparse :

- liste chainee des personnes infectees par cellule spatiale ;
- recherche limitee au voisinage du `FarRadius` ;
- reutilisation de `infectedIndices`, `infectedHeads` et `nextInfected` ;
- ajout du benchmark `BenchmarkPopulationMapTick`.

Le script standard mesure encore uniquement `BenchmarkTick`. Il n'existe donc
pas de comparaison historique strictement equivalente avant/apres pour
`PopulationMap`. Une execution actuelle donne environ **0,986 ms/op**, mais
cette mesure inclut aussi le multithreading du workspace et ne permet pas
d'isoler le gain de cet unique commit.

### 24 septembre : comptage parallele des candidats

Modification actuellement presente dans `game/game.go` et
`game/map_generator.go`.

La phase couteuse de comptage des voisins est decoupee en secteurs traites par
des goroutines, avec un nombre de workers base sur `runtime.GOMAXPROCS`. Chaque
worker ecrit dans ses propres indices de buffers. Les tirages aleatoires
restent ensuite sequentiels et dans l'ordre historique : les resultats restent
reproductibles pour une meme seed.

Mesure de cinq runs et deux warmups dans
`benchmark-20260924-085333540.md` :

- **123,491 ms -> 4,594 ms** par rapport a l'etat indexe ;
- gain supplementaire de **26,9x** ;
- latence reduite de **96,3 %** par rapport a l'indexation seule ;
- **40 187 B/op** et **17 allocations/op**.

Le temps est fortement reduit, mais le parallellisme ajoute des allocations et
un cout de coordination. Pour les petites cartes, le code conserve un chemin
sequentiel afin d'eviter de lancer inutilement des workers.

## Etat apres optimisation

Sur la charge de reference 600 x 600 :

- le calcul est passe d'environ 0,63 seconde a environ 4,6 millisecondes ;
- le debit theorique est passe d'environ 1,58 a 217,7 ticks par seconde ;
- l'indexation a supprime les allocations du tick avant l'ajout des goroutines ;
- le multithreading apporte le gain de temps restant, au prix de 17
  allocations et environ 40 Ko par operation ;
- le calcul de `PopulationMap` est egalement indexe et parallele, mais son gain
  historique reste a mesurer avec un benchmark dedie comparable.

## Methodologie et limites

- Les comparaisons principales utilisent la meme carte 600 x 600, la seed 42,
  5 runs et 2 warmups.
- `ns/op` mesure le benchmark Go, tandis qu'Hyperfine mesure le processus
  complet de benchmark. Ces deux valeurs ne doivent pas etre confondues.
- Le benchmark ne mesure pas la latence HTTP, l'encodage JSON, le rendu canvas,
  les fichiers ou le reseau.
- Les mesures de 4 a 7 ms dependent de la charge de la machine et du nombre de
  processeurs logiques disponibles.
- Le multithreading est valide par `go test ./...`, mais le test race Go n'a
  pas ete execute dans l'environnement Windows actuel car CGO est desactive.

## Commandes de verification

```powershell
go test ./...
go test ./game -run '^$' -bench '^BenchmarkTick$|^BenchmarkPopulationMapTick$' -benchmem -count=1
.\benchmark.ps1 -Runs 5 -Warmup 2
```
