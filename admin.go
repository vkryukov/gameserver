package gameserver

import (
	"log"
	"net/http"
)

func RegisterAdminHandlers(prefix, baseURL string) {
	http.HandleFunc(baseURL+prefix+"/users", Middleware(handleListUsers))
	http.HandleFunc(baseURL+prefix+"/games", Middleware(handleListGames))
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := listUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, users)
}

func handleListGames(w http.ResponseWriter, r *http.Request) {
	games, err := listGames()
	log.Printf("Games: %v", games)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSONResponse(w, games)
}

func ExecuteSQL(sql string, args ...interface{}) error {
	_, err := db.Exec(sql, args...)
	return err
}
