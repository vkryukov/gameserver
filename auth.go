// auth provides authentication and registration services.

package gameserver

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"text/template"

	"database/sql"

	"golang.org/x/crypto/bcrypt"
)

var (
	handlerPrefix = "/auth"
	baseURL       = "http://localhost:8080"
)

func RegisterAuthHandlers(prefix, base string) {
	handlerPrefix = prefix
	baseURL = base
	http.HandleFunc(handlerPrefix+"/login", EnableCors(loginHandler))
	http.HandleFunc(handlerPrefix+"/check", EnableCors(checkHandler))
	http.HandleFunc(handlerPrefix+"/verify", EnableCors(verificationHandler))
	http.HandleFunc(handlerPrefix+"/register", EnableCors(registerUserHandler))
	http.HandleFunc(handlerPrefix+"/changepassword", EnableCors(changePasswordHandler))

	// We need to implement the following endpoints:
	// TODO: resend the verification email
	// TODO: reset the password
	// TODO: change the email address
}

// Tokens

type Token string

func generateToken() Token {
	b := make([]byte, 16)
	rand.Read(b)
	return Token(fmt.Sprintf("%x", b))
}

// Users

type User struct {
	Id            int    `json:"id,omitempty"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	ScreenName    string `json:"screen_name,omitempty"`
	Password      string `json:"password,omitempty"`
	NewPassword   string `json:"new_password,omitempty"`
	CreationTime  int    `json:"creation_time"`
	Token         Token  `json:"token"`
}

func GetUserWithToken(token Token) (*User, error) {
	// TODO: differentiate between a token not found and a general error.
	var user User
	var creationTime float64
	err := db.QueryRow(`
	SELECT users.id, users.email, users.email_verified, users.screen_name, users.password_hash, users.creation_time 
	FROM tokens 
	JOIN users ON tokens.user_id = users.id 
	WHERE tokens.token = ?
	`, token).Scan(&user.Id, &user.Email, &user.EmailVerified, &user.ScreenName, &user.Password, &creationTime)
	if err != nil {
		return nil, err
	}
	user.CreationTime = int(creationTime)
	return &user, nil
}

func GetUserWithEmail(email string) (*User, error) {
	// TODO: differentiate between a user not found and a general error.
	var user User
	var creationTime float64
	err := db.QueryRow(`
	SELECT id, email, email_verified, screen_name, password_hash, creation_time 
	FROM users 
	WHERE email = ?
	`, email).Scan(&user.Id, &user.Email, &user.EmailVerified, &user.ScreenName, &user.Password, &creationTime)
	if err != nil {
		return nil, err
	}
	user.CreationTime = int(creationTime)
	return &user, nil
}

type execer interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
}

func addNewTokenToUser(exec execer, userID int) (Token, error) {
	token := generateToken()
	_, err := exec.Exec("INSERT INTO tokens(user_id, token) VALUES(?, ?)", userID, token)
	return token, err
}

func AuthenticateUser(userReq *User) (*User, error) {
	if userReq.Email == "" {
		return nil, fmt.Errorf("missing email")
	}
	if userReq.Password == "" {
		return nil, fmt.Errorf("missing password")
	}
	user, err := GetUserWithEmail(userReq.Email)
	if err != nil {
		return nil, fmt.Errorf("user '%s' not found", userReq.Email)
	}
	if !comparePasswords(user.Password, userReq.Password) {
		return nil, fmt.Errorf("wrong password for user '%s'", userReq.Email)
	}
	return user, nil
}

func comparePasswords(hashedPwd string, plainPwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPwd), []byte(plainPwd)) == nil
}

func EmailExists(email string) bool {
	_, err := GetUserWithEmail(email)
	return err == nil
}

// serverError logs the detailed error and returns an error message to the client.
func serverError(message string, err error) error {
	log.Printf("Server error %s: %v", message, err)
	return fmt.Errorf("server: " + message)
}

func RegisterUser(userReq *User) (*User, error) {
	if userReq.Email == "" {
		return nil, fmt.Errorf("missing email")
	}
	if userReq.Password == "" {
		return nil, fmt.Errorf("missing password")
	}
	hashedPwd, err := bcrypt.GenerateFromPassword([]byte(userReq.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, serverError("cannot hash password", err)
	}
	if EmailExists(userReq.Email) {
		return nil, fmt.Errorf("email '%s' is already registered", userReq.Email)
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, serverError("cannot start transaction", err)
	}
	res, err := tx.Exec("INSERT INTO users(email, password_hash, screen_name) VALUES(?, ?, ?)", userReq.Email, hashedPwd, userReq.ScreenName)
	if err != nil {
		tx.Rollback()
		return nil, serverError("cannot insert user", err)
	}
	userID, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return nil, serverError("cannot get last insert ID", err)
	}
	verificationLink, err := createVerificationLink(tx, userID)
	if err != nil {
		tx.Rollback()
		return nil, serverError("cannot create verification link", err)
	}
	err = sendRegistrationEmail(userReq.Email, userReq.ScreenName, verificationLink)
	if err != nil {
		tx.Rollback()
		return nil, serverError("cannot send registration email; check email address", err)
	}
	err = tx.Commit()
	if err != nil {
		return nil, serverError("cannot commit transaction", err)
	}
	return &User{
		Email:         userReq.Email,
		ScreenName:    userReq.ScreenName,
		EmailVerified: false,
	}, nil
}

func createVerificationLink(exec execer, userID int64) (string, error) {
	token, err := addNewTokenToUser(exec, int(userID))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s/verify?token=%s", baseURL, handlerPrefix, token), nil
}

var emailTmpl *template.Template

// TODO: the specific template of the email should be configurable by the users of the library.
func init() {
	const emailTemplate = `Hello Gipf player,

Thank you for registering for our game server! Here are the details 
that we have recorded:
	- your email is {{.Email}}
	- your screen name is {{.ScreenName}}

IMPORTANT: Your email address is used to reset your password, and 
needs to be verified. Please click on the following link to verify it:

{{.VerificationLink}}

You cannot join games until you verify your email address.

If you did not register for our game server, please ignore this email.

Regards,
The Gipf Game Master.`

	emailTmpl = template.Must(template.New("email").Parse(emailTemplate))
}

func sendRegistrationEmail(email, screenName, verificationLink string) error {
	var buf bytes.Buffer
	if err := emailTmpl.Execute(&buf, struct {
		Email            string
		ScreenName       string
		VerificationLink string
	}{email, screenName, verificationLink}); err != nil {
		return fmt.Errorf("executing email template: %v", err)
	}
	return SendMessage(email, "Gipf Game Server Registration", buf.String())
}

func changePassword(userReq *User) (*User, error) {
	user, err := AuthenticateUser(userReq)
	if err != nil {
		return nil, err
	}
	newHashPwd, err := bcrypt.GenerateFromPassword([]byte(userReq.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return nil, serverError("cannot hash password", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, serverError("cannot start transaction", err)
	}
	_, err = tx.Exec("DELETE FROM tokens WHERE user_id = ?", user.Id)
	if err != nil {
		tx.Rollback()
		return nil, serverError("cannot delete old tokens", err)
	}
	_, err = tx.Exec("UPDATE users SET password = ? WHERE id = ?", newHashPwd, user.Id)
	if err != nil {
		tx.Rollback()
		return nil, serverError("cannot update password", err)
	}
	err = tx.Commit()
	if err != nil {
		return nil, serverError("cannot commit transaction", err)
	}
	return user, nil
}

// HTTP Handlers

func sendUserResponse(w http.ResponseWriter, user *User) {
	// We need to send the user information back but hide all the sensitive information.
	user.Id = 0
	user.Password = ""
	user.NewPassword = ""
	writeJSONResponse(w, user)
}

func sendError(w http.ResponseWriter, err error) {
	log.Printf("Sending error to client: %v", err)
	writeJSONResponse(w, struct {
		Error string `json:"error"`
	}{err.Error()})
}

func handleUser(w http.ResponseWriter, r *http.Request, userFunc func(*User) (*User, error)) {
	var userReq User
	err := json.NewDecoder(r.Body).Decode(&userReq)
	if err != nil {
		sendError(w, err)
		return
	}

	user, err := userFunc(&userReq)
	if err != nil {
		sendError(w, err)
		return
	}

	token, err := addNewTokenToUser(db, user.Id)
	if err != nil {
		sendError(w, err)
		return
	}

	user.Token = token
	sendUserResponse(w, user)
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	handleUser(w, r, AuthenticateUser)
}

func registerUserHandler(w http.ResponseWriter, r *http.Request) {
	handleUser(w, r, RegisterUser)
}

func changePasswordHandler(w http.ResponseWriter, r *http.Request) {
	handleUser(w, r, changePassword)
}

func authenticateToken(r *http.Request) (*User, error) {
	token := Token(r.URL.Query().Get("token"))
	if token == "" {
		return nil, fmt.Errorf("missing token")
	}
	user, err := GetUserWithToken(token)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("token not found")
	} else if err != nil {
		return nil, serverError("cannot get user with token", err)
	}
	return user, nil
}

func verificationHandler(w http.ResponseWriter, r *http.Request) {
	user, err := authenticateToken(r)
	if err != nil {
		sendError(w, err)
		return
	}
	_, err = db.Exec("UPDATE users SET verified = 1 WHERE email = ?", user.Email)
	if err != nil {
		sendError(w, err)
		return
	}
	// TODO: indicate the verification is successful
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	user, err := authenticateToken(r)
	if err != nil {
		sendError(w, err)
		return
	}
	sendUserResponse(w, user)
}
