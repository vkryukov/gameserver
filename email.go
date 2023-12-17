package gameserver

import (
	"crypto/tls"
	"encoding/json"
	"os"

	"gopkg.in/mail.v2"
)

// EmailSender is an interface for sending emails.
type EmailSender interface {
	Send(to, subject, body string) error
}

type SmtpServer struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Server   string `json:"server"`
	Port     int    `json:"port"`
}

func (server *SmtpServer) Send(to, subject, body string) error {
	m := mail.NewMessage()
	m.SetHeader("From", server.Email)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := mail.NewDialer(server.Server, server.Port, server.Email, server.Password)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	return d.DialAndSend(m)
}

func SmtpServerFromConfig(path string) (*SmtpServer, error) {
	var server SmtpServer
	configFile, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(configFile, &server)
	return &server, err
}

var globalMailServer EmailSender

func InitEmailServer(server EmailSender) {
	globalMailServer = server
}
func SendMessage(to, subject, body string) error {
	return globalMailServer.Send(to, subject, body)
}
