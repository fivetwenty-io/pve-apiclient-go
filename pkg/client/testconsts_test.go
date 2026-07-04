package client_test

// Shared test constants used across client package test files.
const (
	testHost                 = "pve.example.com"
	testUsername             = "root@pam"
	testPassword             = "secret"
	testAPIToken             = "root@pam!token=secret"
	testProtoHTTP            = "http"
	testProtoHTTPS           = "https"
	testErrProtocol          = "invalid protocol"
	testErrPort              = "invalid port"
	testAccessTicketEndpoint = "/api2/json/access/ticket"
)
