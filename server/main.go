package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ═══════════════════════════════════════
//  MODELS
// ═══════════════════════════════════════

type Player struct {
	ID   string
	Conn *websocket.Conn
	Grid [4][4]int
	Score int
	Lost  bool
	Won   bool
	mu    sync.Mutex
}

type Room struct {
	Code    string
	Players []*Player
	Started bool
	mu      sync.Mutex
}

type Message struct {
	Type      string          `json:"type"`
	Room      string          `json:"room,omitempty"`
	Direction string          `json:"direction,omitempty"`
	PlayerID  string          `json:"player_id,omitempty"`
	Grid      *[4][4]int      `json:"grid,omitempty"`
	Score     int             `json:"score,omitempty"`
	Winner    string          `json:"winner,omitempty"`
	Message   string          `json:"message,omitempty"`
}

// ═══════════════════════════════════════
//  GLOBALS
// ═══════════════════════════════════════

var (
	rooms    = make(map[string]*Room)
	roomsMu  sync.RWMutex
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

// ═══════════════════════════════════════
//  ROOM MANAGEMENT
// ═══════════════════════════════════════

func generateRoomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 5)
	for i := range code {
		code[i] = chars[rand.Intn(len(chars))]
	}
	return string(code)
}

func generatePlayerID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	id := make([]byte, 8)
	for i := range id {
		id[i] = chars[rand.Intn(len(chars))]
	}
	return string(id)
}

func getOrCreateRoom(code string) *Room {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	if room, ok := rooms[code]; ok {
		return room
	}
	return nil
}

func createRoom() *Room {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	var code string
	for {
		code = generateRoomCode()
		if _, exists := rooms[code]; !exists {
			break
		}
	}

	room := &Room{Code: code}
	rooms[code] = room
	log.Printf("Room created: %s", code)
	return room
}

func removeRoom(code string) {
	roomsMu.Lock()
	defer roomsMu.Unlock()
	delete(rooms, code)
	log.Printf("Room removed: %s", code)
}

// ═══════════════════════════════════════
//  WEBSOCKET HANDLER
// ═══════════════════════════════════════

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	player := &Player{
		ID:   generatePlayerID(),
		Conn: conn,
	}

	var currentRoom *Room

	log.Printf("Player connected: %s", player.ID)

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Player %s disconnected: %v", player.ID, err)
			if currentRoom != nil {
				handleDisconnect(currentRoom, player)
			}
			return
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "create":
			room := createRoom()
			room.mu.Lock()
			room.Players = append(room.Players, player)
			room.mu.Unlock()
			currentRoom = room

			sendJSON(conn, Message{
				Type:     "room_created",
				Room:     room.Code,
				PlayerID: player.ID,
			})

		case "join":
			code := strings.ToUpper(strings.TrimSpace(msg.Room))
			room := getOrCreateRoom(code)
			if room == nil {
				sendJSON(conn, Message{Type: "error", Message: "Room introuvable"})
				continue
			}

			room.mu.Lock()
			if len(room.Players) >= 2 {
				room.mu.Unlock()
				sendJSON(conn, Message{Type: "error", Message: "Room pleine"})
				continue
			}
			if room.Started {
				room.mu.Unlock()
				sendJSON(conn, Message{Type: "error", Message: "Partie déjà en cours"})
				continue
			}

			room.Players = append(room.Players, player)
			currentRoom = room

			sendJSON(conn, Message{
				Type:     "room_joined",
				Room:     room.Code,
				PlayerID: player.ID,
			})

			// If 2 players, start!
			if len(room.Players) == 2 {
				room.Started = true
				room.mu.Unlock()

				for _, p := range room.Players {
					sendJSON(p.Conn, Message{Type: "game_start", Room: room.Code})
				}
				log.Printf("Game started in room %s", room.Code)
			} else {
				room.mu.Unlock()
			}

		case "move":
			if currentRoom == nil {
				continue
			}
			// Broadcast player state to opponent
			currentRoom.mu.Lock()
			for _, p := range currentRoom.Players {
				if p.ID != player.ID {
					// The client handles the game logic locally,
					// we just relay the move direction so the opponent's
					// display stays in sync via state updates
					sendJSON(p.Conn, Message{
						Type:      "opponent_move",
						Direction: msg.Direction,
						PlayerID:  player.ID,
					})
				}
			}
			currentRoom.mu.Unlock()

		case "state_update":
			// Player sends their current grid state
			if currentRoom == nil {
				continue
			}
			currentRoom.mu.Lock()
			for _, p := range currentRoom.Players {
				if p.ID != player.ID {
					sendJSON(p.Conn, Message{
						Type:  "opponent_state",
						Grid:  msg.Grid,
						Score: msg.Score,
					})
				}
			}
			currentRoom.mu.Unlock()

		case "game_won":
			if currentRoom == nil {
				continue
			}
			currentRoom.mu.Lock()
			for _, p := range currentRoom.Players {
				sendJSON(p.Conn, Message{
					Type:   "game_over",
					Winner: player.ID,
				})
			}
			currentRoom.mu.Unlock()
			log.Printf("Player %s won in room %s", player.ID, currentRoom.Code)
		}
	}
}

func handleDisconnect(room *Room, player *Player) {
	room.mu.Lock()
	defer room.mu.Unlock()

	// Notify other players
	for _, p := range room.Players {
		if p.ID != player.ID {
			sendJSON(p.Conn, Message{
				Type:    "error",
				Message: "L'adversaire s'est déconnecté",
			})
		}
	}

	// Clean up
	remaining := make([]*Player, 0)
	for _, p := range room.Players {
		if p.ID != player.ID {
			remaining = append(remaining, p)
		}
	}
	room.Players = remaining

	if len(room.Players) == 0 {
		go removeRoom(room.Code)
	}
}

func sendJSON(conn *websocket.Conn, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	conn.WriteMessage(websocket.TextMessage, data)
}

// ═══════════════════════════════════════
//  MAIN
// ═══════════════════════════════════════

func main() {
	rand.Seed(time.Now().UnixNano())

	// Serve static files
	fs := http.FileServer(http.Dir("../"))
	http.Handle("/", fs)

	// WebSocket endpoint
	http.HandleFunc("/ws", handleWS)

	port := ":8080"
	fmt.Println("╔═══════════════════════════════════════╗")
	fmt.Println("║       2048 Royale — Server            ║")
	fmt.Println("╠═══════════════════════════════════════╣")
	fmt.Printf("║  🌐 http://localhost%s             ║\n", port)
	fmt.Printf("║  🔌 ws://localhost%s/ws             ║\n", port)
	fmt.Println("║                                       ║")
	fmt.Println("║  Partage l'URL avec ton adversaire !  ║")
	fmt.Println("╚═══════════════════════════════════════╝")

	log.Fatal(http.ListenAndServe(port, nil))
}
