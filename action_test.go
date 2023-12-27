package gameserver_test

import (
	"testing"

	"github.com/vkryukov/gameserver"
)

func TestNumberOfActionsForNewGame(t *testing.T) {
	user1 := mustRegisterAndAuthenticateUser(t, "user1-new-game-creating@example.com", "user1-ws-password", "user1-actiongame1")
	user2 := mustRegisterAndAuthenticateUser(t, "user2-new-game-creating@example.com", "user2-ws-password", "user2-actiongame2")
	game1 := mustCreateGame(t, user1, true, false)
	mustJoinGame(t, user2, game1)

	num, err := gameserver.GetNumberOfActions(game1.Id)
	if err != nil {
		t.Fatalf("Failed to get number of actions: %v", err)
	}
	if num != 0 {
		t.Fatalf("Number of actions for new game is not 0 but %d", num)
	}
}
