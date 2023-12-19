package gameserver_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vkryukov/gameserver"
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

func TestBasicRegistrationAndAuthentication(t *testing.T) {
	mockMailServer := &MockEmailSender{}
	gameserver.SetMailServer(mockMailServer)

	userReq := &gameserver.User{Email: "test@example.com", Password: "password"}

	// Test 1: after registering a user, it can be found with getUserWithToken and getUserWithEmail
	registeredUser, err := gameserver.RegisterUser(userReq)
	if err != nil {
		t.Fatalf("Failed to register user: %v", err)
	}

	foundUser, err := gameserver.GetUserWithEmail(userReq.Email)
	if err != nil || foundUser.Email != registeredUser.Email {
		t.Fatalf("Failed to find user with getUserWithEmail: %v", err)
	}

	if err != nil || foundUser.Email != registeredUser.Email {
		t.Fatalf("Failed to find user with getUserWithToken: %v", err)
	}

	// Test 2: after registering a user, emailExists should return true.
	if !gameserver.EmailExists(userReq.Email) {
		t.Fatalf("emailExists returned false after registering user")
	}

	// Test 3: after registering a user, another one cannot be registered with the same email.
	_, err = gameserver.RegisterUser(userReq)
	if err == nil {
		t.Fatalf("Expected error when registering user with duplicate email, got nil")
	}

	// Test 4: after registering a user, it can be authenticated with the right password
	authenticatedUser, err := gameserver.AuthenticateUser(userReq)
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if authenticatedUser.Email != registeredUser.Email {
		t.Fatalf("Authenticated user has wrong email: %s", authenticatedUser.Email)
	}

	// Test 5: after registering a user, it cannot be authenticated with the wrong password
	userReq.Password = "wrong password"
	_, err = gameserver.AuthenticateUser(userReq)
	if err == nil {
		t.Fatalf("Expected error when authenticating user with wrong password, got nil")
	}

	// Test 6: after registering a user, the email is not verified
	if registeredUser.EmailVerified {
		t.Fatalf("Registered user has verified email")
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

// postUserRequest encodes a userReq as JSON and posts it to the given url.
// It receives a JSON response, which should be equal to expectedResponse's string representation as JSON.
func postUserRequest(t *testing.T, url string, userReq *gameserver.User) []byte {
	body, err := json.Marshal(userReq)
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
	return strings.Contains(response.Error, substring)
}

func TestLoginAndCheckHandler(t *testing.T) {
	mockMailServer := &MockEmailSender{}
	gameserver.SetMailServer(mockMailServer)

	userReq := &gameserver.User{
		Email:    "test@example.com",
		Password: "password",
	}

	// Test 1: registered user can be authenticated with right password
	resp := postUserRequest(t, "http://localhost:1234/auth/login", userReq)
	var responseUser gameserver.User
	err := json.Unmarshal(resp, &responseUser)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if responseUser.Email != userReq.Email {
		t.Fatalf("Authenticated user has wrong email: %s", responseUser.Email)
	}

	// Test 1.1: we can get the user with the token
	user, err := gameserver.GetUserWithToken(responseUser.Token)
	if err != nil {
		t.Fatalf("Failed to get user with token: %v", err)
	}
	if user.Email != userReq.Email {
		t.Fatalf("Authenticated user has wrong email: %s", user.Email)
	}
	if user.Token == "" {
		t.Fatalf("Authenticated user has empty token")
	}

	// Test 1.2: we can check the user with the token
	fmt.Println("Test 1.2: user.Token", user.Token)
	resp = postRequestWithBody(t, "http://localhost:1234/auth/check?token="+string(user.Token), []byte(""))
	err = json.Unmarshal(resp, &responseUser)
	if err != nil {
		t.Fatalf("Failed to unmarshal response '%s': %v", resp, err)
	}
	if responseUser.Email != userReq.Email {
		t.Fatalf("Authenticated user has wrong email: %s", responseUser.Email)
	}
	if responseUser.Token != user.Token {
		t.Fatalf("Authenticated user has wrong token: '%s' insteasd of '%s'", responseUser.Token, user.Token)
	}

	// Test 1.3: we can't check the user with the wrong token
	resp = postRequestWithBody(t, "http://localhost:1234/auth/check?token=wrong-token", []byte(""))
	if !isErrorResponse(resp, "token not found") {
		t.Fatalf("Expected error when checking user with wrong token, got '%s'", resp)
	}

	// Test 2: registered user cannot be authenticated with wrong password
	userReq.Password = "wrong password"
	resp = postUserRequest(t, "http://localhost:1234/auth/login", userReq)
	if !isErrorResponse(resp, "wrong password") {
		t.Fatalf("Expected error when authenticating unregistered user, got %s", resp)
	}

	// Test 3: unregistered user cannot be authenticated
	userReq.Email = "test1@example.com"
	userReq.Password = "password"
	resp = postUserRequest(t, "http://localhost:1234/auth/login", userReq)
	if !isErrorResponse(resp, "not found") {
		t.Fatalf("Expected error when authenticating unregistered user, got %s", resp)
	}

	// Test 4: sending a request with an empty body returns an error
	resp = postUserRequest(t, "http://localhost:1234/auth/login", &gameserver.User{})
	if !isErrorResponse(resp, "missing email") {
		t.Fatalf("Expected error when authenticating with emtpy body, got %s", resp)
	}

	// Test 5: sending a request with an empty password returns an error
	resp = postUserRequest(t, "http://localhost:1234/auth/login", &gameserver.User{Email: "test@example.com"})
	if !isErrorResponse(resp, "missing password") {
		t.Fatalf("Expected error when authenticating with emtpy password, got %s", resp)
	}

	// Test 6: sending a request with wrong body returns an error
	resp = postRequestWithBody(t, "http://localhost:1234/auth/login", []byte("wrong body"))
	if !isErrorResponse(resp, "invalid character") {
		t.Fatalf("Expected error when authenticating with wrong body, got %s", resp)
	}
}
