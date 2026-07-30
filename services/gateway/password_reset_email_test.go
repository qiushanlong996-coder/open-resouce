package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestSMTPPasswordResetDeliveryIntegration(t *testing.T) {
	host := os.Getenv("SMTP_TEST_HOST")
	username := os.Getenv("SMTP_TEST_USERNAME")
	password := os.Getenv("SMTP_TEST_PASSWORD")
	recipient := os.Getenv("SMTP_TEST_RECIPIENT")
	if host == "" || username == "" || password == "" || recipient == "" {
		t.Skip("SMTP test environment is not configured")
	}
	port := 465
	if value := os.Getenv("SMTP_TEST_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("parse SMTP_TEST_PORT: %v", err)
		}
		port = parsed
	}
	from := os.Getenv("SMTP_TEST_FROM")
	if from == "" {
		from = username
	}
	delivery, err := newSMTPPasswordResetDelivery(host, port, username, password, from)
	if err != nil {
		t.Fatalf("create SMTP password reset delivery: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	err = delivery.SendPasswordReset(ctx, authUser{
		Email:       recipient,
		DisplayName: "OpenResource SMTP 测试",
	}, "https://103.236.98.166:8443/?reset_token=smtp-delivery-test")
	if err != nil {
		t.Fatalf("send SMTP password reset message: %v", err)
	}
}
