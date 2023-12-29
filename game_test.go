package gameserver_test

import (
	"encoding/json"
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
	if game.ViewerToken == "" {
		t.Fatalf("Created non-public game with an empty viewer token: %q", game.ViewerToken)
	}
	if game.StartingPosition != "" {
		t.Fatalf("Created game has a non-empty starting position: %s", game.StartingPosition)
	}
	if game.Type != "Gipf" {
		t.Fatalf("Created game has a wrong type: %s", game.Type)
	}

	// Test 2: Can create a game with a starting position
	game, err = gameserver.CreateGame(&gameserver.Game{
		Type:             "Basic Gipf",
		WhitePlayer:      screenName,
		WhiteToken:       user.Token,
		Public:           false,
		StartingPosition: "a b c d e",
	})
	if err != nil {
		t.Fatalf("Failed to create game: %v", err)
	}
	if game.StartingPosition != "a b c d e" || game.NumActions != 0 || game.Type != "Basic Gipf" {
		t.Fatalf("Created game has wrong game record: %s", mustPrettyPrint(t, game))
	}

	// Test 3: same but with a http handler
	game2 := createGameWithRequest(t, &gameserver.Game{
		Type:             "Basic Gipf",
		BlackPlayer:      screenName,
		BlackToken:       user.Token,
		Public:           true,
		StartingPosition: "h i j k",
	})
	if game.Id == game2.Id {
		t.Fatalf("Created game has same id: %d", game.Id)
	}
	if game2.StartingPosition != "h i j k" || game2.NumActions != 0 || game2.Public != true || game2.ViewerToken != "" || game.Type != "Basic Gipf" {
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

func mustCreateGame(t *testing.T, user *gameserver.User, isWhite bool, isPublic bool) *gameserver.Game {
	gameReq := &gameserver.Game{
		Type:   "Gipf",
		Public: isPublic,
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
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/byuser", struct{ Token gameserver.Token }{
		Token: user.Token,
	}, &games)
	if len(games) != 3 {
		t.Fatalf("Expected 3 games, got %d", len(games))
	}
	if games[0].Id != game1.Id || games[1].Id != game2.Id || games[2].Id != game3.Id {
		t.Fatalf("Expected games %d, %d, %d, got %d, %d, %d", game1.Id, game2.Id, game3.Id, games[0].Id, games[1].Id, games[2].Id)
	}

	// Test 2: we should have precisely 4 non-empty tokens: 3 for the user and 2 for the viewers of non-public games
	if games[0].WhiteToken == "" || games[1].BlackToken == "" || games[2].WhiteToken == "" {
		t.Fatalf("Expected non-empty tokens for the user, got %q, %q, %q", games[0].WhiteToken, games[1].BlackToken, games[2].WhiteToken)
	}
	if games[2].ViewerToken != "" {
		t.Fatalf("Expected empty viewer token for game 2")
	}
	if games[0].ViewerToken == "" || games[1].ViewerToken == "" {
		t.Fatalf("Expected non-empty viewer tokens for games 0 and 1")
	}
	if games[0].BlackToken != "" || games[1].WhiteToken != "" || games[2].BlackToken != "" {
		t.Fatalf("Expected empty other player tokens for games 0, 1 and 2")
	}
	if games[0].Public || games[1].Public || !games[2].Public {
		t.Fatalf("Expected game 3 to be public, and games 1 and 2 to be non-public, but found: %v, %v, %v", games[0].Public, games[1].Public, games[2].Public)
	}
}

func areNonEmptyTokens(games []*gameserver.Game) bool {
	for _, game := range games {
		if game.WhiteToken != "" || game.BlackToken != "" || game.ViewerToken != "" {
			return true
		}
	}
	return false
}

func TestJoinableGames(t *testing.T) {
	// Delete all the games currently in the database
	err := gameserver.ExecuteSQL("DELETE FROM games")
	if err != nil {
		t.Fatalf("Failed to delete games: %v", err)
	}

	email1 := "user-joinable-games1@example.com"
	password1 := "user-joinable-games1-password"
	screenName1 := "User Joinable Games 1"

	email2 := "user-joinable-games2@example.com"
	password2 := "user-joinable-games2-password"
	screenName2 := "User Joinable Games 2"

	email3 := "user-joinable-games3@example.com"
	password3 := "user-joinable-games3-password"
	screenName3 := "User Joinable Games 3"

	user1 := mustRegisterAndAuthenticateUser(t, email1, password1, screenName1)
	user2 := mustRegisterAndAuthenticateUser(t, email2, password2, screenName2)
	user3 := mustRegisterAndAuthenticateUser(t, email3, password3, screenName3)

	// user1 has 2 public games
	mustCreateGame(t, user1, false, true)
	mustCreateGame(t, user1, true, true)
	mustCreateGame(t, user1, true, false)

	// user2 has 3 public games
	mustCreateGame(t, user2, false, true)
	mustCreateGame(t, user2, true, true)
	mustCreateGame(t, user2, true, true)
	mustCreateGame(t, user2, false, false)

	// user3 has 1 public game
	mustCreateGame(t, user3, false, true)
	mustCreateGame(t, user3, false, false)
	mustCreateGame(t, user3, true, false)

	var games []*gameserver.Game

	// Test 1: user1 sees 4 public games by other users
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/joinable", struct{ Token gameserver.Token }{user1.Token}, &games)
	if len(games) != 4 {
		t.Fatalf("Expected 4 public games, got %d", len(games))
	}
	if areNonEmptyTokens(games) {
		t.Fatalf("Expected empty tokens for all games, got %s", mustPrettyPrint(t, games))
	}

	// Test 2: user2 sees 3 public games by other users
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/joinable", struct{ Token gameserver.Token }{user2.Token}, &games)
	if len(games) != 3 {
		t.Fatalf("Expected 3 public games, got %d", len(games))
	}
	if areNonEmptyTokens(games) {
		t.Fatalf("Expected empty tokens for all games, got %s", mustPrettyPrint(t, games))
	}

	// Test 3: user3 sees 5 public games by other users
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/joinable", struct{ Token gameserver.Token }{user3.Token}, &games)
	if len(games) != 5 {
		t.Fatalf("Expected 5 public games, got %d", len(games))
	}
	if areNonEmptyTokens(games) {
		t.Fatalf("Expected empty tokens for all games, got %s", mustPrettyPrint(t, games))
	}
}

func joinGame(t *testing.T, user *gameserver.User, game *gameserver.Game) []byte {
	return postObject(t, "http://localhost:1234/game/join", map[string]interface{}{
		"id":    game.Id,
		"token": user.Token,
	})
}

func mustJoinGame(t *testing.T, user *gameserver.User, game *gameserver.Game) *gameserver.Game {
	resp := joinGame(t, user, game)
	if isErrorResponse(resp, "") {
		t.Fatalf("Cannot join a game: %s", resp)
	}
	var game2 gameserver.Game
	err := json.Unmarshal(resp, &game2)
	if err != nil {
		t.Fatalf("Failed to unmarshal response %q: %v", string(resp), err)
	}
	return &game2
}

func TestJoiningGame(t *testing.T) {
	// Delete all the games currently in the database
	err := gameserver.ExecuteSQL("DELETE FROM games")
	if err != nil {
		t.Fatalf("Failed to delete games: %v", err)
	}

	email1 := "user-joinining-games1@example.com"
	password1 := "user-joinining-games1-password"
	screenName1 := "User Joinining Games 1"

	email2 := "user-joinining-games2@example.com"
	password2 := "user-joinining-games2-password"
	screenName2 := "User Joinining Games 2"

	user1 := mustRegisterAndAuthenticateUser(t, email1, password1, screenName1)
	user2 := mustRegisterAndAuthenticateUser(t, email2, password2, screenName2)

	game1 := mustCreateGame(t, user1, true, true)
	game2 := mustCreateGame(t, user1, false, true)

	// Test 1: user2 can join game1 as black player
	game1joined := mustJoinGame(t, user2, game1)
	if game1joined.BlackPlayer != screenName2 {
		t.Fatalf("Joined game has wrong black player: %s", game1joined.BlackPlayer)
	}
	if game1joined.BlackToken == "" {
		t.Fatalf("Joined game has empty black token")
	}
	if game1joined.WhiteToken != "" {
		t.Fatalf("Joined game has non-empty white token")
	}
	if game1joined.BlackToken == user2.Token {
		t.Fatalf("Joined game has the same black token as the user")
	}

	// Test 2: user2 can join game2 as white player
	game2joined := mustJoinGame(t, user2, game2)
	if game2joined.WhitePlayer != screenName2 {
		t.Fatalf("Joined game has wrong white player: %s", game2joined.WhitePlayer)
	}
	if game2joined.WhiteToken == "" {
		t.Fatalf("Joined game has empty white token")
	}
	if game2joined.BlackToken != "" {
		t.Fatalf("Joined game has non-empty black token")
	}
	if game2joined.WhiteToken == user2.Token {
		t.Fatalf("Joined game has the same white token as the user")
	}

	// Test 3: cannot join an already joined game
	resp := joinGame(t, user2, game1)
	if !isErrorResponse(resp, "game is full") {
		t.Fatalf("Expected error when joining an already joined game, got %s", resp)
	}

	// Test 4: cannot join a non-public game
	// TODO: implement this test
	// If we create a private game, we *should* send the other player token to the user who created the game.
	// They can share that token with the other player, who can then join the game. At that point of time,
	// that "join" token will be replaced with the actual token of the other player, which won't be visible
	// to the first player.
}

func TestCancelGame(t *testing.T) {
	// Delete all the games currently in the database
	err := gameserver.ExecuteSQL("DELETE FROM games")
	if err != nil {
		t.Fatalf("Failed to delete games: %v", err)
	}

	email1 := "user-canceling-games1@example.com"
	password1 := "user-canceling-games1-password"
	screenName1 := "User canceling Games 1"

	email2 := "user-canceling-games2@example.com"
	password2 := "user-canceling-games2-password"
	screenName2 := "User canceling Games 2"

	user1 := mustRegisterAndAuthenticateUser(t, email1, password1, screenName1)
	user2 := mustRegisterAndAuthenticateUser(t, email2, password2, screenName2)

	game1 := mustCreateGame(t, user1, true, true)
	game2 := mustCreateGame(t, user2, false, true)

	var games []*gameserver.Game
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/byuser", struct{ Token gameserver.Token }{
		Token: user1.Token,
	}, &games)
	if len(games) != 1 {
		t.Fatalf("Expected 1 game, got %d", len(games))
	}

	// Test 1: user1 can cancel game1
	resp := postObject(t, "http://localhost:1234/game/cancel", map[string]interface{}{
		"id":    game1.Id,
		"token": user1.Token,
	})
	if isErrorResponse(resp, "") {
		t.Fatalf("Cannot cancel a game: %s", resp)
	}
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/byuser", struct{ Token gameserver.Token }{
		Token: user1.Token,
	}, &games)
	if len(games) != 0 {
		t.Fatalf("Expected 0 games after cancellation, got %d", len(games))
	}

	// Test 2: user1 cannot cancel game2
	resp = postObject(t, "http://localhost:1234/game/cancel", map[string]interface{}{
		"id":    game2.Id,
		"token": user1.Token,
	})
	if !isErrorResponse(resp, "invalid token") {
		t.Fatalf("Expected error when canceling a game by another user, got %s", resp)
	}
	mustDecodeRequestWithObject(t, "http://localhost:1234/game/list/byuser", struct{ Token gameserver.Token }{
		Token: user2.Token}, &games)
	if len(games) != 1 {
		t.Fatalf("Expected 1 game after cancellation, got %d", len(games))
	}

	// Test 3: cannot cancel a game that has started
	mustJoinGame(t, user1, game2)
	resp = postObject(t, "http://localhost:1234/game/cancel", map[string]interface{}{
		"id":    game2.Id,
		"token": user1.Token,
	})
	if !isErrorResponse(resp, "cannot cancel") {
		t.Fatalf("Expected error when canceling a game that has started, got %s", resp)
	}
	resp = postObject(t, "http://localhost:1234/game/cancel", map[string]interface{}{
		"id":    game2.Id,
		"token": user2.Token,
	})
	if !isErrorResponse(resp, "cannot cancel") {
		t.Fatalf("Expected error when canceling a game that has started, got %s", resp)
	}
}
