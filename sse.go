package bridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// SSEHandler represents an HTTP handler specifically for Server-Sent Events.
type SSEHandler struct {
	bridge *Bridge
}

// NewSSEHandler creates a new handler that encapsulates the bridge logic.
func NewSSEHandler(b *Bridge) *SSEHandler {
	return &SSEHandler{bridge: b}
}

// ServeHTTP handles building SSE from CLI outputs.
func (s *SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Allow cross-origin default
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	flusher.Flush()

	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.sendEvent(w, flusher, "error", fmt.Sprintf("invalid request payload: %v", err))
		return
	}

	ctx := r.Context()
	ch := make(chan StreamEvent)

	go s.bridge.Stream(ctx, &req, ch)

	for event := range ch {
		if event.Error != nil {
			s.sendEvent(w, flusher, "error", event.Error.Error())
			return
		}
		if event.Done {
			s.sendEvent(w, flusher, "done", "[DONE]")
			return
		}
		if event.Content != "" {
			s.sendEvent(w, flusher, "message", event.Content)
		}
	}
}

// sendEvent formats it to ensure newlines in outputs do not corrupt the SSE framework
func (s *SSEHandler) sendEvent(w http.ResponseWriter, flusher http.Flusher, eventType, data string) {
	if eventType != "" {
		fmt.Fprintf(w, "event: %s\n", eventType)
	}

	// We JSON encode the data content to make it a single line with `\n` safely escaped inside.
	// This helps client-side parsers easily decode SSE streams without multiline data splicing.
	type Payload struct {
		Text string `json:"text"`
	}
	b, _ := json.Marshal(Payload{Text: data})

	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		fmt.Fprintf(w, "data: %s\n", line)
	}

	fmt.Fprintf(w, "\n")
	flusher.Flush()
}
