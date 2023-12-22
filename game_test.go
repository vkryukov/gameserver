package gameserver_test

import (
	"testing"

	"github.com/vkryukov/gameserver"
)

func createGameWithRequest(t *testing.T, req *gameserver.Game) *gameserver.Game {
	var game gameserver.Game
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/create", req, &game)
	return &game
}

func TestGameCreation(t *testing.T) {
	email := "test-game@example.com"
	password := "game-user-password"
	screenName := "Test Game User"

	// Test 1: can create a game with a valid player
	user := mustRegisterAndAuthenticateUser(t, email, password, screenName)

	game, err := gameserver.CreateGame(&gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		WhiteToken:  user.Token,
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
		WhiteToken:  user.Token,
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
		BlackToken:  user.Token,
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
	if game2.BlackToken == "" {
		t.Fatalf("Response to a black player has no black token")
	}
	if game2.BlackToken == user.Token {
		t.Fatalf("Response to a black player has the same black token as the user")
	}

	// Test 4: a game created by white player doesn't see a black token
	game3 := createGameWithRequest(t, &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		WhiteToken:  user.Token,
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
		WhiteToken:  user.Token,
		Public:      false,
	})
	if err == nil {
		t.Fatalf("Expected error when creating game with the same player, got nil")
	}

	// Test 6: creating a game with a non-existing player fails
	resp := postObject(t, "http://localhost:1234/game/create", &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: "non-existing",
		WhiteToken:  user.Token,
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

	// Test 8: create a game with a wrong token fails
	resp = postObject(t, "http://localhost:1234/game/create", &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		WhiteToken:  "wrong-token",
		Public:      false,
	})
	if !isErrorResponse(resp, "cannot create") {
		t.Fatalf("Expected error when creating game with a wrong token, got %s", resp)
	}

	// Test 9: create a game with a correct token but wrong player fails
	resp = postObject(t, "http://localhost:1234/game/create", &gameserver.Game{
		Type:        "Gipf",
		WhitePlayer: screenName,
		BlackToken:  user.Token,
		Public:      false,
	})
	if !isErrorResponse(resp, "cannot create") {
		t.Fatalf("Expected error when creating game with a wrong token, got %s", resp)
	}

}

func mustCreateGame(t *testing.T, user *gameserver.User, isWhite bool, public bool) *gameserver.Game {
	gameReq := &gameserver.Game{
		Type:   "Gipf",
		Public: public,
	}
	if isWhite {
		gameReq.WhitePlayer = user.ScreenName
		gameReq.WhiteToken = user.Token
	} else {
		gameReq.BlackPlayer = user.ScreenName
		gameReq.BlackToken = user.Token
	}
	game, err := gameserver.CreateGame(gameReq)
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}
	return game

}

func TestListingUserGames(t *testing.T) {
	email := "user-listing-games@example.com"
	password := "user-listing-games-password"
	screenName := "User Listing Games"

	user := mustRegisterAndAuthenticateUser(t, email, password, screenName)

	game1 := mustCreateGame(t, user, true, false)
	game2 := mustCreateGame(t, user, false, false)
	game3 := mustCreateGame(t, user, true, true)

	// Test1: a list of active games contains all games
	var games []*gameserver.Game
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list", struct{ Token gameserver.Token }{
		Token: user.Token,
	}, &games)
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

func TestListPublicGames(t *testing.T) {
	// Delete all the games currently in the database
	err := gameserver.ExecuteSQL("DELETE FROM games")
	if err != nil {
		t.Fatalf("Failed to delete games: %v", err)
	}

	email1 := "user-public-games1@example.com"
	password1 := "user-public-games1-password"
	screenName1 := "User Public Games 1"

	email2 := "user-public-games2@example.com"
	password2 := "user-public-games2-password"
	screenName2 := "User Public Games 2"

	email3 := "user-public-games3@example.com"
	password3 := "user-public-games3-password"
	screenName3 := "User Public Games 3"

	user1 := mustRegisterAndAuthenticateUser(t, email1, password1, screenName1)
	user2 := mustRegisterAndAuthenticateUser(t, email2, password2, screenName2)
	user3 := mustRegisterAndAuthenticateUser(t, email3, password3, screenName3)

	// user1 has 2 public games
	mustCreateGame(t, user1, true, false)
	mustCreateGame(t, user1, false, true)
	mustCreateGame(t, user1, true, true)

	// user2 has 3 public games
	mustCreateGame(t, user2, false, true)
	mustCreateGame(t, user2, true, true)
	mustCreateGame(t, user2, true, true)
	mustCreateGame(t, user2, false, false)

	// user3 has 1 public game
	mustCreateGame(t, user3, false, true)
	mustCreateGame(t, user3, false, false)
	mustCreateGame(t, user3, true, false)

}
