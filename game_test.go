package gameserver_test

import (
	"encoding/json"
	"log"
	"testing"

	"github.com/vkryukov/gameserver"
)

func createGameWithRequest(t *testing.T, req *gameserver.Game) *gameserver.Game {
	resp := postObject(t, "http://localhost:1234/game/create", req)
	var game gameserver.Game

	err := json.Unmarshal(resp, &game)
	if err != nil {
		t.Fatalf("Failed to unmarshal response %q: %v", string(resp), err)
	}
	return &game
}

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
	game2 := createGameWithRequest(t, &gameserver.Game{
		Type:        "Gipf",
		BlackPlayer: screenName,
		Public:      true,
		GameRecord:  "h i j k",
	})
	if game.Id == game2.Id {
		t.Fatalf("Created game has same id: %d", game.Id)
	}
	if game2.GameRecord != "h i j k" || game2.NumActions != 4 || game2.Public != true || game2.ViewerToken == "" {
		t.Fatalf("Created game has wrong game record: %s", mustPrettyPrint(t, game2))
	}
	if game2.WhiteToken != "" {
		t.Fatalf("Response to a black player has white token visible: %q", game2.WhiteToken)
	}

	// Test 4: a game created by white player doesn't see a black token
	game3 := createGameWithRequest(t, &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		Public:      true,
	})
	if game3.BlackToken != "" {
		t.Fatalf("Response to a white player has black token visible: %q", game3.BlackToken)
	}

	// Test 5: cannot create a game with the same player
	_, err = gameserver.CreateGame(&gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		BlackPlayer: screenName,
		Public:      false,
	})
	if err == nil {
		t.Fatalf("Expected error when creating game with the same player, got nil")
	}

	// Test 6: creating a game with a non-existing player fails
	resp := postObject(t, "http://localhost:1234/game/create", &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: "non-existing",
		Public:      false,
	})
	if !isErrorResponse(resp, "cannot create") {
		t.Fatalf("Expected error when creating game with a non-existing player, got %s", resp)
	}

	// Test 7: create a game with nonsense request fails
	resp = postRequestWithBody(t, "http://localhost:1234/game/create", []byte("nonsense"))
	if !isErrorResponse(resp, "incorrect request") {
		t.Fatalf("Expected error when creating game with a nonsense request, got %s", resp)
	}

}

func TestListingUserGames(t *testing.T) {
	email := "user-listing-games@example.com"
	password := "user-listing-games-password"
	screenName := "User Listing Games"

	mustRegisterUser(t, email, password, screenName)
	user := mustAuthenticateUser(t, email, password)

	game1 := createGameWithRequest(t, &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		Public:      false,
	})
	game2 := createGameWithRequest(t, &gameserver.Game{
		Type:        "Standard Gipf",
		BlackPlayer: screenName,
		Public:      false,
		GameRecord:  "one two three",
	})
	game3 := createGameWithRequest(t, &gameserver.Game{
		Type:        "Basic Gipf",
		WhitePlayer: screenName,
		Public:      true,
	})

	// Test1: a list of active games contains all games
	log.Printf("token: %q", user.Token)
	resp := postObject(t, "http://localhost:1234/game/list", struct{ Token gameserver.Token }{
		Token: user.Token,
	})
	var games []*gameserver.Game
	err := json.Unmarshal(resp, &games)
	if err != nil {
		t.Fatalf("Failed to unmarshal response %q: %v", string(resp), err)
	}
	if len(games) != 3 {
		t.Fatalf("Expected 3 games, got %d", len(games))
	}
	if games[0].Id != game1.Id || games[1].Id != game2.Id || games[2].Id != game3.Id {
		t.Fatalf("Expected games %d, %d, %d, got %d, %d, %d", game1.Id, game2.Id, game3.Id, games[0].Id, games[1].Id, games[2].Id)
	}

	// Test 2: we should have precisely 4 non-empty tokens: 3 for the user and 1 for the viewer
	if games[0].WhiteToken == "" || games[1].BlackToken == "" || games[2].WhiteToken == "" {
		t.Fatalf("Expected non-empty tokens for the user, got %q, %q, %q", games[0].WhiteToken, games[1].BlackToken, games[2].WhiteToken)
	}
	if games[2].ViewerToken == "" {
		t.Fatalf("Expected non-empty viewer token for game 2")
	}
	if games[0].ViewerToken != "" || games[1].ViewerToken != "" {
		t.Fatalf("Expected empty viewer tokens for games 0 and 1")
	}
	if games[0].BlackToken != "" || games[1].WhiteToken != "" || games[2].BlackToken != "" {
		t.Fatalf("Expected empty other player tokens for games 0, 1 and 2")
	}
}
