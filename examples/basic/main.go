package main

import (
	"context"
	"fmt"
	"log"

	bridge "github.com/shachiku-ai/shachiku-cli-bridge"
)

func main() {
	b := bridge.NewBridge()
	// Optionally override paths if they aren't globally in $PATH
	// b.GeminiPath = "/usr/local/bin/gemini"
	b.Debug = true

	req := &bridge.Request{
		Provider: bridge.ProviderGemini,
		Messages: []bridge.Message{
			{
				Role:    "user",
				Content: "Hello, what is 2+2?",
			},
		},
	}

	// 1. Synchronous Execution
	out, err := b.Execute(context.Background(), req)
	if err != nil {
		log.Fatalf("Execute failed: %v", err)
	}
	fmt.Printf("Sync Response:\n%s\n", out)

	// 2. Stream Execution (SSE like functionality context)
	req.Messages[0].Content = "Write a haiku about go routines"

	ch := make(chan bridge.StreamEvent)
	go b.Stream(context.Background(), req, ch)

	fmt.Println("Streaming Response:")
	for ev := range ch {
		if ev.Error != nil {
			log.Fatalf("Stream error: %v", ev.Error)
		}
		if ev.Done {
			fmt.Println("\n[Stream Done]")
			break
		}
		fmt.Print(ev.Content)
	}
}
