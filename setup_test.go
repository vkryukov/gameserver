package gameserver_test

import (
	"context"
	"log"
	"net/http"
	"os"
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
	gameserver.SetMailServer(&MockEmailSender{})
	port = ":1234"
	baseURL = "http://localhost" + port
	srv = http.Server{
		Addr:    port,
		Handler: nil,
	}
	gameserver.RegisterAuthHandlers("/auth", baseURL)
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
}
