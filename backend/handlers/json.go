package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

func decodeJSON(r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func decodeJSONOptional(r *http.Request, target interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return nil
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// respondError writes a JSON error response.
func respondError(w http.ResponseWriter, status int, message string) {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "server_config_path_unsafe:") {
		detail := strings.TrimSpace(strings.TrimPrefix(trimmed, "server_config_path_unsafe:"))
		if detail == "" {
			detail = "Server path is outside managed directories."
		}
		respondJSON(w, status, map[string]string{
			"error":   "server_config_path_unsafe",
			"message": detail,
		})
		return
	}
	respondJSON(w, status, map[string]string{"error": message})
}
