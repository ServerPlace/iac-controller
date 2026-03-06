package httputil

import (
	"encoding/json"
	"net/http"
)

// RespondJSON writes body as JSON with the given HTTP status code.
func RespondJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
