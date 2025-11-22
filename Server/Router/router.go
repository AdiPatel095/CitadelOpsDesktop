package Router

import (
	"CitadelDesktop/Server/Websocket"
	"net/http"
)

func NewRouter(hub *Websocket.Hub) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		Websocket.ServeWs(hub, w, r)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
