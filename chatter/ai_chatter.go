package chatter

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func tokenTrackFilePath() string {
	if p := os.Getenv("TOKEN_TRACKER_FILE"); p != "" {
		return p
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		return filepath.Join(cacheDir, "konda", "tokenTracker.txt")
	}
	return filepath.Join(os.TempDir(), "konda-tokenTracker.txt")
}

type Chatter interface {
	Chat(messages []string, options *ChatOptions) (response string, tokensUsed int, err error)
	Upload(filePath string) (string, error)
}

type AIChatter struct {
	chatOptions *ChatOptions
	client      Chatter
	// TODO: fix system and user messages
	messages []string
}

type ChatOptions struct {
	Model         string
	SystemMessage string
	Temperature   float32
}

func NewAIChatter(client Chatter, options *ChatOptions) *AIChatter {
	return &AIChatter{client: client, chatOptions: options, messages: nil}
}

func (a *AIChatter) Chat(prompt string, options *ChatOptions) (string, error) {
	// TODO: How much context do we want to keep? This needs to be configured with each model and each API
	if chatterDisabled() {
		return "", fmt.Errorf("chatter is disabled")
	}

	if options == nil {
		options = a.chatOptions
	}

	a.messages = append(a.messages, prompt)

	reply, tokens, err := a.client.Chat(a.messages, options)
	if err != nil {
		return "", err
	}

	saveTokensUsed(tokens)

	a.messages = append(a.messages, prompt)
	a.messages = append(a.messages, reply)
	// fmt.Println("--------------")
	// fmt.Println("Prompt:")
	// fmt.Println(prompt)
	// fmt.Println("--------------")
	// fmt.Println("Response:")
	// fmt.Println(reply)
	// fmt.Println("--------------")
	// fmt.Println("Tokens used:", tokens)
	// fmt.Println("--------------")
	return reply, nil
}

func (a *AIChatter) Upload(filePath string) (string, error) {
	fmt.Println("Uploading file", filePath, "...")
	return a.client.Upload(filePath)
}

func saveTokensUsed(tokens int) {
	tokenTrackFile := tokenTrackFilePath()
	_ = os.MkdirAll(filepath.Dir(tokenTrackFile), 0o755)

	// Create file if it does not exist
	if _, err := os.Stat(tokenTrackFile); os.IsNotExist(err) {
		// If file does not exist, create it with an initial value of 0
		fmt.Println("TokenTrackerFile does not exist. Creating TokenTrackerFile with initial value 0.")
		err := os.WriteFile(tokenTrackFile, []byte("0"), 0644)
		if err != nil {
			fmt.Printf("Failed to create TokenTrackerFile: %v\n", err)
			return
		}
	}

	// Read the file
	data, err := os.ReadFile(tokenTrackFile)
	if err != nil {
		fmt.Printf("Failed to read TokenTrackerFile: %v\n", err)
		return
	}

	// Convert the content to an integer
	content := strings.TrimSpace(string(data)) // Remove any extra spaces or newlines
	number, err := strconv.Atoi(content)
	if err != nil {
		fmt.Printf("Failed to parse number: %v\n", err)
		return
	}

	// Add a number
	number += tokens

	// Save it back to the file
	err = os.WriteFile(tokenTrackFile, fmt.Appendf(nil, "%d", number), 0644)
	if err != nil {
		fmt.Printf("Failed to write to TokenTrackerFile: %v\n", err)
		return
	}
}

func chatterDisabled() bool {
	return false
}
