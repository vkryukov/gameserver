package gameserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

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
)

func setup() {
	gameserver.InitDB(":memory:")
	gameserver.SetMailServer(&gameserver.MockEmailSender{})
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
}

func teardown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown: %v", err)
	}
	gameserver.CloseDB()
}

func mustRegisterUser(t *testing.T, email string, password string, screenName string) *gameserver.User {
	userReq := &gameserver.User{Email: email, Password: password, ScreenName: screenName}
	_, err := gameserver.RegisterUser(userReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}
	user, err := gameserver.GetUserWithEmail(email)
	if err != nil {
		t.Fatalf("Failed to get user: %v", err)
	}
	if user.Email != email {
		t.Fatalf("Registered user has wrong email: %s", user.Email)
	}
	if user.EmailVerified {
		t.Fatalf("Newly registered user has a verified email")
	}
	return user
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
