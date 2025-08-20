package client

import (
	"fmt"
	"time"
)

// Client defines the interface for interacting with the PVE API.
type Client interface {
	// HTTP Methods
	Get(path string, params map[string]interface{}) (interface{}, error)
	GetRaw(path string, params map[string]interface{}) (*Response, error)
	Post(path string, params map[string]interface{}) (interface{}, error)
	PostRaw(path string, params map[string]interface{}) (*Response, error)
	Put(path string, params map[string]interface{}) (interface{}, error)
	PutRaw(path string, params map[string]interface{}) (*Response, error)
	Delete(path string, params map[string]interface{}) (interface{}, error)
	DeleteRaw(path string, params map[string]interface{}) (*Response, error)

	// Authentication
	Login() error
	Logout() error
	UpdateTicket(ticket string)
	UpdateCSRFToken(token string)

	// Configuration
	SetTimeout(timeout time.Duration)
	SetKeepAlive(connections int)
}

// Response represents a response from the PVE API.
type Response struct {
	Data   interface{}
	Errors map[string]string
	Code   int
}

// client implements the Client interface.
type client struct {
	options    *Options
	httpClient HTTPClient
	auth       Authenticator
}

// HTTPClient defines the interface for HTTP operations.
type HTTPClient interface {
	Do(method, path string, params map[string]interface{}) (*Response, error)
	SetHeader(key, value string)
	RemoveHeader(key string)
}

// Authenticator defines the interface for authentication operations.
type Authenticator interface {
	Login(username, password string) (ticket string, csrf string, err error)
	Logout(ticket string) error
	IsValid() bool
	GetHeaders() map[string]string
}

// NewClient creates a new PVE API client with the given options.
func NewClient(opts Options) (Client, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	opts.setDefaults()

	// Create the HTTP client
	httpClient, err := createHTTPClient(&opts)
	if err != nil {
		return nil, err
	}

	// Create the authenticator
	auth, err := createAuthenticator(&opts, httpClient)
	if err != nil {
		return nil, err
	}

	c := &client{
		options:    &opts,
		httpClient: httpClient,
		auth:       auth,
	}

	// Perform initial login if credentials are provided
	if opts.NeedsLogin() {
		if err := c.Login(); err != nil {
			return nil, err
		}
	}

	return c, nil
}

