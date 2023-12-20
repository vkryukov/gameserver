package gameserver_test

import (
	"encoding/json"
	"testing"

	"github.com/vkryukov/gameserver"
)

func TestGameCreation(t *testing.T) {

	email := "test-game@example.com"
	password := "game-user-password"
	screenName := "Test Game User"

	// Test 1: can create a game with a valid player
	mustRegisterUser(t, email, password, screenName)

	game, err := gameserver.CreateGame(&gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		BlackPlayer: "",
		Public:      false,
	})
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}
	if game.WhitePlayer != screenName {
		t.Fatalf("Created game has wrong white player: %s", game.WhitePlayer)
	}
	if game.BlackPlayer != "" {
		t.Fatalf("Created game has wrong black player: %s", game.BlackPlayer)
	}

	if game.WhiteToken == "" || game.BlackToken == "" {
		t.Fatalf("Created game has an empty token: white/black/viewer: %q/%q", game.WhiteToken, game.BlackToken)
	}
	if game.ViewerToken != "" {
		t.Fatalf("Created non-public game with a viewer token: %q", game.ViewerToken)
	}

	// Test 2: Can create a game with a starting position
	game, err = gameserver.CreateGame(&gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		Public:      false,
		GameRecord:  "a b c d e",
	})
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}
	if game.GameRecord != "a b c d e" || game.NumActions != 5 {
		t.Fatalf("Created game has wrong game record: %s", game.GameRecord)
	}

	// Test 3: same but with a http handler
	resp := postObject(t, "http://localhost:1234/game/create", &gameserver.Game{
		Type:        "Gipf",
		BlackPlayer: screenName,
		Public:      true,
		GameRecord:  "h i j k",
	})
	var game2 gameserver.Game

	err = json.Unmarshal(resp, &game2)
	if err != nil {
		t.Fatalf("Failed to unmarshal response %q: %v", string(resp), err)
	}
	if game.Id == game2.Id {
		t.Fatalf("Created game has same id: %d", game.Id)
	}
	if game2.GameRecord != "h i j k" || game2.NumActions != 4 || game2.Public != true || game2.ViewerToken == "" {
		t.Fatalf("Created game has wrong game record: %s", mustPrettyPrint(t, game2))
	}
}
