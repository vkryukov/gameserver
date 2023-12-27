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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

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
		body TEXT,
		is_printed INTEGER DEFAULT 0
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

func StartPrintingLog(interval time.Duration) {
	go func() {
		currentTime := time.Now().Unix()
		for {
			time.Sleep(interval)
			rows, err := logDb.Query(`
			SELECT 
				rq.uuid, rq.timestamp, rq.endpoint, rq.method, rq.params, rq.body, rs.status_code, rs.body
			FROM 
				requests rq
			LEFT JOIN
				responses rs
			ON
				rq.uuid = rs.uuid
			WHERE
				rq.is_printed = 0 AND rs.id IS NOT NULL AND rq.timestamp > ?
			ORDER BY
				rq.timestamp ASC
			`, currentTime)
			if err != nil {
				log.Printf("Error querying requests: %v", err)
				return
			}
			defer func(rows *sql.Rows) {
				err := rows.Close()
				if err != nil {
					log.Printf("Error closing rows: %v", err)
				}
			}(rows)
			var uuids []string
			for rows.Next() {
				var (
					uuid         string
					timestamp    int64
					endpoint     string
					method       string
					params       string
					body         string
					statusCode   int
					responseBody string
				)
				if err := rows.Scan(&uuid, &timestamp, &endpoint, &method, &params, &body, &statusCode, &responseBody); err != nil {
					log.Printf("Error scanning row: %v", err)
					return
				}
				var paramsOrBody string
				if params != "" {
					paramsOrBody = cyanColor + "?" + params + resetColor
				} else {
					paramsOrBody = maybePrettyJSON(body)
				}
				var coloredStatusCode string
				if statusCode >= 200 && statusCode < 300 {
					coloredStatusCode = greenColor + fmt.Sprintf("%d", statusCode) + resetColor
				} else if statusCode >= 400 && statusCode < 500 {
					coloredStatusCode = redColor + fmt.Sprintf("%d", statusCode) + resetColor
				} else {
					coloredStatusCode = fmt.Sprintf("%d", statusCode)
				}
				fmt.Printf("%s%s%s\n%s %s%s\n=> %s%s\n\n",
					brightBlueColor, time.Unix(timestamp, 0).Format("2006-01-02 15:04:05"), resetColor,
					method, endpoint, paramsOrBody,
					coloredStatusCode, maybePrettyJSON(responseBody))
				uuids = append(uuids, uuid)

			}
			for _, uuid := range uuids {
				_, err = logDb.Exec("UPDATE requests SET is_printed = 1 WHERE uuid = ?", uuid)
				if err != nil {
					log.Printf("Error updating request: %v", err)
					return
				}
			}
		}
	}()
}

func maybePrettyJSON(s string) string {
	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		var prettyJSON bytes.Buffer
		err := json.Indent(&prettyJSON, []byte(s), "", "  ")
		if err != nil {
			return s
		}
		return "\n" + cyanColor + prettyJSON.String() + resetColor
	}
	if s == "null" {
		return "\n" + cyanColor + s + resetColor
	}
	return s
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
			http.Error(w, "CORS origin not allowed", http.StatusForbidden)
		}
	}
}

// all middleware
func Middleware(handler http.HandlerFunc) http.HandlerFunc {
	return loggingMiddleware(EnableCors(handler))
}
