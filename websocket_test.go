package gameserver_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/vkryukov/gameserver"
)

func mustSendWSMessage(t *testing.T, wsm *gameserver.WebSocketMessage) {
	data, err := json.Marshal(wsm)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}
	err = ws.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
}

func mustReadWSMessage(t *testing.T) *gameserver.WebSocketMessage {
	messageType, message, err := ws.ReadMessage()
	if messageType != websocket.TextMessage {
		t.Fatalf("Expected text message, got %v", messageType)
	}
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}
	var wsm gameserver.WebSocketMessage
	err = json.Unmarshal(message, &wsm)
	if err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}
	return &wsm
}

// mustMakeAction sends a move and returns both the immediate response and the broadcasted response
func mustMakeAction(t *testing.T, user *gameserver.User, game *gameserver.Game, move string, num int) *gameserver.WebSocketMessage {
	action := &gameserver.Action{ActionNum: num, Action: move}
	data, err := json.Marshal(action)
	if err != nil {
		t.Fatalf("Failed to marshal action: %v", err)
	}
	mustSendWSMessage(t, &gameserver.WebSocketMessage{GameID: game.Id, Token: user.Token, Type: "Action", Message: string(data)})
	fmt.Println("Sent move")
	r1 := mustReadWSMessage(t)
	fmt.Println("Received immediate response")
	if r1.Type == "Error" {
		t.Fatalf("Received error message: %v", r1.Message)
	}
	return r1
}

func TestJoiningNewGame(t *testing.T) {
	user1 := mustRegisterAndAuthenticateRandomUser(t)
	user2 := mustRegisterAndAuthenticateRandomUser(t)
	game1 := mustCreateGame(t, user1, true, true)
	mustJoinGame(t, user2, game1)
	mustSendWSMessage(t, &gameserver.WebSocketMessage{GameID: game1.Id, Token: user1.Token, Type: "Join"})
	resp := mustReadWSMessage(t)
	fmt.Println(resp.Type)
	if resp.Type == "Error" {
		t.Fatalf("Received error message: %v", resp.Message)
	}

	// Test 1: we can send moves and receive responses

	r1 := mustMakeAction(t, user1, game1, "a", 1)
	if r1.Type != "Action" {
		t.Fatalf("Expected Action, got %v", r1.Type)
	}
	r2 := mustMakeAction(t, user2, game1, "b", 2)
	if r2.Type != "Action" {
		t.Fatalf("Expected Action, got %v", r2.Type)
	}

	// Test 2: the num actions is adjusted correctly
	var games []*gameserver.Game
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/byuser", struct{ Token gameserver.Token }{user1.Token}, &games)
	if len(games) != 1 {
		t.Fatalf("Expected 1 game, got %d", len(games))
	}
	if games[0].NumActions != 2 {
		t.Fatalf("Expected 2 actions, got %d", games[0].NumActions)
	}
}
