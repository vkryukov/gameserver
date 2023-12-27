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

func TestJoiningNewGame(t *testing.T) {
	user1 := mustRegisterAndAuthenticateUser(t, "user1-ws-joining@example.com", "user1-ws-password", "user1-ws")
	user2 := mustRegisterAndAuthenticateUser(t, "user2-ws-joining@example.com", "user2-ws-password", "user2-ws")
	game1 := mustCreateGame(t, user1, true, false)
	mustJoinGame(t, user2, game1)
	mustSendWSMessage(t, &gameserver.WebSocketMessage{GameID: game1.Id, Token: user1.Token, Type: "Join"})
	resp := mustReadWSMessage(t)
	fmt.Println(resp.Type)
	if resp.Type == "Error" {
		t.Fatalf("Received error message: %v", resp.Message)
	}
}
