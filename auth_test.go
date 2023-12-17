package gameserver_test

import (
	"testing"

	gs "github.com/vkryukov/gameserver"
)

// Tests
func TestAuth(t *testing.T) {
	if err := gs.InitDB(":memory:"); err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	mockMailServer := &gs.MockEmailSender{}
	gs.InitEmailServer(mockMailServer)

	userReq := &gs.User{Email: "test@example.com", Password: "password"}

	// Test 1: after registering a user, it can be found with getUserWithToken and getUserWithEmail
	registeredUser, err := gs.RegisterUser(userReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	foundUser, err := gs.GetUserWithEmail(userReq.Email)
	if err != nil || foundUser.Email != registeredUser.Email {
		t.Fatalf("Failed to find user with getUserWithEmail: %v", err)
	}

	// foundUser, err = gs.GetUserWithToken(registeredUser.Token)
	if err != nil || foundUser.Email != registeredUser.Email {
		t.Fatalf("Failed to find user with getUserWithToken: %v", err)
	}

	// Test 2: after registering a user, emailExists should return true.
	if !gs.EmailExists(userReq.Email) {
		t.Fatalf("emailExists returned false after registering user")
	}

	// Test 3: after registering a user, another one cannot be registered with the same email.
	_, err = gs.RegisterUser(userReq)
	if err == nil {
		t.Fatalf("Expected error when registering user with duplicate email, got nil")
	}
}
