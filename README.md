# 2048 Royale 🎮⚔️

Un clone du jeu 2048 avec animations fluides, mode 1v1 local et multijoueur en ligne via WebSocket.

## Modes de jeu

### 🎯 Solo
Mode classique — atteins 2048 et bats ton record.

### ⚔️ 1v1 Local
2 joueurs sur le même clavier, le premier à 2048 gagne !
- **Joueur 1** : `W` `A` `S` `D`
- **Joueur 2** : `↑` `↓` `←` `→`

### 🌐 En ligne
Affronte un ami via WebSocket. Nécessite le serveur Go.

## Lancer le jeu

### Mode solo / local (pas de serveur requis)
Ouvrir `index.html` dans un navigateur.

### Mode en ligne
```bash
cd server
go mod tidy
go run main.go
```
Puis ouvrir http://localhost:8080 dans le navigateur.

Un joueur crée une room et partage le code, l'autre le rejoint.

## Features
- **Animations fluides** : glissement des tuiles avec easing, pop au merge, spawn animé
- **Particules** : score flottant au merge
- **Glow** : halo lumineux sur les tuiles >= 128
- **Countdown** : décompte 3-2-1-GO en mode multi
- **Responsive** : desktop + mobile (swipe tactile)
- **Design** : thème dark/néon, typographies JetBrains Mono + Outfit

## Stack
| Composant | Techno |
|-----------|--------|
| Frontend | HTML5 Canvas, CSS3, JavaScript vanilla (ES6+) |
| Backend | Go + gorilla/websocket |
| Fonts | Google Fonts (Outfit + JetBrains Mono) |

## Structure
```
2048-royale/
├── index.html          # Jeu complet (solo + local + client online)
├── README.md
└── server/
    ├── go.mod
    └── main.go         # Serveur WebSocket pour le multi en ligne
```

## Roadmap
- [ ] Système de malus (freeze, scramble, blind, block)
- [ ] Leaderboard avec système ELO
- [ ] Mode speed (2 min chrono)
- [ ] Mode survie (tuiles spéciales)
- [ ] Spectateur mode
