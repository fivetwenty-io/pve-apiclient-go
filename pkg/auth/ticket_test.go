package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fivetwenty-io/pve-apiclient-go/pkg/auth"
)

func TestTicketAuthenticator_NewFormatAndCookieName(t *testing.T) {
	t.Parallel()

	// Mock API server
	var sawNewFormat bool

	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, "/api2/json") {
			http.NotFound(writer, request)

			return
		}

		switch request.URL.Path {
		case "/api2/json/access/ticket":
			err := request.ParseForm()
			if err == nil {
				if request.Form.Get("new-format") == "1" {
					sawNewFormat = true
				}
			}

			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"data":{"ticket":"TICKET","CSRFPreventionToken":"CSRF","username":"root@pam"},"success":1}`)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	baseURL := u.Scheme + "://" + u.Host + "/api2/json"

	httpClient := srv.Client()
	creds := &auth.Credentials{Username: "root", Password: "secret", Realm: "pam"}
	ticketAuth := auth.NewTicketAuthenticator(baseURL, creds, httpClient, "CustomCookie", true)

	err := ticketAuth.Authenticate()
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}

	if !sawNewFormat {
		t.Fatalf("expected new-format=1 to be sent in login request")
	}

	headers := ticketAuth.GetHeaders()
	if got := headers["Cookie"]; !strings.HasPrefix(got, "CustomCookie=") {
		t.Fatalf("expected Cookie header to use custom name, got %q", got)
	}

	if headers["CSRFPreventionToken"] != "CSRF" {
		t.Fatalf("expected CSRFPreventionToken header to be set")
	}
}
