package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Arseniy22877/game/internal/api"
)

func TestRoutes(t *testing.T) {
	web, scenarios := t.TempDir(), t.TempDir()
	images := filepath.Join(scenarios, "v2", "images")
	writeFixture(t, filepath.Join(web, "index.html"), "app")
	writeFixture(t, filepath.Join(web, "assets", "app.js"), "script")
	writeFixture(t, filepath.Join(images, "nested", "event.jpg"), "image")
	writeFixture(t, filepath.Join(scenarios, "v2", "private.json"), "private")
	writeFixture(t, filepath.Join(scenarios, "private.json"), "private")
	for link, target := range map[string]string{
		filepath.Join(images, "escape.jpg"):         "../private.json",
		filepath.Join(images, "local.jpg"):          "nested/event.jpg",
		filepath.Join(scenarios, "alias"):           "v2",
		filepath.Join(scenarios, "other", "images"): "../v2",
	} {
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	handler := New(slog.New(slog.DiscardHandler), web, scenarios)
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/healthz", "ok"},
		{"/?token=invite", "app"},
		{"/admin", "app"},
		{"/admin/round/", "app"},
		{"/assets/app.js", "script"},
		{"/api", ""},
		{"/api/unknown", ""},
		{"/ws?token=invite", ""},
		{"/ws/unknown", ""},
		{"/images", ""},
		{"/images/v2", ""},
		{"/images/v2/images/nested/event.jpg", ""},
		{"/images/v2/nested/event.jpg", "image"},
		{"/images/v2/local.jpg", "image"},
		{"/images/v2/nested/", ""},
		{"/images/v2/nested", ""},
		{"/images/v2/missing.jpg", ""},
		{"/images/v2/../private.json", ""},
		{"/images/v2/%2e%2e%2fprivate.json", ""},
		{"/images/v2/..%5cprivate.json", ""},
		{"/images/v2/escape.jpg", ""},
		{"/images/alias/nested/event.jpg", ""},
		{"/images/v2%2fnested/private.json", ""},
		{"/images/other/private.json", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if tc.body == "" {
				assertError(t, response, http.StatusNotFound, api.CodeNotFound)
				return
			}
			if response.Code != http.StatusOK || response.Body.String() != tc.body {
				t.Fatalf("got %d %q, want 200 %q", response.Code, response.Body, tc.body)
			}
		})
	}
}

func TestRealScenarioImageWithoutWebDist(t *testing.T) {
	scenarios := filepath.Join("..", "..", "scenarios")
	name := "event-1794843351907019500.jpg"
	want, err := os.ReadFile(filepath.Join(scenarios, "republic_n_scenario_v2", "images", name))
	if err != nil {
		t.Fatal(err)
	}
	handler := New(slog.New(slog.DiscardHandler), filepath.Join(t.TempDir(), "missing"), scenarios)

	image := httptest.NewRecorder()
	handler.ServeHTTP(image, httptest.NewRequest(http.MethodGet, "/images/republic_n_scenario_v2/"+name, nil))
	if image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/jpeg" ||
		!bytes.Equal(image.Body.Bytes(), want) {
		t.Fatalf("got status %d, type %q and %d bytes, want the scenario JPEG", image.Code, image.Header().Get("Content-Type"), image.Body.Len())
	}
	app := httptest.NewRecorder()
	handler.ServeHTTP(app, httptest.NewRequest(http.MethodGet, "/admin", nil))
	assertError(t, app, http.StatusNotFound, api.CodeNotFound)
}

func TestMiddleware(t *testing.T) {
	var logs bytes.Buffer
	handler := middleware(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		var tooLarge *http.MaxBytesError
		if len(body) != maxBodyBytes || !errors.As(err, &tooLarge) {
			t.Fatalf("read %d bytes with error %v, want MaxBytesError at %d", len(body), err, maxBodyBytes)
		}
		panic("test panic")
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/test?token=secret", strings.NewReader(strings.Repeat("x", maxBodyBytes+1)))
	handler.ServeHTTP(response, request)
	assertError(t, response, http.StatusInternalServerError, api.CodeInternal)
	if response.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(logs.String(), `"status":500`) ||
		!strings.Contains(logs.String(), `"stack":`) ||
		strings.Contains(logs.String(), "secret") {
		t.Fatalf("unexpected headers or logs: %v %s", response.Header(), &logs)
	}
}

func writeFixture(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body api.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON error %q: %v", response.Body, err)
	}
	if response.Code != status || body.Error.Code != code || body.Error.Message == "" ||
		response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("got %d %s, want %d %s", response.Code, response.Body, status, code)
	}
}
