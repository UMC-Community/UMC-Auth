package main

import (
	"bufio"
	"net"
	"strings"
	"testing"
)

func TestMailerSend(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	var received strings.Builder
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		reply := func(line string) {
			_, _ = writer.WriteString(line + "\r\n")
			_ = writer.Flush()
		}
		reply("220 test.local ESMTP")
		dataMode := false
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if dataMode {
				if line == "." {
					dataMode = false
					reply("250 queued")
					continue
				}
				received.WriteString(line + "\n")
				continue
			}
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				reply("250 test.local")
			case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
				reply("250 ok")
			case line == "DATA":
				reply("354 end with .")
				dataMode = true
			case line == "QUIT":
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()

	mailer, err := NewMailer(SMTPConfig{
		Host: "127.0.0.1",
		Port: port,
		From: "UMC Auth <no-reply@example.com>",
		TLS:  "none",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mailer.Send("player@example.com", "Test subject", "Test body"); err != nil {
		t.Fatal(err)
	}
	<-serverDone
	message := received.String()
	for _, expected := range []string{
		"Subject: Test subject",
		"To: <player@example.com>",
		"Test body",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("message is missing %q:\n%s", expected, message)
		}
	}
}
