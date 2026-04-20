package azureOpenAI

import (
	"context"
	"fmt"
	"os"

	"git.rwth-aachen.de/dsma/publications/software/konda/chatter"
	"github.com/Azure/azure-sdk-for-go/sdk/ai/azopenai"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

type azureClient struct {
	sdkClient *azopenai.Client
}

func NewAzureClient(apiEndpoint, apiKey string) (*azureClient, error) {
	if apiEndpoint == "" || apiKey == "" {
		return nil, fmt.Errorf("missing parameters")
	}

	keyCredential := azcore.NewKeyCredential(apiKey)
	client, err := azopenai.NewClientWithKeyCredential(apiEndpoint, keyCredential, nil)

	if err != nil {
		return nil, err
	}

	return &azureClient{client}, nil
}

func (a *azureClient) Chat(messages []string, options *chatter.ChatOptions) (response string, tokensUsed int, err error) {
	reply := ""

	// TODO: fix todo context
	resp, err := a.sdkClient.GetChatCompletions(context.TODO(), azopenai.ChatCompletionsOptions{
		Messages:       convertMessages(messages, options.SystemMessage),
		Temperature:    &options.Temperature,
		DeploymentName: &options.Model,
	}, nil)

	if err != nil {
		return "", 0, err
	}

	for _, choice := range resp.Choices {
		// Typically we only have one choice. This simplifies things..
		if choice.Message != nil && choice.Message.Content != nil {
			reply = *choice.Message.Content
		}
	}

	return reply, int(*resp.Usage.TotalTokens), nil
}

func convertMessages(messages []string, systemMessage string) []azopenai.ChatRequestMessageClassification {
	// TODO: this is disgusting - handle your message types properly
	// sysMesOffset := 1
	azMessages := make([]azopenai.ChatRequestMessageClassification, 2)

	if systemMessage != "" {
		azMessages[0] = &azopenai.ChatRequestSystemMessage{Content: azopenai.NewChatRequestSystemMessageContent(systemMessage)}
	}

	azMessages[1] = &azopenai.ChatRequestUserMessage{Content: azopenai.NewChatRequestUserMessageContent(messages[len(messages)-1])}
	// for i, message := range messages {
	// 	azMessages[i+sysMesOffset] = &azopenai.ChatRequestUserMessage{Content: azopenai.NewChatRequestUserMessageContent(message)}
	// }

	return azMessages
}

func (a *azureClient) Upload(file_path string) (response string, err error) {
	file, err := os.Open(file_path)
	if err != nil {
		return "", err
	}
	resp, err := a.sdkClient.UploadFile(context.TODO(), file, azopenai.FilePurposeAssistants, nil)
	if err != nil {
		return "", err
	}

	return *resp.ID, nil
}
