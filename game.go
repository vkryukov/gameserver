package gameserver

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// WebSockets
func RegisterGameHandlers(prefix string) {
	http.HandleFunc(prefix+"/ws", EnableCors(handleWebSocket))
	http.HandleFunc(prefix+"/create", Middleware(createGameHandler))
	http.HandleFunc(prefix+"/list/byuser", Middleware(listGamesByUserHandler))
	http.HandleFunc(prefix+"/list/joinable", Middleware(joinableGamesHandler))
	http.HandleFunc(prefix+"/join", Middleware(joinGameHandler))
	http.HandleFunc(prefix+"/cancel", Middleware(cancelGameHandler))
}

// Game

type Game struct {
	Id           int    `json:"id"`
	Type         string `json:"type"`
	WhitePlayer  string `json:"white_player"`
	BlackPlayer  string `json:"black_player"`
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

func GetGameWithId(id int) (*Game, error) {
	query := `
		SELECT 
			g.id, g.type, u1.screen_name, u2.screen_name, g.white_token, g.black_token, g.viewer_token, g.game_over, g.game_result, g.creation_time
		FROM games g
		LEFT JOIN users u1 ON g.white_user_id = u1.id
		LEFT JOIN users u2 ON g.black_user_id = u2.id
		WHERE g.id = ?
		GROUP BY g.id
	`
	var game Game
	var whiteUser, blackUser sql.NullString
	var creationTime float64

	err := db.QueryRow(query, id).Scan(&game.Id, &game.Type, &whiteUser, &blackUser, &game.WhiteToken, &game.BlackToken, &game.ViewerToken,
		&game.GameOver, &game.GameResult, &creationTime)
	if err != nil {
		return nil, err
	}
	game.CreationTime = int(creationTime)
	if game.ViewerToken == "" {
		game.Public = true
	}

	if whiteUser.Valid {
		game.WhitePlayer = whiteUser.String
	}
	if blackUser.Valid {
		game.BlackPlayer = blackUser.String
	}

	// Get the game record
	gameRecord, numActions, err := GetGameRecord(id)
	if err != nil {
		return nil, err
	}
	game.GameRecord = gameRecord
	game.NumActions = numActions

	return &game, nil
}

func GetGameRecord(gameID int) (string, int, error) {
	var actions []string
	rows, err := db.Query("SELECT action FROM actions WHERE game_id = ? ORDER BY action_num", gameID)
	if err != nil {
		return "", 0, err
	}
	for rows.Next() {
		var action string
		if err := rows.Scan(&action); err != nil {
			return "", 0, err
		}
		actions = append(actions, action)
	}
	if err := rows.Close(); err != nil {
		return "", 0, err
	}
	return strings.Join(actions, " "), len(actions), nil
}

func CreateGame(request *Game) (*Game, error) {
	var whiteToken, blackToken, viewerToken Token

	if request.WhitePlayer != "" {
		_, err := getUserIDFromScreenName(request.WhitePlayer)
		if err != nil {
			return nil, err
		}
		if tokenMismatchUser(request.WhitePlayer, request.WhiteToken) {
			return nil, fmt.Errorf("incorrect token for white player")
		}
	}

	if request.BlackPlayer != "" {
		_, err := getUserIDFromScreenName(request.BlackPlayer)
		if err != nil {
			return nil, err
		}
		if tokenMismatchUser(request.BlackPlayer, request.BlackToken) {
			return nil, fmt.Errorf("incorrect token for black player")
		}
	}

	if request.WhitePlayer == request.BlackPlayer {
		return nil, fmt.Errorf("white and black players cannot be the same")
	}

	whiteToken = generateToken()
	blackToken = generateToken()
	if !request.Public {
		viewerToken = generateToken()
	}

	var whiteUserID, blackUserID int
	var err error
	if request.WhitePlayer == "" {
		whiteUserID = -1
	} else {
		whiteUserID, err = getUserIDFromScreenName(request.WhitePlayer)
		if err != nil {
			return nil, err
		}
	}
	if request.BlackPlayer == "" {
		blackUserID = -1
	} else {
		blackUserID, err = getUserIDFromScreenName(request.BlackPlayer)
		if err != nil {
			return nil, err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}

	res, err := tx.Exec(
		"INSERT INTO games(type, white_user_id, black_user_id, white_token, black_token, viewer_token) VALUES(?, ?, ?, ?, ?, ?)",
		request.Type, whiteUserID, blackUserID, whiteToken, blackToken, viewerToken)
	if err != nil {
		return nil, err
	}
	gameID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	actions := strings.Split(request.GameRecord, " ")
	for i, action := range actions {
		_, err := tx.Exec("INSERT INTO actions(game_id, action_num, action) VALUES(?, ?, ?)", gameID, i+1, action)
		if err != nil {
			log.Printf("error inserting action %d: %v", i+1, err)
			tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return GetGameWithId(int(gameID))
}

func tokenMismatchUser(screenName string, token Token) bool {
	user, err := GetUserWithToken(token)
	if err != nil {
		return true
	}
	return user.ScreenName != screenName
}

func createGameHandler(w http.ResponseWriter, r *http.Request) {
	// extract from request body
	var request Game
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		sendError(w, serverError("incorrect request", err))
		return
	}

	// create new game
	newGame, err := CreateGame(&request)
	if err != nil {
		sendError(w, serverError("cannot create a new game", err))
		return
	}

	// We only want to return the tokens for the player who created the game and possibly the viewer token.
	if request.BlackPlayer == "" {
		newGame.BlackToken = ""
	}
	if request.WhitePlayer == "" {
		newGame.WhiteToken = ""
	}

	writeJSONResponse(w, newGame)
}

func listGamesByUserHandler(w http.ResponseWriter, r *http.Request) {
	user := extractUserFromRequest(w, r)
	if user == nil {
		return
	}

	// get games
	games, err := listGamesByUser(user)
	if err != nil {
		sendError(w, serverError("cannot list games", err))
		return
	}

	// We only want to return the tokens for the player who created the game and possibly the viewer token.
	for _, game := range games {
		if game.WhitePlayer != user.ScreenName {
			game.WhiteToken = ""
		}
		if game.BlackPlayer != user.ScreenName {
			game.BlackToken = ""
		}
	}

	writeJSONResponse(w, games)
}

func joinableGamesHandler(w http.ResponseWriter, r *http.Request) {
	user := extractUserFromRequest(w, r)
	if user == nil {
		return
	}

	// get games
	games, err := joinableGamesByUser(user)
	if err != nil {
		sendError(w, serverError("cannot list games", err))
		return
	}

	for _, game := range games {
		game.WhiteToken = ""
		game.BlackToken = ""
	}

	writeJSONResponse(w, games)
}

func extractUserFromRequest(w http.ResponseWriter, r *http.Request) *User {
	var request struct {
		Token Token `json:"token"`
	}
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		sendError(w, serverError("incorrect request", err))
		return nil
	}

	user, err := GetUserWithToken(request.Token)
	if err != nil {
		sendError(w, serverError("incorrect token", err))
		return nil
	}

	return user
}

func listGamesByUser(user *User) ([]*Game, error) {
	query := `
		SELECT 
			g.id, g.type, u1.screen_name, u2.screen_name, g.white_token, g.black_token, g.viewer_token, g.game_over, g.game_result, g.creation_time
		FROM games g
		LEFT JOIN users u1 ON g.white_user_id = u1.id
		LEFT JOIN users u2 ON g.black_user_id = u2.id
		WHERE g.white_user_id = ? OR g.black_user_id = ?
		GROUP BY g.id
		ORDER BY (g.white_user_id == -1 OR g.black_user_id == -1)
	`
	return getGamesWithQuery(query, user.Id, user.Id)
}

func joinableGamesByUser(user *User) ([]*Game, error) {
	query := `
		SELECT 
			g.id, g.type, u1.screen_name, u2.screen_name, g.white_token, g.black_token, g.viewer_token, g.game_over, g.game_result, g.creation_time
		FROM games g
		LEFT JOIN users u1 ON g.white_user_id = u1.id
		LEFT JOIN users u2 ON g.black_user_id = u2.id
		WHERE 
			(g.white_user_id = -1 OR g.black_user_id = -1) 
			AND
			g.viewer_token = ''
			AND
			(g.white_user_id != ? AND g.black_user_id != ?)
		GROUP BY g.id
	`
	return getGamesWithQuery(query, user.Id, user.Id)
}

func getGamesWithQuery(query string, params ...any) ([]*Game, error) {
	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		var game Game
		var whiteUser, blackUser sql.NullString
		var creationTime float64

		err := rows.Scan(&game.Id, &game.Type, &whiteUser, &blackUser, &game.WhiteToken, &game.BlackToken, &game.ViewerToken,
			&game.GameOver, &game.GameResult, &creationTime)
		if err != nil {
			return nil, err
		}
		game.CreationTime = int(creationTime)
		if game.ViewerToken != "" {
			game.Public = true
		}

		if whiteUser.Valid {
			game.WhitePlayer = whiteUser.String
		}
		if blackUser.Valid {
			game.BlackPlayer = blackUser.String
		}

		games = append(games, &game)
	}

	return games, nil
}

func joinGameHandler(w http.ResponseWriter, r *http.Request) {
	// extract from request body
	var request struct {
		Id    int   `json:"id"`
		Token Token `json:"token"`
	}
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		sendError(w, serverError("incorrect request", err))
		return
	}

	// check that the game exists and is joinable
	game, err := GetGameWithId(request.Id)
	if err != nil {
		sendError(w, serverError("invalid game id", err))
		return
	}
	if game.WhitePlayer != "" && game.BlackPlayer != "" {
		sendError(w, serverError("game is full", nil))
		return
	}

	// check that the user with this token exists
	user, err := GetUserWithToken(request.Token)
	if err != nil {
		sendError(w, serverError("incorrect token", err))
		return
	}

	token := generateToken()

	// update the game
	err = updateGame(game, user.Id, token)
	if err != nil {
		sendError(w, serverError("cannot update game", err))
		return
	}

	// clear the other player token
	if game.WhitePlayer == "" {
		game.BlackToken = ""
		game.WhitePlayer = user.ScreenName
		game.WhiteToken = token
	} else {
		game.WhiteToken = ""
		game.BlackPlayer = user.ScreenName
		game.BlackToken = token
	}

	writeJSONResponse(w, game)
}

