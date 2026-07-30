package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type passwordResetDelivery interface {
	SendPasswordReset(context.Context, authUser, string) error
}

type smtpPasswordResetDelivery struct {
	host     string
	port     int
	username string
	password string
	from     string
}

var passwordResetDeliveryStore passwordResetDelivery

func newSMTPPasswordResetDelivery(
	host string, port int, username string, password string, from string,
) (*smtpPasswordResetDelivery, error) {
	if strings.TrimSpace(host) == "" || port < 1 || port > 65535 ||
		strings.TrimSpace(username) == "" || password == "" {
		return nil, fmt.Errorf("incomplete SMTP configuration")
	}
	parsedFrom, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("invalid SMTP sender: %w", err)
	}
	return &smtpPasswordResetDelivery{
		host: strings.TrimSpace(host), port: port, username: strings.TrimSpace(username),
		password: password, from: parsedFrom.Address,
	}, nil
}

func (delivery *smtpPasswordResetDelivery) SendPasswordReset(
	ctx context.Context, user authUser, resetURL string,
) error {
	recipient, err := mail.ParseAddress(user.Email)
	if err != nil {
		return fmt.Errorf("invalid reset recipient: %w", err)
	}
	address := net.JoinHostPort(delivery.host, strconv.Itoa(delivery.port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	rawConnection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial SMTP: %w", err)
	}
	tlsConnection := tls.Client(rawConnection, &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: delivery.host,
	})
	if deadline, ok := ctx.Deadline(); ok {
		_ = tlsConnection.SetDeadline(deadline)
	} else {
		_ = tlsConnection.SetDeadline(time.Now().Add(20 * time.Second))
	}
	if err := tlsConnection.HandshakeContext(ctx); err != nil {
		_ = rawConnection.Close()
		return fmt.Errorf("SMTP TLS handshake: %w", err)
	}
	client, err := smtp.NewClient(tlsConnection, delivery.host)
	if err != nil {
		_ = tlsConnection.Close()
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()
	if err := client.Auth(smtp.PlainAuth("", delivery.username, delivery.password, delivery.host)); err != nil {
		return fmt.Errorf("authenticate SMTP: %w", err)
	}
	if err := client.Mail(delivery.from); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(recipient.Address); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP message: %w", err)
	}
	message := strings.Join([]string{
		"From: " + delivery.from,
		"To: " + recipient.Address,
		"Subject: OpenResource password reset",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		"你好，" + user.DisplayName + "：",
		"",
		"请在 30 分钟内打开以下链接重置密码：",
		resetURL,
		"",
		"如果不是你发起的请求，请忽略此邮件。",
	}, "\r\n")
	if _, err := io.WriteString(writer, message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit SMTP: %w", err)
	}
	return nil
}
