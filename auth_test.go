package gameserver_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/vkryukov/gameserver"
)

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
	return user
}

// Test user 1
var testEmail = "test-register@example.com"
var testPassword = "register-user-password"
var testScreenName = "Test Register User"

func TestBasicRegistrationAndAuthentication(t *testing.T) {
	testUser := mustRegisterUser(t, testEmail, testPassword, testScreenName)
	// Test 1: after registering a user, it can be found with getUserWithEmail
	foundUser1, err := gameserver.GetUserWithEmail(testEmail)
	if err != nil || foundUser1.Email != testEmail {
		t.Fatalf("Failed to find user with GetUserWithEmail: %v", err)
	}

	// Test 2: after registering a user, emailExists should return true.
	if !gameserver.EmailExists(testEmail) {
		t.Fatalf("emailExists returned false after registering user")
	}

	// Test 3: after registering a user, another one cannot be registered with the same email.
	_, err = gameserver.RegisterUser(testUser)
	if err == nil {
		t.Fatalf("Expected error when registering user with duplicate email, got nil")
	}

	// Test 4: after registering a user, it can be authenticated with the right password
	authenticatedUser, err := gameserver.AuthenticateUser(&gameserver.User{Email: testEmail, Password: testPassword})
	if err != nil {
		t.Fatalf("Failed to authenticate user: %v", err)
	}
	if authenticatedUser.Email != testEmail {
		t.Fatalf("Authenticated user has wrong email: %s", authenticatedUser.Email)
	}

	// Test 5: after registering a user, it cannot be authenticated with the wrong password
	_, err = gameserver.AuthenticateUser(&gameserver.User{Email: testEmail, Password: "wrong password"})
	if err == nil {
		t.Fatalf("Expected error when authenticating user with wrong password, got nil")
	}

	// Test 6: after registering a user, the email is not verified
	if testUser.EmailVerified {
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
	return response.Error != "" && strings.Contains(response.Error, substring)
}

func TestLoginAndCheckHandler(t *testing.T) {
	// Test 1: registered user can be authenticated with right password
	resp := postUserRequest(t, "http://localhost:1234/auth/login", &gameserver.User{Email: testEmail, Password: testPassword})
	var responseUser gameserver.User
	err := json.Unmarshal(resp, &responseUser)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if responseUser.Email != testEmail {
		t.Fatalf("Authenticated user has wrong email: %s", responseUser.Email)
	}

	// Test 1.1: we can get the user with the token
	user, err := gameserver.GetUserWithToken(responseUser.Token)
	if err != nil {
		t.Fatalf("Failed to get user with token: %v", err)
	}
	if user.Email != testEmail {
		t.Fatalf("Authenticated user has wrong email: %s", user.Email)
	}
	if user.Token == "" {
		t.Fatalf("Authenticated user has empty token")
	}

	// Test 1.2: we can check the user with the token
	resp = postRequestWithBody(t, "http://localhost:1234/auth/check?token="+string(user.Token), []byte(""))
	err = json.Unmarshal(resp, &responseUser)
	if err != nil {
		t.Fatalf("Failed to unmarshal response '%s': %v", resp, err)
	}
	if responseUser.Email != testEmail {
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
	resp = postUserRequest(t, "http://localhost:1234/auth/login", &gameserver.User{Email: testEmail, Password: "wrong password"})
	if !isErrorResponse(resp, "wrong password") {
		t.Fatalf("Expected error when authenticating unregistered user, got %s", resp)
	}

	// Test 3: unregistered user cannot be authenticated
	resp = postUserRequest(t, "http://localhost:1234/auth/login", &gameserver.User{Email: "user-doesnt-exist@example.com", Password: "password"})
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

func TestEmailVerification(t *testing.T) {
	mockMailServer := &gameserver.MockEmailSender{}
	gameserver.SetMailServer(mockMailServer)

	testEmailUser := "test-email@example.com"
	testEmailPassword := "email-user-password"
	testEmailScreenName := "Test Email User"

	mustRegisterUser(t, testEmailUser, testEmailPassword, testEmailScreenName)

	// Test 1: we can verify the user with the verification URL

	// Finding a verification token in the email.
	verificationUrlRx := regexp.MustCompile(`(https?://.*/auth/verify\?token=[a-f0-9]+)[\s\n]`)
	matches := verificationUrlRx.FindStringSubmatch(mockMailServer.Body)
	if len(matches) != 2 {
		t.Fatalf("Failed to find verification URL in email body: %s", mockMailServer.Body)
	}
	verificationUrl := matches[1]

	// Visit the verification URL, and make sure there is no error.
	resp := postRequestWithBody(t, verificationUrl, []byte(""))
	log.Printf("resp = %s", resp)
	if isErrorResponse(resp, "") {
		t.Fatalf("Failed to verify email: %s", resp)
	}

	resp = postUserRequest(t, "http://localhost:1234/auth/login", &gameserver.User{Email: testEmailUser, Password: testEmailPassword})
	var responseUser gameserver.User
	if isErrorResponse(resp, "") {
		t.Fatalf("Failed log in again: %s", resp)
	}
	err := json.Unmarshal(resp, &responseUser)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if responseUser.Email != testEmailUser {
		t.Fatalf("Authenticated user has wrong email: %s", responseUser.Email)
	}
	if !responseUser.EmailVerified {
		t.Fatalf("Authenticated user has unverified email")
	}

}
