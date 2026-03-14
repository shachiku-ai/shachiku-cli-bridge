package bridge

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildCommand(t *testing.T) {
	b := NewBridge()
	b.GeminiPath = "gemini"
	b.CodexPath = "codexcli"
	b.ClaudePath = "claude"

	tests := []struct {
		name     string
		req      *Request
		wantArgs []string
		wantErr  bool
	}{
		{
			name: "Gemini text only",
			req: &Request{
				Provider: ProviderGemini,
				Messages: []Message{{Role: "user", Content: "Hello"}},
			},
			wantArgs: []string{"gemini", "-p", "Hello"},
		},
		{
			name: "Gemini text and file",
			req: &Request{
				Provider: ProviderGemini,
				Files:    []string{"testdata/test.jpg"},
				Messages: []Message{{Role: "user", Content: "Analyze this image"}},
			},
			wantArgs: []string{"gemini", "--include-directories", "testdata", "-p", "Analyze this image"},
		},
		{
			name: "Gemini text and multiple files",
			req: &Request{
				Provider: ProviderGemini,
				Files:    []string{"testdata/img1.png", "testdata/img2.jpg"},
				Messages: []Message{{Role: "user", Content: "Analyze these"}},
			},
			wantArgs: []string{"gemini", "--include-directories", "testdata", "--include-directories", "testdata", "-p", "Analyze these"},
		},
		{
			name: "Codex text and file",
			req: &Request{
				Provider: ProviderCodex,
				Files:    []string{"testdata/code.png"},
				Messages: []Message{{Role: "user", Content: "What is this?"}},
			},
			wantArgs: []string{"codexcli", "--image", "testdata/code.png", "exec", "--skip-git-repo-check", "What is this?"},
		},
		{
			name: "Claude text and file",
			req: &Request{
				Provider: ProviderClaude,
				Files:    []string{"testdata/server.log"},
				Messages: []Message{{Role: "user", Content: "Summarize this log"}},
			},
			wantArgs: []string{"claude", "-p", "Summarize this log\n\nFiles: testdata/server.log"},
		},
		{
			name: "System Prompt Inclusion",
			req: &Request{
				Provider:     ProviderGemini,
				SystemPrompt: "You are a helpful assistant.",
				Messages:     []Message{{Role: "user", Content: "Hi"}},
			},
			wantArgs: []string{"gemini", "-p", "System: You are a helpful assistant.\n\nUser: Hi"},
		},
		{
			name: "Error no user message",
			req: &Request{
				Provider: ProviderGemini,
				Messages: []Message{{Role: "assistant", Content: "How can I help you?"}},
			},
			wantErr: true,
		},
		{
			name: "Error empty messages",
			req: &Request{
				Provider: ProviderGemini,
				Messages: []Message{},
			},
			wantErr: true,
		},
		{
			name: "Unsupported provider",
			req: &Request{
				Provider: "unknown_provider",
				Messages: []Message{{Role: "user", Content: "Test"}},
			},
			wantErr: true,
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := b.BuildCommand(ctx, tt.req)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// We extract command path + args to compare easily
			var actualArgs []string
			if strings.HasSuffix(cmd.Path, tt.wantArgs[0]) {
				// command matches
				actualArgs = append([]string{tt.wantArgs[0]}, cmd.Args[1:]...)
			} else {
				actualArgs = cmd.Args
			}

			if !reflect.DeepEqual(actualArgs, tt.wantArgs) {
				t.Errorf("BuildCommand args:\ngot  = %v\nwant = %v", actualArgs, tt.wantArgs)
			}

			// Test that NO_COLOR format flag is added
			envMatched := false
			for _, e := range cmd.Env {
				if e == "NO_COLOR=1" {
					envMatched = true
				}
			}
			if !envMatched {
				t.Errorf("Expected NO_COLOR=1 in command environment but not found")
			}
		})
	}
}

func TestExecuteAndStream(t *testing.T) {
	mockScript, _ := filepath.Abs("testdata/mock_cli.sh")
	// Make sure the mock script is present
	if _, err := exec.LookPath(mockScript); err != nil {
		t.Skip("mock_cli.sh is not executable or not found, skipping integration test")
	}

	b := NewBridge()
	// Override all providers to point at our mock binary
	b.GeminiPath = mockScript
	b.CodexPath = mockScript
	b.ClaudePath = mockScript
	// enable debug to test printing slightly
	b.Debug = true

	req := &Request{
		Provider: ProviderGemini,
		Files:    []string{"mock.jpg"},
		Messages: []Message{
			{Role: "user", Content: "Here is to testing streaming"},
		},
	}

	t.Run("Execute Synchronous", func(t *testing.T) {
		out, err := b.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if strings.Contains(out, "\x1b[31m") {
			t.Errorf("Output should have been stripped of ANSI colors but contains them")
		}

		if !strings.Contains(out, "Colored Text") {
			t.Errorf("Missing expected colored text stream. Output: %s", out)
		}

		wantArgs := "Received args: --include-directories . -p Here is to testing streaming"
		if !strings.Contains(out, wantArgs) {
			t.Errorf("Missing proper argument pass-down.\nGot: %s", out)
		}

		if !strings.Contains(out, "Stream testing done.") {
			t.Errorf("Missing final stream part. Output: %s", out)
		}
	})

	t.Run("Stream Asynchronous", func(t *testing.T) {
		ch := make(chan StreamEvent)
		go b.Stream(context.Background(), req, ch)

		var sb strings.Builder
		for ev := range ch {
			if ev.Error != nil && !ev.Done {
				t.Fatalf("Stream unexpected error: %v", ev.Error)
			}
			if ev.Done {
				break
			}
			sb.WriteString(ev.Content)
		}

		out := sb.String()

		if strings.Contains(out, "\x1b[") {
			t.Errorf("Async stream Output should have been stripped of ANSI colors but contains them")
		}

		if !strings.Contains(out, "Colored Text") {
			t.Errorf("Stream missing expected colored text stream")
		}
	})
}
