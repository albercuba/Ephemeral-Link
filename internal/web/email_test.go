package web

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"ephemeral-link/internal/redisstore"
)

type fakeGraphClient struct{ requests []*http.Request }

func (c *fakeGraphClient) Do(req *http.Request) (*http.Response, error) {
	c.requests = append(c.requests, req)
	if strings.Contains(req.URL.Host, "login.microsoftonline.com") {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"access_token":"token"}`)), Header: http.Header{}}, nil
	}
	if req.URL.Host == "graph.microsoft.com" && req.Header.Get("Authorization") == "Bearer token" {
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}
	return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
}

func TestSendGraphRequestsTokenAndSendMail(t *testing.T) {
	client := &fakeGraphClient{}
	cfg := redisstore.IntegrationConfig{GraphTenantID: "tenant", GraphClientID: "client", GraphClientSecret: "secret", GraphSender: "sender@example.com"}
	if err := sendGraph(context.Background(), client, cfg, "to@example.com", "Subject", emailContent{Plain: "Body"}); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected token and sendMail requests, got %d", len(client.requests))
	}
	if client.requests[0].URL.Path != "/tenant/oauth2/v2.0/token" {
		t.Fatalf("unexpected token path: %s", client.requests[0].URL.Path)
	}
	if client.requests[1].URL.Path != "/v1.0/users/sender@example.com/sendMail" {
		t.Fatalf("unexpected sendMail path: %s", client.requests[1].URL.Path)
	}
}
