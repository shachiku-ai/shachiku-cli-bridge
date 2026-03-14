# shachiku-cli-bridge

A unified Go bridge for executing AI command-line tools including `gemini`, `codexcli`, and `claude`.

This package exposes both synchronous executions and streaming (Server-Sent Events) executions. It wraps around CLI programs using a pseudo-terminal (PTY) to ensure raw, unbroken text streams without buffering issues typically seen in Standard pipes.

## Features
- **Unified Interface:** Talk to Google Gemini, OpenAI Codex, and Anthropic Claude via the same logical requests.
- **File & System Prompt Support:** Pass system instructions and files seamlessly; they map properly to the respective CLI flags.
- **SSE Streaming Support:** Features a native built-in `http.Handler` specifically for Server-Sent Events.

## Supported Providers
- **gemini:** Uses `--include-directories` for files. Needs to be in `$PATH` or configured.
- **codexcli:** Uses `--image` for files. Needs to be in `$PATH` or configured.
- **claude:** (Claude Code) Uses `--file` and `-p` for files and prompt respectively. Needs to be in `$PATH` or configured.

## Usage

### 1. Simple Streaming / Execution

```go
package main

import (
    "context"
    "fmt"
    "github.com/shachiku-ai/shachiku-cli-bridge"
)

func main() {
    b := bridge.NewBridge()
    b.ClaudePath = "/usr/local/bin/claude" // Configure path if needed

    req := &bridge.Request{
        Provider:     bridge.ProviderClaude,
        SystemPrompt: "You are a professional science explainer.",
        Files:        []string{"/path/to/local_image.png"}, // Automatically attached
        Messages: []bridge.Message{
            {
                Role:    "user",
                Content: "Explain quantum computing in one paragraph.",
            },
        },
    }

    // Stream
    ch := make(chan bridge.StreamEvent)
    go b.Stream(context.Background(), req, ch)

    for ev := range ch {
        if ev.Done {
            break
        }
        fmt.Print(ev.Content)
    }
}
```

### 2. Built-in SSE Server

Use the provided helper to quickly start accepting JSON requests and streaming SSE replies directly to browser clients.

```go
package main

import (
	"log"
	"net/http"

	"github.com/shachiku-ai/shachiku-cli-bridge"
)

func main() {
	b := bridge.NewBridge()
	http.Handle("/api/chat", bridge.NewSSEHandler(b))

	log.Println("Listening on :8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// Clients send POST /api/chat:
// {
//    "provider": "claude",
//    "system_prompt": "You are a helpful assistant.",
//    "files": ["/path/to/local_image.png"],
//    "messages": [
//        { "role": "user", "content": "Hello!" }
//    ]
// }
```
