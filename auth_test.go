package gameserver_test

import (
	"testing"

	gs "github.com/vkryukov/gameserver"
)

// MockEmailSender implements EmailSender for testing purposes.
type MockEmailSender struct {
	To      string
	Subject string
	Body    string
}

func (s *MockEmailSender) Send(to, subject, body string) error {
	s.To = to
	s.Subject = subject
	s.Body = body
	return nil
}

// Tests
func TestBasicRegistrationAndAuthentication(t *testing.T) {
	mockMailServer := &MockEmailSender{}
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

	// Test 4: after registering a user, it can be authenticated with the right password
	authenticatedUser, err := gs.AuthenticateUser(userReq)
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if authenticatedUser.Email != registeredUser.Email {
		t.Fatalf("Authenticated user has wrong email: %s", authenticatedUser.Email)
	}

	// Test 5: after registering a user, it cannot be authenticated with the wrong password
	userReq.Password = "wrong password"
	_, err = gs.AuthenticateUser(userReq)
	if err == nil {
		t.Fatalf("Expected error when authenticating user with wrong password, got nil")
	}
}
