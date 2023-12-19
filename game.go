package gameserver

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSockets
func RegisterGameHandlers(prefix string) {
	http.HandleFunc(prefix+"/ws", EnableCors(handleWebSocket))
	log.Printf("Handling websocket connections at %s%s/ws", baseURL, prefix)
	http.HandleFunc(prefix+"/newgame", EnableCors(createNewGameHandler))
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
		game, err := GetGameById(message.GameID)
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

// Game

type Game struct {
	ID           int    `json:"id"`
	Type         string `json:"type"`
	WhitePlayer  string `json:"white_user"`
	BlackPlayer  string `json:"black_user"`
	WhiteToken   Token  `json:"white_token"`
	BlackToken   Token  `json:"black_token"`
	ViewerToken  Token  `json:"viewer_token"`
	GameOver     bool   `json:"game_over"`
	GameResult   string `json:"game_result"`
	CreationTime int    `json:"creation_time"`
	NumActions   int    `json:"num_actions"`
	GameRecord   string `json:"game_record"`
	Public       bool   `json:"public"`
}

func GetGameById(id int) (*Game, error) {
	query := `
		SELECT 
			g.id, g.type, u1.screen_name, u2.screen_name, g.white_token, g.black_token, g.viewer_token, g.game_over, g.game_result, g.creation_time, g.public,
			COUNT(a.action_num) AS num_actions, 
			COALESCE(GROUP_CONCAT(a.action ORDER BY a.creation_time, ' '), '')  AS game_record
		FROM games g
		LEFT JOIN users u1 ON g.white_user_id = u1.id
		LEFT JOIN users u2 ON g.black_user_id = u2.id
		LEFT JOIN actions a ON g.id = a.game_id
		WHERE g.id = ?
		GROUP BY g.id
	`
	var game Game
	var whiteUser, blackUser sql.NullString
	var creationTime float64

	err := db.QueryRow(query, id).Scan(&game.ID, &game.Type, &whiteUser, &blackUser, &game.WhiteToken, &game.BlackToken, &game.ViewerToken,
		&game.GameOver, &game.GameResult, &creationTime, &game.Public, &game.NumActions, &game.GameRecord)
	if err != nil {
		return nil, err
	}
	game.CreationTime = int(creationTime)

	if whiteUser.Valid {
		game.WhitePlayer = whiteUser.String
	}
	if blackUser.Valid {
		game.BlackPlayer = blackUser.String
	}

	return &game, nil
}

func CreateGame(request *Game) (*Game, error) {
	var whiteToken, blackToken, viewerToken Token

	if request.WhitePlayer != "" {
		_, err := getUserIDFromUsername(request.WhitePlayer)
		if err != nil {
			return nil, err
		}
	}

	if request.BlackPlayer != "" {
		_, err := getUserIDFromUsername(request.BlackPlayer)
		if err != nil {
			return nil, err
		}
	}

	whiteToken = generateToken()
	blackToken = generateToken()
	if request.Public {
		viewerToken = generateToken()
	}

	var whiteUserID, blackUserID int
	var err error
	if request.WhitePlayer == "" {
		whiteUserID = -1
	} else {
		whiteUserID, err = getUserIDFromUsername(request.WhitePlayer)
		if err != nil {
			return nil, err
		}
	}
	if request.BlackPlayer == "" {
		blackUserID = -1
	} else {
		blackUserID, err = getUserIDFromUsername(request.BlackPlayer)
		if err != nil {
			return nil, err
		}
	}

	res, err := db.Exec(
		"INSERT INTO games(type, white_user_id, black_user_id, white_token, black_token, viewer_token) VALUES(?, ?, ?, ?, ?, ?)",
		request.Type, whiteUserID, blackUserID, whiteToken, blackToken, viewerToken)
	if err != nil {
		return nil, err
	}

	gameID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Game{
		ID:          int(gameID),
		WhiteToken:  whiteToken,
		BlackToken:  blackToken,
		ViewerToken: viewerToken,
		WhitePlayer: request.WhitePlayer,
		BlackPlayer: request.BlackPlayer,
		Type:        request.Type,
		Public:      request.Public,
	}, nil
}

func createNewGameHandler(w http.ResponseWriter, r *http.Request) {
	// extract from request body
	var request Game
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// create new game
	newGame, err := CreateGame(&request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, newGame)
}