func updateGame(game *Game, userId int, token Token) error {
	var query string
	if game.WhitePlayer == "" {
		query = "UPDATE games SET white_user_id = ?, white_token = ? WHERE id = ?"
	} else if game.BlackPlayer == "" {
		query = "UPDATE games SET black_user_id = ?, black_token = ? WHERE id = ?"
	} else {
		return fmt.Errorf("game is full: %v", game)
	}
	_, err := db.Exec(query, userId, token, game.Id)
	return err
}

func cancelGameHandler(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Id    int   `json:"id"`
		Token Token `json:"token"`
	}
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		sendError(w, serverError("incorrect request", err))
		return
	}
	game, err := GetGameWithId(request.Id)
	if err != nil {
		sendError(w, serverError("invalid game id", err))
		return
	}
	player, _ := validateGameToken(request.Id, request.Token)
	if player == InvalidPlayer {
		sendError(w, serverError("invalid token", nil))
		return
	}
	if game.WhitePlayer != "" && game.BlackPlayer != "" {
		sendError(w, serverError("cannot cancel a game that has already started", nil))
		return
	}
	_, err = db.Exec("DELETE FROM games WHERE id = ?", request.Id)
	if err != nil {
		sendError(w, serverError("cannot delete game", err))
		return
	}
	writeJSONResponse(w, map[string]interface{}{"status": "game deleted successfully", "id": request.Id})
}
