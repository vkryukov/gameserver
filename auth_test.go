package gameserver_test

import (
	"encoding/json"
	"log"
	"regexp"
	"testing"

	"github.com/vkryukov/gameserver"
)

// Test user 1
var testEmail = "test-register@example.com"
var testPassword = "register-user-password"
var testScreenName = "Test Register User"

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

func mustAuthenticateUser(t *testing.T, email string, password string) *gameserver.User {
	var user gameserver.User
	mustDecodeRequestWithObject(t, "http://localhost:1234/auth/login", &gameserver.User{Email: email, Password: password}, &user)
	if user.Email != email {
		t.Fatalf("Authenticated user has wrong email: %s", user.Email)
	}
	if user.Token == "" {
		t.Fatalf("Authenticated user has empty token")
	}
	return &user
}

func mustRegisterAndAuthenticateUser(t *testing.T, email string, password string, screenName string) *gameserver.User {
	mustRegisterUser(t, email, password, screenName)
	return mustAuthenticateUser(t, email, password)
}

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
	mustAuthenticateUser(t, testEmail, testPassword)

	// Test 5: after registering a user, it cannot be authenticated with the wrong password
	_, err = gameserver.AuthenticateUser(&gameserver.User{Email: testEmail, Password: "wrong password"})
	if err == nil {
		t.Fatalf("Expected error when authenticating user with wrong password, got nil")
	}

}

func TestLoginAndCheckHandler(t *testing.T) {
	// Test 1: registered user can be authenticated with right password
	var responseUser gameserver.User
	mustDecodeRequestWithObject(t, "http://localhost:1234/auth/login", &gameserver.User{Email: testEmail, Password: testPassword}, &responseUser)
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
	mustDecodeRequestWithObject(t, "http://localhost:1234/auth/check?token="+string(user.Token), []byte(""), &responseUser)
	if responseUser.Email != testEmail {
		t.Fatalf("Authenticated user has wrong email: %s", responseUser.Email)
	}
	if responseUser.Token != user.Token {
		t.Fatalf("Authenticated user has wrong token: '%s' insteasd of '%s'", responseUser.Token, user.Token)
	}

	// Test 1.3: we can't check the user with the wrong token
	resp := postRequestWithBody(t, "http://localhost:1234/auth/check?token=wrong-token", []byte(""))
	if !isErrorResponse(resp, "token not found") {
		t.Fatalf("Expected error when checking user with wrong token, got '%s'", resp)
	}

	// Test 2: registered user cannot be authenticated with wrong password
	resp = postObject(t, "http://localhost:1234/auth/login", &gameserver.User{Email: testEmail, Password: "wrong password"})
	if !isErrorResponse(resp, "wrong password") {
		t.Fatalf("Expected error when authenticating unregistered user, got %s", resp)
	}

	// Test 3: unregistered user cannot be authenticated
	resp = postObject(t, "http://localhost:1234/auth/login", &gameserver.User{Email: "user-doesnt-exist@example.com", Password: "password"})
	if !isErrorResponse(resp, "not found") {
		t.Fatalf("Expected error when authenticating unregistered user, got %s", resp)
	}

	// Test 4: sending a request with an empty body returns an error
	resp = postObject(t, "http://localhost:1234/auth/login", &gameserver.User{})
	if !isErrorResponse(resp, "missing email") {
		t.Fatalf("Expected error when authenticating with emtpy body, got %s", resp)
	}

	// Test 5: sending a request with an empty password returns an error
	resp = postObject(t, "http://localhost:1234/auth/login", &gameserver.User{Email: "test@example.com"})
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

	resp = postObject(t, "http://localhost:1234/auth/login", &gameserver.User{Email: testEmailUser, Password: testEmailPassword})
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

// TODO: Check that login/check responses do not leak the password hash or the user id.
