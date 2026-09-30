package msgateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	responseRecorder := httptest.NewRecorder()

	NewHandler("http://localhost:5173").ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, responseRecorder.Code)
	}

	if contentType := responseRecorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected application/json, got %s", contentType)
	}
}

func TestCORSPreflight(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/api/orders", nil)
	responseRecorder := httptest.NewRecorder()

	NewHandler("http://localhost:5173").ServeHTTP(responseRecorder, request)

	if responseRecorder.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, responseRecorder.Code)
	}

	if origin := responseRecorder.Header().Get("Access-Control-Allow-Origin"); origin != "http://localhost:5173" {
		t.Fatalf("unexpected CORS origin: %s", origin)
	}
}
