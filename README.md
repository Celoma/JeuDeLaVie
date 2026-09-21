# Jeu de la vie

Projet de cours d'optimisation backend en Go autour du jeu de la vie de Conway.

## Prerequis

- Go 1.22 ou plus recent
- Un navigateur web

## Lancer le projet

Depuis la racine du projet :

```bash
go run .
```

Puis ouvrir <http://localhost:8080>.

## Tester

```bash
go test ./...
```

## Structure

- `main.go` : serveur HTTP et endpoints API.
- `game/` : moteur de simulation et tests unitaires.
- `frontend/` : canvas plein écran et simulation locale en JavaScript.

## API

- `GET /api/state` retourne la grille courante.
- `POST /api/tick` calcule la generation suivante.
- `POST /api/reset` recree une grille aleatoire.

Le frontend commence avec une vue de 100 par 100 cellules sur un monde de 10 000 par 10 000 cellules. Les cellules sont ajoutées au clic, la vue se déplace par glisser-déposer et se zoome à la molette. La simulation du frontend fonctionne localement.
