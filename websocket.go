package gameserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type Conn struct {
	*websocket.Conn
}

func (c Conn) String() string {
	return fmt.Sprintf("%s%p%s", blueColor, c.Conn, resetColor)
}

var (
	connectedUsers   = make(map[int][]Conn)
	connectedUsersMu sync.Mutex
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections with a null origin (for local file testing)
		origin := r.Header.Get("Origin")
		return origin == "" || origin == "null" || allowedOrigins[origin]
	},
}

type WebSocketMessage struct {
	GameID  int    `json:"game_id"`
	Token   Token  `json:"token"`
	Type    string `json:"message_type,omitempty"`
	Message string `json:"message,omitempty"`
}

// TODO: add logging for websocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Printf("Handling websocket connection from %s", r.RemoteAddr)
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade the connection: %v", err)
		return
	}
	conn := Conn{c}
	go listenForWebSocketMessages(conn)
}

// TODO: add error logging for websocket connections
func listenForWebSocketMessages(conn Conn) {
	defer conn.Close()

	for {
		messageType, messageData, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		switch messageType {
		case websocket.TextMessage:
			var message WebSocketMessage
			err := json.Unmarshal(messageData, &message)
			if err != nil {
				log.Printf("Error unmarshalling message for %s: %v", conn, err)
				return
			}
			playerType, token := validateGameToken(message.GameID, message.Token)
			if playerType == InvalidPlayer {
				log.Printf("Invalid game id or token for %s: %d %s", conn, message.GameID, message.Token)
				return
			}
			processMessage(conn, message, playerType, token)
		case websocket.BinaryMessage:
			log.Printf("Error: received non-supported binary message %s", messageData)
			return
		}
	}
}

func processMessage(conn Conn, message WebSocketMessage, playerType PlayerType, token Token) {
	switch message.Type {
	case "Join":
		game, err := GetGameWithId(message.GameID)
		if handleError(conn, message.GameID, err) {
			return
		}
		actions, err := getAllActions(message.GameID)
		if handleError(conn, message.GameID, err) {
			return
		}
		addConnection(message.GameID, conn)
		sendJSONMessage(conn, message.GameID, "GameJoined", map[string]interface{}{
			"player":       playerType.String(),
			"game_token":   token,
			"white_player": game.WhitePlayer,
			"black_player": game.BlackPlayer,
			"actions":      actions,
		})

	case "Action":
		var action Action
		err := json.Unmarshal([]byte(message.Message), &action)
		if err != nil {
			log.Printf("Error unmarshalling action message: %v", err)
			return
		}
		if handleError(conn, message.GameID, checkGameStatus(message.GameID)) {
			log.Printf("Game %d is not in progress", message.GameID)
			return
		}
		if handleError(conn, message.GameID, checkActionValidity(message.GameID, action.ActionNum)) {
			log.Printf("Invalid action number %d for game %d", action.ActionNum, message.GameID)
			return
		}
		// Save the action to the database
		if err := saveAction(message.GameID, action.ActionNum, action.Action, action.Signature); handleError(conn, message.GameID, err) {
			log.Printf("Error saving action: %v", err)
			return
		}
		broadcast(message.GameID, message)

	case "SendFullGame":
		if allActions, err := getAllActions(message.GameID); handleError(conn, message.GameID, err) {
			return
		} else {
			sendJSONMessage(conn, message.GameID, "FullGame", allActions)
		}

	case "RejectAction":
		broadcast(message.GameID, WebSocketMessage{GameID: message.GameID, Type: "GameOver", Message: "Rejected action"})
		if err := markGameAsFinished(message.GameID, "Rejected action detected"); err != nil {
			log.Printf("Error marking game as finished: %v", err)
		}
		return

	case "GameOver":
		broadcast(message.GameID, WebSocketMessage{GameID: message.GameID, Type: "GameOver", Message: message.Message})
		if err := markGameAsFinished(message.GameID, message.Message); err != nil {
			log.Printf("Error marking game as finished: %v", err)
		}

	default:
		sendJSONMessage(conn, message.GameID, "Error", fmt.Sprintf("Unknown message type %s", message.Type))
	}
}

func addConnection(gameID int, conn Conn) {
	connectedUsersMu.Lock()
	connectedUsers[gameID] = append(connectedUsers[gameID], conn)
	connectedUsersMu.Unlock()
}

// handleError checks if there is an error and sends an appropriate JSON message. Returns true if there was an error.
func handleError(conn Conn, gameID int, err error) bool {
	if err != nil {
		sendJSONMessage(conn, gameID, "Error", err.Error())
		return true
	}
	return false
}

func sendJSONMessage(conn Conn, gameId int, messageType string, data any) error {
	prettyJson, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Error marshalling JSON: %v", err)
		return err
	}
	err = conn.WriteJSON(WebSocketMessage{GameID: gameId, Type: messageType, Message: string(prettyJson)})
	if err != nil {
		log.Printf("Error sending JSON message: %v", err)
		return err
	}
	return nil
}

func broadcast(gameID int, action WebSocketMessage) {
	connectedUsersMu.Lock()
	defer connectedUsersMu.Unlock()

	var activeConnections []Conn

	for _, conn := range connectedUsers[gameID] {
		err := conn.WriteJSON(action)
		if err != nil {
			log.Printf("Failed to send action to conn %s: %v", conn, err)
			conn.Close() // Close the failed connection
		} else {
			activeConnections = append(activeConnections, conn)
		}
	}

	connectedUsers[gameID] = activeConnections

	if len(connectedUsers[gameID]) == 0 {
		delete(connectedUsers, gameID)
	}
}
