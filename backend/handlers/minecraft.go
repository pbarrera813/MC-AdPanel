package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"minecraft-admin/minecraft"
)

// MinecraftHandler handles WebSocket connections for console streaming
type MinecraftHandler struct {
	mgr      *minecraft.Manager
	upgrader websocket.Upgrader
}

// NewMinecraftHandler creates a new MinecraftHandler
func NewMinecraftHandler(mgr *minecraft.Manager) *MinecraftHandler {
	return &MinecraftHandler{
		mgr: mgr,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins (CORS handled at middleware level)
			},
		},
	}
}

// wsMessage is the JSON structure sent to WebSocket clients
type wsMessage struct {
	Type    string                      `json:"type"`
	Seq     uint64                      `json:"seq,omitempty"`
	Line    string                      `json:"line,omitempty"`
	Entries []minecraft.ConsoleLogEntry `json:"entries,omitempty"`
	Reset   bool                        `json:"reset,omitempty"`
}

// WebSocketLogs returns an HTTP handler that upgrades to WebSocket for log streaming
func (h *MinecraftHandler) WebSocketLogs() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Server ID is required", http.StatusBadRequest)
			return
		}

		// Verify server exists
		if _, err := h.mgr.GetStatus(id); err != nil {
			http.Error(w, "Server not found", http.StatusNotFound)
			return
		}

		// Upgrade to WebSocket
		conn, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket upgrade failed for server %s: %v", id, err)
			return
		}
		defer conn.Close()

		log.Printf("WebSocket connected for server %s", id)

		var lastSeq uint64
		if raw := strings.TrimSpace(r.URL.Query().Get("lastSeq")); raw != "" {
			if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
				lastSeq = parsed
			}
		}

		// Subscribe to live stream and capture missing entries since lastSeq.
		snapshot, reset, logCh, unsubscribe := h.mgr.SubscribeLogsWithSnapshot(id, lastSeq)
		defer unsubscribe()

		if err := conn.WriteJSON(wsMessage{
			Type:    "snapshot",
			Entries: snapshot,
			Reset:   reset,
		}); err != nil {
			log.Printf("WebSocket initial snapshot write error for server %s: %v", id, err)
			return
		}

		// Channel to signal connection close
		done := make(chan struct{})

		// Read goroutine: client sends commands
		go func() {
			defer close(done)
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
						log.Printf("WebSocket read error for server %s: %v", id, err)
					}
					return
				}

				command := strings.TrimSpace(string(msg))
				if command != "" {
					if err := h.mgr.SendCommand(id, command); err != nil {
						log.Printf("Failed to send command to server %s: %v", id, err)
						continue
					}
					if err := h.mgr.RecordConsoleCommand(id, command); err != nil {
						log.Printf("Failed to record command in console for server %s: %v", id, err)
					}
				}
			}
		}()

		// Write loop: send log lines to client
		for {
			select {
			case entry, ok := <-logCh:
				if !ok {
					return // Channel closed
				}
				err := conn.WriteJSON(wsMessage{
					Type: "log",
					Seq:  entry.Seq,
					Line: entry.Line,
				})
				if err != nil {
					log.Printf("WebSocket write error for server %s: %v", id, err)
					return
				}
			case <-done:
				return // Client disconnected
			}
		}
	})
}