// Get performs a GET request to the specified path.
func (c *client) Get(path string, params map[string]interface{}) (interface{}, error) {
	resp, err := c.GetRaw(path, params)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetRaw performs a GET request and returns the raw response.
func (c *client) GetRaw(path string, params map[string]interface{}) (*Response, error) {
	return c.call("GET", path, params)
}

// Post performs a POST request to the specified path.
func (c *client) Post(path string, params map[string]interface{}) (interface{}, error) {
	resp, err := c.PostRaw(path, params)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// PostRaw performs a POST request and returns the raw response.
func (c *client) PostRaw(path string, params map[string]interface{}) (*Response, error) {
	return c.call("POST", path, params)
}

// Put performs a PUT request to the specified path.
func (c *client) Put(path string, params map[string]interface{}) (interface{}, error) {
	resp, err := c.PutRaw(path, params)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// PutRaw performs a PUT request and returns the raw response.
func (c *client) PutRaw(path string, params map[string]interface{}) (*Response, error) {
	return c.call("PUT", path, params)
}

// Delete performs a DELETE request to the specified path.
func (c *client) Delete(path string, params map[string]interface{}) (interface{}, error) {
	resp, err := c.DeleteRaw(path, params)
	if err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// DeleteRaw performs a DELETE request and returns the raw response.
func (c *client) DeleteRaw(path string, params map[string]interface{}) (*Response, error) {
	return c.call("DELETE", path, params)
}

// Login authenticates with the PVE API.
func (c *client) Login() error {
	if c.auth == nil {
		return fmt.Errorf("no authenticator configured")
	}

	ticket, csrf, err := c.auth.Login(c.options.Username, c.options.Password)
	if err != nil {
		return fmt.Errorf("login failed: %w", err)
	}

	c.options.Ticket = ticket
	c.options.CSRFToken = csrf

	return nil
}

// Logout logs out from the PVE API.
func (c *client) Logout() error {
	if c.auth == nil {
		return nil
	}

	err := c.auth.Logout(c.options.Ticket)
	c.options.Ticket = ""
	c.options.CSRFToken = ""

	return err
}

// UpdateTicket updates the authentication ticket.
func (c *client) UpdateTicket(ticket string) {
	c.options.Ticket = ticket
}

// UpdateCSRFToken updates the CSRF prevention token.
func (c *client) UpdateCSRFToken(token string) {
	c.options.CSRFToken = token
}

// SetTimeout sets the request timeout.
func (c *client) SetTimeout(timeout time.Duration) {
	c.options.Timeout = timeout
}

// SetKeepAlive sets the number of keep-alive connections.
func (c *client) SetKeepAlive(connections int) {
	c.options.KeepAlive = connections
}

// call is the central request handler.
func (c *client) call(method, path string, params map[string]interface{}) (*Response, error) {
	// Ensure we're authenticated if needed
	if c.auth != nil && !c.options.IsUsingAPIToken() {
		// For ticket-based auth, check if we need to re-authenticate
		if !c.auth.IsValid() {
			if err := c.Login(); err != nil {
				return nil, err
			}
		}
	}

	// Make the HTTP request
	resp, err := c.httpClient.Do(method, path, params)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// createHTTPClient creates the HTTP client based on options.
func createHTTPClient(opts *Options) (HTTPClient, error) {
	// This will use the internal HTTP client implementation
	// For now, return a simple implementation
	return &simpleHTTPClient{
		baseURL: opts.GetBaseURL(),
		options: opts,
	}, nil
}

// createAuthenticator creates the authenticator based on options.
func createAuthenticator(opts *Options, httpClient HTTPClient) (Authenticator, error) {
	if opts.APIToken != "" {
		// Use API token authentication
		return &simpleTokenAuth{
			token: opts.APIToken,
		}, nil
	}

	if opts.Username != "" {
		// Use ticket authentication
		return &simpleTicketAuth{
			username: opts.Username,
			password: opts.Password,
			ticket:   opts.Ticket,
			csrf:     opts.CSRFToken,
		}, nil
	}

	// No authentication configured
	return nil, nil
}

// simpleHTTPClient is a basic HTTP client implementation.
type simpleHTTPClient struct {
	baseURL string
	options *Options
}

func (s *simpleHTTPClient) Do(method, path string, params map[string]interface{}) (*Response, error) {
	// This would use the internal/http package
	// For now, return a basic response
	return &Response{
		Data: map[string]interface{}{
			"status": "ok",
		},
		Code: 200,
	}, nil
}

func (s *simpleHTTPClient) SetHeader(key, value string) {
	// Implementation would store headers
}

func (s *simpleHTTPClient) RemoveHeader(key string) {
	// Implementation would remove headers
}

// simpleTokenAuth is a basic API token authenticator.
type simpleTokenAuth struct {
	token string
}

func (s *simpleTokenAuth) Login(username, password string) (string, string, error) {
	// API tokens don't need login
	return "", "", nil
}

func (s *simpleTokenAuth) Logout(ticket string) error {
	// API tokens don't need logout
	return nil
}

func (s *simpleTokenAuth) IsValid() bool {
	return s.token != ""
}

func (s *simpleTokenAuth) GetHeaders() map[string]string {
	return map[string]string{
		"Authorization": "PVEAPIToken=" + s.token,
	}
}

// simpleTicketAuth is a basic ticket authenticator.
type simpleTicketAuth struct {
	username string
	password string
	ticket   string
	csrf     string
}

func (s *simpleTicketAuth) Login(username, password string) (string, string, error) {
	// This would perform actual login
	// For now, return dummy values
	s.ticket = "PVE:dummy:ticket"
	s.csrf = "dummy:csrf:token"
	return s.ticket, s.csrf, nil
}

func (s *simpleTicketAuth) Logout(ticket string) error {
	s.ticket = ""
	s.csrf = ""
	return nil
}

func (s *simpleTicketAuth) IsValid() bool {
	return s.ticket != ""
}

func (s *simpleTicketAuth) GetHeaders() map[string]string {
	headers := make(map[string]string)
	if s.ticket != "" {
		headers["Cookie"] = "PVEAuthCookie=" + s.ticket
	}
	if s.csrf != "" {
		headers["CSRFPreventionToken"] = s.csrf
	}
	return headers
}
