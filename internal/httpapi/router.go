package httpapi

import (
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/game"
)

func New(logger *slog.Logger, webDir, scenariosDir string) http.Handler {
	return NewWithSocket(logger, webDir, scenariosDir, nil)
}
func NewWithSocket(logger *slog.Logger, webDir, scenariosDir string, socket http.Handler) http.Handler {
	return newRouter(logger, webDir, scenariosDir, socket, nil)
}

func NewServer(logger *slog.Logger, webDir, scenariosDir string, socket http.Handler, manager *game.Manager, adminToken string) http.Handler {
	return newRouter(logger, webDir, scenariosDir, socket, &serverAPI{logger: logger, manager: manager, adminToken: adminToken})
}

func newRouter(logger *slog.Logger, webDir, scenariosDir string, socket http.Handler, routes *serverAPI) http.Handler {
	mux := http.NewServeMux()
	if routes != nil {
		routes.lobbyRoutes(mux)
		routes.roundRoutes(mux)
		routes.historyRoutes(mux)
	}
	if socket != nil {
		mux.Handle("GET /ws", socket)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /images/{assetSet}/{file...}", func(w http.ResponseWriter, r *http.Request) {
		serveImage(w, r, scenariosDir)
	})
	for _, pattern := range []string{"/api", "/api/", "/ws", "/ws/", "/images", "/images/", "/images/{assetSet}"} {
		mux.HandleFunc(pattern, notFound)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			notFound(w, r)
			return
		}
		serveSPA(w, r, webDir)
	})

	return middleware(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject invalid paths before ServeMux can redirect them to another route.
		name := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/")
		if r.URL.Path != "/" && !localFileName(name) {
			notFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))
}

func localFileName(name string) bool {
	return name != "." && fs.ValidPath(name) && filepath.IsLocal(name) &&
		!strings.ContainsAny(name, "\\:\x00")
}

func serveSPA(w http.ResponseWriter, r *http.Request, webDir string) {
	root, err := os.OpenRoot(webDir)
	if err != nil {
		notFound(w, r)
		return
	}
	defer root.Close()

	name := strings.TrimPrefix(r.URL.Path, "/")
	if !serveFile(w, r, root, name) && !serveFile(w, r, root, "index.html") {
		notFound(w, r)
	}
}

func serveImage(w http.ResponseWriter, r *http.Request, scenariosDir string) {
	assetSet := r.PathValue("assetSet")
	name := r.PathValue("file")
	if !localFileName(assetSet) || strings.Contains(assetSet, "/") || !localFileName(name) {
		notFound(w, r)
		return
	}

	scenarios, err := os.OpenRoot(scenariosDir)
	if err != nil {
		notFound(w, r)
		return
	}
	defer scenarios.Close()
	assets, err := openDirectory(scenarios, assetSet)
	if err != nil {
		notFound(w, r)
		return
	}
	defer assets.Close()
	images, err := openDirectory(assets, "images")
	if err != nil {
		notFound(w, r)
		return
	}
	defer images.Close()

	if !serveFile(w, r, images, name) {
		notFound(w, r)
	}
}

// Keep the asset-set and images boundaries tied to real directories.
func openDirectory(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if err != nil || !info.IsDir() {
		return nil, fs.ErrNotExist
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		child.Close()
		return nil, fs.ErrNotExist
	}
	return child, nil
}

func serveFile(w http.ResponseWriter, r *http.Request, root *os.Root, name string) bool {
	if !localFileName(name) {
		return false
	}
	name = filepath.FromSlash(name)
	info, err := root.Stat(name)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	file, err := root.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
	return true
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, api.CodeNotFound, "Ресурс не найден")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{
		Error: api.ErrorBody{Code: code, Message: message},
	})
}
