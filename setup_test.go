package gameserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vkryukov/gameserver"
)

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)

}

var (
	port    string
	baseURL string
	srv     http.Server
	ws      *websocket.Conn
)

func setup() {
	if err := gameserver.InitDB(":memory:"); err != nil {
		log.Fatalf("Failed to initialize DB: %v", err)
	}
	gameserver.SetMailServer(&gameserver.MockEmailSender{})
	gameserver.SetMiddlewareConfig(true, false)
	gameserver.StartPrintingLog(time.Second)
	if err := gameserver.InitLogDB(":memory:"); err != nil {
		log.Fatalf("Failed to initialize log DB: %v", err)
	}
	port = ":1234"
	baseURL = "http://localhost" + port
	srv = http.Server{
		Addr:    port,
		Handler: nil,
	}
	gameserver.RegisterAuthHandlers("/auth", baseURL)
	gameserver.RegisterGameHandlers("/game")
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()

	ws = newWSConnection()
}

func newWSConnection() *websocket.Conn {
	u := url.URL{Scheme: "ws", Host: "localhost:1234", Path: "/game/ws"}
	var err error
	ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	return ws
}

func teardown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer ws.Close()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown: %v", err)
	}
	if err := gameserver.CloseDB(); err != nil {
		log.Fatalf("Failed to close DB: %v", err)
	}
	if err := gameserver.CloseLogDB(); err != nil {
		log.Fatalf("Failed to close log DB: %v", err)
	}
}

func postRequestWithBody(t *testing.T, url string, body []byte) []byte {
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("Failed to post userReq: %v", err)
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	return body
}

func postObject(t *testing.T, url string, obj any) []byte {
	body, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("Failed to marshal userReq: %v", err)
	}

	return postRequestWithBody(t, url, body)
}

func mustDecodeRequestWithObject(t *testing.T, url string, obj any, target any) {
	resp := postObject(t, url, obj)
	err := json.Unmarshal(resp, target)
	if err != nil {
		t.Fatalf("Failed to unmarshal response %q: %v", string(resp), err)
	}
}

type errorResponse struct {
	Error string `json:"error"`
}

func isErrorResponse(resp []byte, substring string) bool {
	// Return true if the response is an error response and contains the given substring.
	var response errorResponse
	err := json.Unmarshal(resp, &response)
	if err != nil {
		return false
	}
	return response.Error != "" && strings.Contains(response.Error, substring)
}

func mustPrettyPrint(t *testing.T, obj any) string {
	pretty, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		t.Fatalf("Failed to pretty print object %v: %v", obj, err)
	}
	return string(pretty)
}
