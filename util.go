package gameserver

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

var allowedOrigins = map[string]bool{
	"http://localhost:8080": true,
	"https://playgipf.com":  true,
}

func enableCors(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if strings.HasPrefix(origin, "http://localhost") || origin == "" || origin == "null" || allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			handler(w, r)
		} else {
			log.Printf("CORS origin not allowed: %s", origin)
			http.Error(w, "CORS origin not allowed", http.StatusForbidden)
		}
	}
}

func writeJSONResponse(w http.ResponseWriter, response interface{}) {
	jsonResponse, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("Sending JSON response:\n%s%s%s", Cyan, string(jsonResponse), Reset)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResponse)
}
