package httpapi

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Arseniy22877/game/internal/api"
)

const maxBodyBytes = 64 * 1024

func middleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		response := &responseStatus{ResponseWriter: w}
		isAPI := r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/")
		if isAPI {
			w.Header().Set("Cache-Control", "no-store")
		}
		defer func() {
			if response.status == 0 {
				response.status = http.StatusOK
			}
			logger.Info("http request", "method", r.Method, "path", r.URL.Path,
				"status", response.status, "duration", time.Since(start))
		}()
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				logger.Error("http panic", "stack", string(debug.Stack()))
				if response.status != 0 {
					panic(http.ErrAbortHandler)
				}
				clear(w.Header())
				if isAPI {
					w.Header().Set("Cache-Control", "no-store")
				}
				writeError(response, http.StatusInternalServerError, api.CodeInternal, "Внутренняя ошибка сервера")
			}
		}()

		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(response, r)
	})
}

type responseStatus struct {
	http.ResponseWriter
	status int
}

func (w *responseStatus) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseStatus) WriteHeader(status int) {
	if w.status == 0 && (status >= 200 || status == http.StatusSwitchingProtocols) {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseStatus) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(data)
}
