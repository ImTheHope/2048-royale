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

type Player struct {
	ID   string
	Conn *websocket.Conn
	mu   sync.Mutex
}

type Room struct {
	Code    string
	Players []*Player
	Started bool
	mu      sync.Mutex
}

type Message struct {
	Type     string     `json:"type"`
	Room     string     `json:"room,omitempty"`
	Dir      string     `json:"direction,omitempty"`
	PlayerID string     `json:"player_id,omitempty"`
	Grid     *[4][4]int `json:"grid,omitempty"`
	Score    *int       `json:"score,omitempty"`
	Winner   string     `json:"winner,omitempty"`
	Msg      string     `json:"message,omitempty"`
}

var (
	rooms    = make(map[string]*Room)
	roomsMu  sync.RWMutex
	upgrader = websocket.Upgrader{
		CheckOrigin:     func(r *http.Request) bool { return true },
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

func genCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 5)
	for i := range code {
		code[i] = chars[rand.Intn(len(chars))]
	}
	return string(code)
}

func genID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	id := make([]byte, 8)
	for i := range id {
		id[i] = chars[rand.Intn(len(chars))]
	}
	return string(id)
}

func getRoom(code string) *Room {
	roomsMu.RLock()
	defer roomsMu.RUnlock()
	return rooms[code]
}

func createRoom() *Room {
	roomsMu.Lock()
	defer roomsMu.Unlock()
	var code string
	for {
		code = genCode()
		if _, ok := rooms[code]; !ok {
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

func sendJSON(p *Player, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Conn.WriteMessage(websocket.TextMessage, data)
}

// Relay a message to the opponent(s) in the room
func relayToOthers(room *Room, senderID string, msg Message) {
	room.mu.Lock()
	defer room.mu.Unlock()
	for _, p := range room.Players {
		if p.ID != senderID {
			sendJSON(p, msg)
		}
	}
}

func broadcastAll(room *Room, msg Message) {
	room.mu.Lock()
	defer room.mu.Unlock()
	for _, p := range room.Players {
		sendJSON(p, msg)
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	player := &Player{ID: genID(), Conn: conn}
	var currentRoom *Room

	log.Printf("Player connected: %s", player.ID)

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Player %s disconnected", player.ID)
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
			sendJSON(player, Message{Type: "room_created", Room: room.Code, PlayerID: player.ID})

		case "join":
			code := strings.ToUpper(strings.TrimSpace(msg.Room))
			room := getRoom(code)
			if room == nil {
				sendJSON(player, Message{Type: "error", Msg: "Room introuvable"})
				continue
			}
			room.mu.Lock()
			if len(room.Players) >= 2 {
				room.mu.Unlock()
				sendJSON(player, Message{Type: "error", Msg: "Room pleine"})
				continue
			}
			if room.Started {
				room.mu.Unlock()
				sendJSON(player, Message{Type: "error", Msg: "Partie déjà en cours"})
				continue
			}
			room.Players = append(room.Players, player)
			currentRoom = room
			sendJSON(player, Message{Type: "room_joined", Room: room.Code, PlayerID: player.ID})

			if len(room.Players) == 2 {
				room.Started = true
				room.mu.Unlock()
				broadcastAll(room, Message{Type: "game_start", Room: room.Code})
				log.Printf("Game started in room %s", room.Code)
			} else {
				room.mu.Unlock()
			}

		case "state_update":
			if currentRoom == nil {
				continue
			}
			relayToOthers(currentRoom, player.ID, Message{
				Type:  "opponent_state",
				Grid:  msg.Grid,
				Score: msg.Score,
			})

		case "game_won":
			if currentRoom == nil {
				continue
			}
			broadcastAll(currentRoom, Message{Type: "game_over", Winner: player.ID})
			log.Printf("Player %s won in room %s", player.ID, currentRoom.Code)

		case "player_lost":
			if currentRoom == nil {
				continue
			}
			relayToOthers(currentRoom, player.ID, Message{
				Type:  "opponent_lost",
				Score: msg.Score,
			})

		// ─── RESTART PROTOCOL ───
		case "restart_request":
			if currentRoom != nil {
				relayToOthers(currentRoom, player.ID, Message{Type: "restart_request"})
			}

		case "restart_accept":
			if currentRoom != nil {
				relayToOthers(currentRoom, player.ID, Message{Type: "restart_accept"})
			}

		case "restart_reject":
			if currentRoom != nil {
				relayToOthers(currentRoom, player.ID, Message{Type: "restart_reject"})
			}
		}
	}
}

func handleDisconnect(room *Room, player *Player) {
	room.mu.Lock()
	defer room.mu.Unlock()
	for _, p := range room.Players {
		if p.ID != player.ID {
			sendJSON(p, Message{Type: "error", Msg: "L'adversaire s'est déconnecté"})
		}
	}
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

func main() {
	rand.Seed(time.Now().UnixNano())

	fs := http.FileServer(http.Dir("./"))
	http.Handle("/", fs)
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
