package mail

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"testing"
)

// fakeSMTPS is a minimal SMTP server that speaks TLS from the first byte,
// like port 465; it records the DATA it receives.
func fakeSMTPS(t *testing.T) (addr string, pool *x509.CertPool, got chan string) {
	t.Helper()
	hs := httptest.NewUnstartedServer(nil)
	hs.StartTLS()
	cert := hs.TLS.Certificates[0]
	pool = x509.NewCertPool()
	pool.AddCert(hs.Certificate())
	hs.Close()

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	got = make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		r := bufio.NewReader(conn)
		say := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
		say("220 fake ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			if inData {
				if line == ".\r\n" {
					inData = false
					got <- data.String()
					say("250 queued")
					continue
				}
				data.WriteString(line)
				continue
			}
			switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250-fake")
				say("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH"):
				say("235 ok")
			case strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				say("250 ok")
			case cmd == "DATA":
				inData = true
				say("354 go")
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	return ln.Addr().String(), pool, got
}

func TestSendImplicitTLS(t *testing.T) {
	addr, pool, got := fakeSMTPS(t)
	s := &SMTPSender{host: "127.0.0.1", tlsConfig: &tls.Config{RootCAs: pool, ServerName: "127.0.0.1"}}
	auth := smtp.PlainAuth("", "u@example.com", "pw", "127.0.0.1")
	if err := s.sendImplicitTLS(addr, auth, "u@example.com", []string{"to@example.com"}, []byte("Subject: hi\r\n\r\nbody\r\n")); err != nil {
		t.Fatalf("sendImplicitTLS: %v", err)
	}
	if body := <-got; !strings.Contains(body, "Subject: hi") {
		t.Fatalf("server got %q", body)
	}
}
