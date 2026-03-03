package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func TestHealthz(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Get("/healthz")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("expected ok status, got %s", body)
	}
}

func TestVersion(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Get("/version")
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"version"`) {
		t.Fatalf("expected version field, got %s", body)
	}
}

func TestEcho(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Post("/echo", strings.NewReader(`{"message":"hello","count":42}`))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `"message":"hello"`) {
		t.Fatalf("expected echo of message, got %s", body)
	}
	if !strings.Contains(body, `"count":42`) {
		t.Fatalf("expected echo of count, got %s", body)
	}
}

func TestEchoEmpty(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("Test", "0.0.0"))
	registerRoutes(api)

	resp := api.Post("/echo", strings.NewReader(`{}`))
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
}
