# Bilan des optimisations backend

Date du bilan : 2026-09-24

Ce document recense les optimisations backend mises en place dans le moteur de
simulation et mesure ce qu'elles ont apporte. Les temps ci-dessous concernent
le calcul d'un tick Go, sans HTTP, rendu frontend, disque ni reseau.

## Resume executif

Sur le benchmark comparable d'une carte de 600 x 600, le temps moyen d'un tick
est passe de **632,765 ms** avant optimisation a **123,491 ms** apres la
premiere optimisation. Le dernier benchmark mesure **6,346 ms** avec
l'indexation, le comptage parallele, le pool persistant, la preallocation des
buffers et les offsets circulaires.

| Etat | Temps moyen | Gain vs initial | Allocations | Debit theorique |
| --- | ---: | ---: | ---: | ---: |
| Avant optimisation | 632,765 ms | - | 24 allocs/op | 1,58 tick/s |
| Indexation spatiale | 123,491 ms | 5,1x plus rapide | 0 alloc/op | 8,10 tick/s |
| Indexation + goroutines par tick | 4,594 ms | 137,7x plus rapide | 17 allocs/op | 217,7 tick/s |
| Indexation + pool + buffers + offsets circulaires | 6,346 ms | 99,7x plus rapide | 0 allocs/op | 157,6 tick/s |

Le dernier chiffre est une mesure de cinq executions avec deux warmups. Il
correspond au code actuel, qui contient le pool de workers persistant et les
buffers prealloues. Il faut donc le considerer comme le resultat de l'arbre de
travail actuel, pas comme la mesure d'une release versionnee.

## Ordre des quatre optimisations

La reduction de la taille de la carte et la mise en place du benchmark sont des
etapes de preparation de la mesure. Les quatre optimisations du moteur sont :

1. **Indexation spatiale** de `Board`, puis extension a `PopulationMap`.
2. **Comptage parallele** des candidats avec plusieurs goroutines.
3. **Pool de workers persistants**, avec preallocation des buffers pour eviter
  les allocations a chaque tick.
4. **Offsets circulaires**, pour ne parcourir que le disque reel du rayon au
  lieu de toute sa boite carree.

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

### Optimisation 1 — 24 septembre, 10:29 : indexation spatiale de `Board`

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

#### Extension de l'optimisation 1 a `PopulationMap`

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

### Optimisation 2 — 24 septembre : comptage parallele des candidats

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

Le temps est fortement reduit, mais la creation de goroutines a chaque tick
ajoute des allocations et un cout de coordination. Pour les petites cartes,
le code conserve un chemin sequentiel afin d'eviter de lancer inutilement des
workers.

### Optimisation 3 — 24 septembre : pool de workers persistants

Modification actuellement presente dans `game/game.go` et
`game/map_generator.go`.

Les workers ne sont plus recrees a chaque tick :

- un pool est initialise une seule fois avec `runtime.GOMAXPROCS(0)` workers ;
- chaque tick envoie des secteurs au pool via un canal reutilise ;
- les buffers de compteurs et les `WaitGroup` appartiennent aux cartes et sont
  reutilises ;
- le chemin sequentiel est conserve pour les petites cartes.

Une mesure intermediaire de cinq runs et deux warmups donnait :

- **4,594 ms -> 4,321 ms** ;
- gain supplementaire de **5,9 %** sur la latence ;
- **17 -> 0 allocations/op** reportees ;
- **35 503 B/op** ;
- debit theorique : **231,4 ticks/s**.

Le pool supprime donc les allocations liees au lancement repete des goroutines.
La latence varie selon la charge de la machine ; le gain principal de cette
etape est la suppression des allocations de goroutines, pas une garantie de
gain de latence a chaque execution.

#### Preallocation des buffers dans l'optimisation 3

Les tableaux `closeCounts` et `farCounts` de `Board`, ainsi que les buffers
scratch de `PopulationMap`, sont maintenant alloues dans les constructeurs,
avant le debut de la mesure du tick.

Resultat du benchmark officiel final, cinq runs et deux warmups :

- **0,4 B/op** ;
- **0 allocs/op** ;
- le benchmark cible affiche **0 B/op** sur une execution directe ;
- temps moyen : **6,265 ms/op** ;
- debit theorique : **159,6 ticks/s**.

Le `0,4 B/op` du rapport est un reliquat moyen arrondi provenant de la mesure
du runtime ; les allocations par tick sont nulles dans le benchmark cible.

### Optimisation 4 — 24 septembre : offsets circulaires du voisinage

Modification presente dans `game/game.go` et `game/map_generator.go`.

La recherche ne parcourt plus toute la boite carree du `FarRadius`. Une liste
d'offsets appartenant au disque du rayon est construite une fois puis reutilisee
par les ticks suivants :

- les cases situees dans les coins de la boite sont ignorees ;
- la distance au centre est pre-calculee dans chaque offset ;
- les deux moteurs (`Board` et `PopulationMap`) utilisent le meme principe ;
- les offsets sont mis en cache et les benchmarks les prechauffent avant la
  mesure pour conserver `0 B/op` et `0 allocs/op` en steady-state.

Sur une execution ciblee, `BenchmarkTick` est passe d'environ `6,09 ms` avec
le balayage carre a `5,14 ms` avec les offsets circulaires, soit environ
**15,6 %** de mieux. Le run officiel suivant a signale un outlier Hyperfine et
mesure `6,346 ms`; le gain doit donc etre confirme sur une machine calme avec
plusieurs repetitions.

## Etat apres optimisation

Sur la charge de reference 600 x 600 :

- le calcul est passe d'environ 0,63 seconde a environ 6,3 millisecondes sur
  le dernier run officiel ;
- le debit theorique est passe d'environ 1,58 a 157,6 ticks par seconde sur ce
  run ;
- l'indexation a supprime les allocations du tick avant l'ajout des goroutines ;
- le pool persistant conserve le multithreading tout en ramenant les allocations
  reportees a 0 par operation ;
- la preallocation des buffers ramene la memoire par operation a environ 0 B/op ;
- les offsets circulaires reduisent le nombre de cases vides examinees, mais
  leur gain doit etre confirme avec des runs sans outlier ;
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
- Le benchmark final est mesure avec le pool persistant : les premiers ticks
  peuvent inclure son initialisation, alors que les ticks suivants reutilisent
  les workers.
- Le multithreading est valide par `go test ./...`, mais le test race Go n'a
  pas ete execute dans l'environnement Windows actuel car CGO est desactive.

## Commandes de verification

```powershell
go test ./...
go test ./game -run '^$' -bench '^BenchmarkTick$|^BenchmarkPopulationMapTick$' -benchmem -count=1
.\benchmark.ps1 -Runs 5 -Warmup 2
```
