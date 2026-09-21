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
- `frontend/` : page HTML et JavaScript sans framework ni style.

## API

- `GET /api/state` retourne la grille courante.
- `POST /api/tick` calcule la generation suivante.
- `POST /api/reset` recree une grille aleatoire.

L'etat est conserve en memoire. La grille initiale fait 40 colonnes par 25 lignes.
