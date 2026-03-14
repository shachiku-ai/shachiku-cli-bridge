package bridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/acarl005/stripansi"
	"github.com/creack/pty"
)

// Provider represents the supported CLI providers
type Provider string

const (
	ProviderGemini Provider = "gemini"
	ProviderCodex  Provider = "codex"
	ProviderClaude Provider = "claude"
)

// Message represents a chat message
type Message struct {
	Role    string   `json:"role"`
	Content string   `json:"content"`
	Images  []string `json:"images,omitempty"` // Local paths or URLs if CLI supports it
}

// Request is the generic request payload
type Request struct {
	Provider Provider  `json:"provider"`
	Messages []Message `json:"messages"`
	Model    string    `json:"model,omitempty"`
}

// Bridge is the main client structure for interacting with the CLIs
type Bridge struct {
	GeminiPath string
	CodexPath  string
	ClaudePath string
	Debug      bool
}

// NewBridge initializes a new client with default CLI paths.
func NewBridge() *Bridge {
	return &Bridge{
		GeminiPath: "gemini",
		CodexPath:  "codexcli",
		ClaudePath: "claude",
	}
}

// StreamEvent represents a single token or error returned during a SSE/Stream run
type StreamEvent struct {
	Content string
	Error   error
	Done    bool
}

// BuildCommand constructs the executable command for the matched provider
func (b *Bridge) BuildCommand(ctx context.Context, req *Request) (*exec.Cmd, error) {
	if len(req.Messages) == 0 {
		return nil, errors.New("no messages provided")
	}

	var prompt string
	var images []string

	// Seek final user message
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			prompt = req.Messages[i].Content
			images = append(images, req.Messages[i].Images...)
			break
		}
	}

	if prompt == "" {
		return nil, errors.New("no user prompt found across messages")
	}

	var cmd *exec.Cmd

	switch req.Provider {
	case ProviderGemini:
		args := []string{}
		for _, img := range images {
			args = append(args, "--include-directories", filepath.Dir(img))
		}
		args = append(args, "-p", prompt)
		cmd = exec.CommandContext(ctx, b.GeminiPath, args...)

	case ProviderCodex:
		args := []string{}
		for _, img := range images {
			args = append(args, "--image", img)
		}
		args = append(args, "exec", "--skip-git-repo-check", prompt)
		cmd = exec.CommandContext(ctx, b.CodexPath, args...)

	case ProviderClaude:
		args := []string{}
		for _, img := range images {
			// claude code might take file using --file or similar, adjust if known
			args = append(args, "--file", img)
		}
		args = append(args, "-p", prompt)
		cmd = exec.CommandContext(ctx, b.ClaudePath, args...)

	default:
		return nil, fmt.Errorf("unsupported provider: %s", req.Provider)
	}

	// Disable interactivity/spinners from standard CLI behavior
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb", "CLICOLOR=0")
	return cmd, nil
}

// Stream executes the chosen CLI provider and streams the output directly
func (b *Bridge) Stream(ctx context.Context, req *Request, ch chan<- StreamEvent) {
	defer close(ch)

	cmd, err := b.BuildCommand(ctx, req)
	if err != nil {
		ch <- StreamEvent{Error: err}
		return
	}

	if b.Debug {
		fmt.Printf("Executing command: %s %s\n", cmd.Path, strings.Join(cmd.Args, " "))
	}

	// Start under PTY to ensure CLI unflushed outputs aren't buffered in a pipe
	ptmx, err := pty.Start(cmd)
	if err != nil {
		ch <- StreamEvent{Error: fmt.Errorf("failed to start pty: %v", err)}
		return
	}
	defer ptmx.Close()

	// Wait and cleanup routine
	go func() {
		_ = cmd.Wait()
	}()

	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			cmd.Process.Kill()
			ch <- StreamEvent{Error: ctx.Err()}
			return
		default:
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				cleanChunk := stripansi.Strip(chunk)
				if b.Debug {
					fmt.Printf("CHUNK: %q\n", cleanChunk)
				}
				ch <- StreamEvent{Content: cleanChunk}
			}
			if err != nil {
				if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "input/output error") {
					ch <- StreamEvent{Done: true}
					return
				}
				ch <- StreamEvent{Error: fmt.Errorf("read error: %v", err)}
				return
			}
		}
	}
}

// Execute is a wrapper around Stream to aggregate results into a single string
func (b *Bridge) Execute(ctx context.Context, req *Request) (string, error) {
	ch := make(chan StreamEvent)
	go b.Stream(ctx, req, ch)

	var sb strings.Builder
	for ev := range ch {
		if ev.Error != nil {
			return sb.String(), ev.Error
		}
		if ev.Done {
			break
		}
		sb.WriteString(ev.Content)
	}
	return sb.String(), nil
}
