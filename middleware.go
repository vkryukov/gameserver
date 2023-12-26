// middleware.go implements the logging and CORS middleware for the game server.
//
// The logging middleware is used to do the following things:
// - Assign a UUID to each request
// - Log the request information (method, URL, params, body)
// - Log the response information (status code, body)
// - Additionally, log all database queries and their execution time
//
// It also provides a function to start a goroutine that periodically prints the logs to stdout,
// maintaing the correct order of the log lines (e.g., first the request, then all the db queries, then the response)
// even when there are multiple goroutines running.
//
// The CORS middleware is used to allow requests from the frontend to the backend for the development server.
package gameserver

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Saving to database

var logDb *sql.DB

func InitLogDB(path string) error {
	var err error
	logDb, err = sql.Open("sqlite3", setupPath(path))
	if err != nil {
		return err
	}
	_, err = logDb.Exec(`
	CREATE TABLE IF NOT EXISTS requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT, 
		timestamp INTEGER DEFAULT (strftime('%s', 'now')),
		endpoint TEXT,
		method TEXT,
		params TEXT,
		body TEXT
	);

	CREATE TABLE IF NOT EXISTS responses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT,
		timestamp INTEGER DEFAULT (strftime('%s', 'now')),
		status_code INTEGER,
		body TEXT
	);

	CREATE TABLE IF NOT EXISTS queries (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		uuid TEXT,
		timestamp INTEGER DEFAULT (strftime('%s', 'now')),
		query TEXT,
		params TEXT,
		duration INTEGER
	);
	`)

	if err == nil {
		fmt.Printf("Created tables\n")
		rows, err := logDb.Query("SELECT name FROM sqlite_master WHERE type='table'")
		if err != nil {
			log.Fatalf("Error querying tables: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			rows.Scan(&name)
			log.Printf("Found table: %s", name)
		}
	} else {
		log.Fatalf("Error creating tables: %v", err)
	}

	return err
}

func CloseLogDB() error {
	return logDb.Close()
}

// Logging middleware

type config struct {
	EnableCORS    bool
	EnableLogging bool
}

var _config config

func SetMiddlewareConfig(enableCors bool, enableLogging bool) {
	_config = config{enableCors, enableLogging}
}

type contextKey string

type loggingResponseWriter struct {
	requestID uuid.UUID
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (lrw *loggingResponseWriter) Write(b []byte) (int, error) {
	lrw.body.Write(b)
	return lrw.ResponseWriter.Write(b)
}

func (lrw *loggingResponseWriter) save() error {
	_, err := logDb.Exec("INSERT INTO responses(uuid, status_code, body) VALUES(?, ?, ?)",
		lrw.requestID, lrw.statusCode, lrw.body.String())
	return err
}

func (lrw *loggingResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}

func loggingMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	if !_config.EnableLogging {
		return handler
	}

	return func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New()
		ctx := context.WithValue(r.Context(), contextKey("requestID"), requestID)
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("Error reading request body: %v", err)
		}
		err = r.Body.Close()
		if err != nil {
			log.Printf("Error closing request body: %v", err)
		}
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		bodyString := string(bodyBytes)
		_, err = logDb.Exec("INSERT INTO requests(uuid, endpoint, method, params, body) VALUES(?, ?, ?, ?, ?)",
			requestID.String(), r.URL.Path, r.Method, r.URL.Query().Encode(), bodyString)
		if err != nil {
			log.Printf("Error logging request: %v", err)
		}

		lrw := &loggingResponseWriter{requestID, w, http.StatusOK, bytes.Buffer{}}
		handler(lrw, r.WithContext(ctx))

		err = lrw.save()
		if err != nil {
			log.Printf("Error logging response: %v", err)
		}
	}
}

// CORS middleware

var allowedOrigins = map[string]bool{
	"http://localhost:8080": true,
}

func EnableCors(handler http.HandlerFunc) http.HandlerFunc {
	if !_config.EnableCORS {
		return handler
	}

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

// all middleware
func Middleware(handler http.HandlerFunc) http.HandlerFunc {
	return loggingMiddleware(EnableCors(handler))
}
