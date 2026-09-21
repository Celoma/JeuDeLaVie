# Jeu de contamination

Projet de cours d'optimisation backend en Go autour d'une simulation de contamination.

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

- `GET /api/state` retourne l'état courant de la contamination.
- `POST /api/tick` calcule le tour suivant avec les rayons et probabilités de contamination.
- `POST /api/reset` recrée une population aléatoire avec une personne contaminée initiale.

Le frontend permet de régler les deux rayons, leurs chances de contamination, la population initiale et le mode de placement manuel. La vue se déplace par glisser-déposer et se zoome à la molette.
