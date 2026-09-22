# Jeu de contamination

Projet de cours d'optimisation backend en Go autour d'une simulation de contamination.

## Prerequis

- Go 1.22 ou plus recent
- Un navigateur web
- Hyperfine (pour les benchmarks)

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

Le benchmark `BenchmarkBoardStep` mesure une transition complète du moteur Go avec des rayons de test un peu plus larges (`4` proche et `20` lointain). Le script lance plusieurs exécutions indépendantes pour calculer les statistiques affichées dans le front : latence moyenne, médiane, P95/P99, max, `ns/op`, `B/op`, `allocs/op` et débit.

```bash
go test ./game -run '^$' -bench '^BenchmarkBoardStep$' -benchmem -count 1
```

Ou, sous PowerShell :

```powershell
.\benchmark.ps1
```

Le script crée `benchmarks/latest.json` pour l'application et `benchmarks/latest.md` pour une lecture humaine sous forme de tableau. Le panneau « Dernier benchmark » de l'interface affiche automatiquement le contenu du rapport JSON au prochain chargement, puis ajoute des latences API mesurées en direct pour `/api/simulation`, `/api/tick` et `/api/reset`.

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
