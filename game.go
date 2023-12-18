package gameserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSockets
func RegisterGameHandlers(prefix, baseURL string) {
	// TODO: probably shouldn't expose /newgame without a prefix
	http.HandleFunc(baseURL+prefix+"/ws", handleWebSocket)
	http.HandleFunc(baseURL+prefix+"/newgame", EnableCors(createNewGameHandler))
}

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
		log.Printf("Origin in WS upgrader: %s", origin)
		return origin == "" || origin == "null" || allowedOrigins[origin]
	},
}

type WebSocketMessage struct {
	GameID  int    `json:"game_id"`
	Token   Token  `json:"token"`
	Type    string `json:"message_type,omitempty"`
	Message string `json:"message,omitempty"`
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	log.Println("Handling websocket connection")
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade the connection: %v", err)
		return
	}
	conn := Conn{c}
	log.Printf("Upgraded connection %s", conn)
	go listenForWebSocketMessages(conn)
}

func listenForWebSocketMessages(conn Conn) {
	defer conn.Close()

	for {
		messageType, messageData, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}
		log.Printf("Received message from %s: %s", conn, messageData)

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
	log.Printf("Processing message from %v: %v", conn, message)
	switch message.Type {
	case "Join":
		log.Printf("Player %s joined game %d with token %s", playerType, message.GameID, message.Token)
		game, err := getGame(message.GameID)
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
			"white_player": game.WhiteUser,
			"black_player": game.BlackUser,
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
		log.Printf("Error: %v", err)
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
	log.Printf("Sending JSON message to conn=%s:\n%s%s%s", conn, cyanColor, prettyJson, resetColor)
	err = conn.WriteJSON(WebSocketMessage{GameID: gameId, Type: messageType, Message: string(prettyJson)})
	if err != nil {
		log.Printf("Error sending JSON message: %v", err)
		return err
	}
	return nil
}

func broadcast(gameID int, action WebSocketMessage) {
	log.Printf("Broadcasting action %v to game %d", action, gameID)
	connectedUsersMu.Lock()
	defer connectedUsersMu.Unlock()

	var activeConnections []Conn

	for _, conn := range connectedUsers[gameID] {
		log.Printf("Sending action to conn %s", conn)
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

func createNewGameHandler(w http.ResponseWriter, r *http.Request) {
	// extract NewGameRequest from request body
	var request NewGameRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// create new game
	newGame, err := createGame(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, newGame)
}
