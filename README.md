# Jeu de contamination

Projet de cours d'optimisation backend en Go autour d'une simulation de contamination.

## Prerequis

- Go 1.22 ou plus recent
- Un navigateur web
- Hyperfine (pour les benchmarks)
- Python 3 avec `reportlab`, `matplotlib` et `pillow` pour le rapport PDF des benchmarks
- Graphviz (optionnel, pour les graphes d'appel CPU et mémoire)

Installation des dependances Python :

```powershell
python -m pip install reportlab matplotlib pillow
```

Graphviz peut être installé sous Windows avec :

```powershell
winget install --id Graphviz.Graphviz --exact
```

## Lancer le projet

Depuis la racine du projet :

```bash
go run .
```

Puis ouvrir <http://localhost:8080>.

Dans l'interface, `Commencer` enchaîne automatiquement les tours, `Pause` suspend la partie et `Réinitialiser` revient au tour zéro. Le curseur est réglé par défaut sur `1 tour/s` ; sa position minimale `Sans limite` enchaîne un tour dès que la réponse du précédent est reçue.

Pour régénérer la carte avec la seed déterministe par défaut :

```bash
go run ./cmd/generate-map
```

Pour choisir une autre seed :

```bash
go run ./cmd/generate-map -seed 123
```

## Backend de simulation

Le serveur maintient un plateau et applique les règles à chaque transition :

- `GET /api/state` retourne le plateau courant.
- `POST /api/tick` calcule puis retourne l'état suivant.
- `POST /api/reset` remet la simulation à son état initial avec la seed du serveur.
- `GET /api/simulation` retourne le plateau, les règles et le numéro du tick.
- `GET /api/rules` retourne les règles courantes.
- `PUT /api/rules` remplace les règles avec un JSON `closeRadius`, `closeChance`, `farRadius` et `farChance`.

Le serveur utilise la seed `42` par défaut. Les paramètres utiles sont configurables :

```bash
go run . -seed 42 -width 40 -height 25 -density 0.25 -addr :8080
```

Exemple de changement de règles :

```bash
curl -X PUT http://localhost:8080/api/rules `
	-H "Content-Type: application/json" `
	-d '{"closeRadius":2,"closeChance":0.5,"farRadius":15,"farChance":0.15}'
```

## Tester

```bash
go test ./...
```

## Benchmark performance

Le microbenchmark `BenchmarkTick` mesure uniquement une transition complète du moteur Go (`Board.Step`) sur une carte `600 × 600`, avec la seed `42`, des rayons `4` proche et `20` lointain, sans HTTP ni rendu. Le script utilise par défaut `3` runs et `1` warmup ; augmente-les avec `-Runs` et `-Warmup` pour une mesure plus précise.

```bash
go test ./game -run '^$' -bench '^BenchmarkTick$' -benchmem -count 1
```

Ou, sous PowerShell :

```powershell
.\benchmark.ps1
```

Le script crée `benchmarks/latest.json` pour l'application, `benchmarks/latest.md` pour le dernier résultat et une archive Markdown horodatée `benchmarks/benchmark-YYYYMMDD-HHmmssfff.md` à chaque exécution. Il produit aussi deux profils `pprof` horodatés (`cpu-*.prof` et `memory-*.prof`) et, si Hyperfine est installé, un export `hyperfine-*.json`. Le panneau « Dernier benchmark » de l'interface affiche automatiquement le contenu du rapport JSON au prochain chargement, puis ajoute des latences API mesurées en direct pour `/api/simulation`, `/api/tick` et `/api/reset`.

À la fin de chaque exécution, `generate_benchmark.py` génère également `benchmarks/pdf/benchmark-report-*.pdf` à partir des profils et des résultats les plus récents. Le PDF contient les mesures CPU `pprof`, les allocations mémoire `pprof`, la mémoire par opération (`B/op`), la trace détaillée du GC (`gc-*.log`) et les graphes d'appel si Graphviz est disponible. Les fichiers bruts (`.md`, `.json`, `.prof` et `.log`) restent ignorés par Git ; seuls les rapports PDF peuvent être versionnés.

Pour analyser un profil :

```powershell
go tool pprof .\benchmarks\cpu-YYYYMMDD-HHmmssfff.prof
go tool pprof .\benchmarks\memory-YYYYMMDD-HHmmssfff.prof
```

Pour analyser le profil CPU :

```powershell
go tool pprof -top benchmarks\cpu-YYYYMMDD-HHmmssfff.prof
```

Sur Windows, Hyperfine peut être installé avec :

```powershell
winget install sharkdp.hyperfine
```

## Structure

- `main.go` : serveur HTTP et endpoints API.
- `benchmark.ps1` : exécution des benchmarks Go et génération des rapports de mesure.
- `benchmarks/` : rapport JSON consommé par l'application et rapport Markdown lisible.
- `game/` : moteur de simulation, règles, tests unitaires et benchmarks.
- `frontend/` : canvas plein écran et simulation locale en JavaScript.

## API

- `GET /api/state` retourne l'état courant de la contamination.
- `POST /api/tick` calcule le tour suivant avec les rayons et probabilités de contamination.
- `POST /api/reset` recrée une population aléatoire avec une personne contaminée initiale.

Le frontend permet de régler les deux rayons, leurs chances de contamination, la population initiale et le mode de placement manuel. La vue se déplace par glisser-déposer et se zoome à la molette.
