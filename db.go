// db.go contains all the database related functions.

package gameserver

import (
	"database/sql"
	"fmt"
	"sort"

	_ "github.com/mattn/go-sqlite3"
)

// Database initialization

var db *sql.DB

// InitDB initializes the database. User :memory: to use an in-memory database.
func setupPath(path string) string {
	var prefix string
	if path == ":memory:" {
		prefix = path
	} else {
		prefix = "file:" + path
	}
	return prefix + "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000"
}

func InitDB(path string) error {
	var err error
	db, err = sql.Open("sqlite3", setupPath(path))
	if err != nil {
		return err
	}

	sqlStmt := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE,
		email_verified INTEGER DEFAULT 0,
		password_hash TEXT,
		screen_name TEXT UNIQUE,
		is_admin INTEGER DEFAULT 0,
		creation_time REAL DEFAULT ((julianday('now') - 2440587.5)*86400000)
	);

	CREATE TABLE IF NOT EXISTS tokens (
		user_id INTEGER,
		token TEXT,
		creation_time REAL DEFAULT ((julianday('now') - 2440587.5)*86400000),
		PRIMARY KEY (user_id, token), 
		FOREIGN KEY (user_id) REFERENCES users(user_id)
	);

    CREATE TABLE IF NOT EXISTS games (
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		type TEXT, -- type of the game (such as Gipf, ...)
		starting_position TEXT,

		-- white_user_id is the foreign key  of the user who playes as white
		-- black_user_id is the id of the user who playes as black
		-- white_user_id and black_user_id can be null if the game is played by a guest
		white_user_id INTEGER DEFAULT -1,
		black_user_id INTEGER DEFAULT -1,

		white_token TEXT,
		black_token TEXT,
		viewer_token TEXT,
		game_over INTEGER DEFAULT 0,
		game_result TEXT DEFAULT "",
		creation_time REAL DEFAULT ((julianday('now') - 2440587.5)*86400000)
	);

	CREATE TABLE IF NOT EXISTS actions (
		game_id INTEGER, 
		-- the number of the action in the sequence (starting from 1)
		action_num INTEGER,
		action TEXT,
		-- an MD5 hash of the (game_id, action_num, player_key, action), calculated by the client, for client integrity verification
		action_signature TEXT, 
		creation_time REAL DEFAULT ((julianday('now') - 2440587.5)*86400000), 
		PRIMARY KEY (game_id, action_num)
	);
    `
	_, err = db.Exec(sqlStmt)
	return err
}

func CloseDB() error {
	_, err := db.Exec("PRAGMA wal_checkpoint;")
	if err != nil {
		return fmt.Errorf("Error executing PRAGMA wal_checkpoint: %v", err)
	}
	return db.Close()
}

type PlayerType int

const (
	WhitePlayer PlayerType = iota
	BlackPlayer
	Viewer
	InvalidPlayer
)

func (p PlayerType) String() string {
	switch p {
	case WhitePlayer:
		return "white"
	case BlackPlayer:
		return "black"
	case Viewer:
		return "viewer"
	default:
		return "invalid"
	}
}

// validateGameToken checks if the given token is valid player token for the given game, and returns the player type and the game token.
// Note: the game token is not necessarily the same as the given token (which could just help identify the user).
//
//	The token is valid if:
//	a) the token is either the white token or the black token, or
//	b) the token belongs to the user who is playing as white or black in the game, or
//	c) the token is the viewer token, or
//	d) the viewer token associated with the game is "", which means that the game is public and anyone can view it.
func validateGameToken(gameID int, token Token) (PlayerType, Token) {
	var whiteToken, blackToken, viewerToken Token
	var whiteUserID, blackUserID int
	err := db.QueryRow(
		"SELECT white_token, black_token, viewer_token, white_user_id, black_user_id FROM games WHERE id = ?",
		gameID).Scan(&whiteToken, &blackToken, &viewerToken, &whiteUserID, &blackUserID)
	if err != nil {
		return InvalidPlayer, "" // the game does not exist
	}
	if token != "" && token == whiteToken {
		return WhitePlayer, whiteToken
	} else if token != "" && token == blackToken {
		return BlackPlayer, blackToken
	}

	var userID int
	err = db.QueryRow("SELECT user_id FROM tokens WHERE token = ?", token).Scan(&userID)
	if err == nil {
		if userID == whiteUserID {
			return WhitePlayer, whiteToken
		} else if userID == blackUserID {
			return BlackPlayer, blackToken
		}
	}
	if token == viewerToken && viewerToken != "" {
		return Viewer, viewerToken
	}
	return InvalidPlayer, ""
}

func getUserIDFromScreenName(screenName string) (int, error) {
	var userID int
	err := db.QueryRow("SELECT id FROM users WHERE screen_name = ?", screenName).Scan(&userID)
	if err != nil {
		return -1, err
	}
	return userID, nil
}

func markGameAsFinished(gameID int, result string) error {
	_, err := db.Exec("UPDATE games SET game_over = 1, game_result = ? WHERE id = ?", result, gameID)
	return err
}

// checkGameStatus checks the game's status and returns an error if the game is finished or other issues are found.
func checkGameStatus(gameID int) error {
	var gameOver int
	err := db.QueryRow("SELECT game_over FROM games WHERE id = ?", gameID).Scan(&gameOver)
	if err != nil {
		return err
	}
	if gameOver == 1 {
		return fmt.Errorf("game is over")
	}
	return nil
}

// Server administration

// User is a struct that represents a user in the database.

func listUsers() ([]*User, error) {
	query := `
    SELECT u.id, u.email, u.screen_name, u.creation_time, t.token
    FROM users u
    LEFT JOIN (
        SELECT token, user_id
        FROM tokens
        ORDER BY id DESC
        LIMIT 1
    ) t ON u.id = t.user_id
	ORDER BY u.created_time DESC
`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var token sql.NullString
		var user User
		var creationTime float64

		if err := rows.Scan(&user.Id, &user.Email, &user.ScreenName, &creationTime, &token); err != nil {
			return nil, err
		}
		user.CreationTime = int(creationTime)
		users = append(users, &user)

	}
	return users, nil
}

func listGames() ([]Game, error) {
	query := `
		SELECT 
			g.id, g.type, u1.username, u2.username, g.white_token, g.black_token, g.viewer_token, g.game_over, g.game_result, g.creation_time,
			COUNT(a.action_num) AS num_actions, 
            COALESCE(GROUP_CONCAT(a.action ORDER BY a.creation_time, ', '), '')  AS game_record
		FROM games g
		LEFT JOIN users u1 ON g.white_user_id = u1.id
		LEFT JOIN users u2 ON g.black_user_id = u2.id
		LEFT JOIN actions a ON g.id = a.game_id
		GROUP BY g.id
	`
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	games := make([]Game, 0)
	for rows.Next() {
		var game Game
		var whiteUser, blackUser sql.NullString
		var creationTime float64

		if err := rows.Scan(&game.Id, &game.Type, &whiteUser, &blackUser, &game.WhiteToken, &game.BlackToken, &game.ViewerToken,
			&game.GameOver, &game.GameResult, &creationTime, &game.NumActions, &game.GameRecord); err != nil {
			return nil, err
		}
		game.CreationTime = int(creationTime)

		if whiteUser.Valid {
			game.WhitePlayer = whiteUser.String
		}
		if blackUser.Valid {
			game.BlackPlayer = blackUser.String
		}

		games = append(games, game)
	}

	sort.Slice(games, func(i, j int) bool {
		return games[i].CreationTime > games[j].CreationTime
	})

	return games, nil
}
