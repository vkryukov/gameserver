package gameserver

import (
	"encoding/json"
	"net/http"
)

func writeJSONResponse(w http.ResponseWriter, response interface{}) {
	jsonResponse, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}
