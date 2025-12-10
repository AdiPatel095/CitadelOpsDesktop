package Router

import (
	"net/http"
)

func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/equipment/reconfigure", HandleReconfigure)
	mux.HandleFunc("/license/unverified/", HandleGetLicense) // Trailing slash for wildcard if needed, but path is usually exact in ServeMux unless strip prefix. Client uses /license/unverified/:hwid
	// Standard http.ServeMux matches patterns. "/license/unverified/" matches anything starting with that.
	return mux
}
