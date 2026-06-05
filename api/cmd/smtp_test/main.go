// Command smtp_test sends a single test email using the same SMTP_* environment
// variables and code path as handlers.SMTPEmailSender. It exists purely to
// isolate SMTP/SES configuration problems from the data-export flow: run it
// locally with the deployment's SMTP credentials exported and read the full
// error it prints.
//
//	SMTP_HOST=email-smtp.us-east-1.amazonaws.com \
//	SMTP_PORT=587 \
//	SMTP_USER=... SMTP_PASSWORD=... SMTP_FROM=noreply@wikisubmission.org \
//	go run ./api/cmd/smtp_test you@example.com
package main

import (
	"fmt"
	"net/smtp"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: smtp_test <recipient-email>")
		os.Exit(2)
	}
	to := os.Args[1]

	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASSWORD")
	from := os.Getenv("SMTP_FROM")

	fmt.Printf("host=%q port=%q user=%q from=%q pass_set=%v\n", host, port, user, from, pass != "")
	if host == "" || port == "" || user == "" || pass == "" || from == "" {
		fmt.Fprintln(os.Stderr, "ERROR: one or more SMTP_* variables are empty")
		os.Exit(1)
	}

	msg := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: wikisubmission.org SMTP test\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"This is a test from the smtp_test command. If you received this, SMTP works."

	addr := host + ":" + port
	authn := smtp.PlainAuth("", user, pass, host)
	if err := smtp.SendMail(addr, authn, from, []string{to}, []byte(msg)); err != nil {
		fmt.Fprintf(os.Stderr, "SEND FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("SEND OK — check the inbox for", to)
}
