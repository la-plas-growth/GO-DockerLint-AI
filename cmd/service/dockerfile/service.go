package dockerfile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anaskhan96/soup"
	"github.com/la-plas-growth/GO-DockerLint-AI/env"
	"github.com/la-plas-growth/GO-DockerLint-AI/lib"
	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

// IService defines the interface for the dockerfile service
type IService interface {
	CreateDockerFile(lang string) (*DockerfileResponse, error)
}

// service implements the IService interface
type service struct {
	configuration *env.Configuration
	logger        *zap.SugaredLogger
	client        *openai.Client
}

// NewService creates a new instance of the linting service
func NewService(configuration *env.Configuration, logger *zap.SugaredLogger) IService {
	if configuration.OpenAIAPIKey == "" {
		logger.Panic("OpenAI API key is required")
	}
	client := openai.NewClient(configuration.OpenAIAPIKey)
	return &service{
		configuration: configuration,
		logger:        logger,
		client:        client,
	}
}

// fetch bestpractice from html
func (s *service) FetchBestPracticesMarkdown() (string, error) {
	s.logger.Debugf("Fetching best practices from URL: %s", s.configuration.BestPracticesURL)
	resp, err := soup.Get(s.configuration.BestPracticesURL)
	//
	if err != nil {
		s.logger.Errorf("Unexpected status code: %d", err)
		return "", fmt.Errorf("unexpected status code: %d", err)
	}
	doc := soup.HTMLParse(resp)
	//
	return doc.FullText(), nil
}

// Create the finale Dockerfile
func (s *service) CreateDockerFile(lang string) (*DockerfileResponse, error) {
	s.logger.Debugf("Create DockerFile with OpenAI for lang: %s", lang)
	//
	resp, err := s.getDockerfileWithChatGPT(lang)
	if err != nil {
		s.logger.Errorf("Failed to getDockerfileWithChatGPT: %v", err)
		return nil, err
	}
	// create dockerfile in filesystem
	err = lib.WriteFile("Dockerfile", resp.Dockerfile)
	if err != nil {
		s.logger.Errorf("Failed to WriteFile: %v", err)
		return nil, err
	}
	return resp, nil
}

func (s *service) getDockerfileWithChatGPT(lang string) (*DockerfileResponse, error) {
	// Create OpenAI API context
	ctx := context.Background()

	// Construct the prompt for OpenAI
	prompt := fmt.Sprintf(`Create a Dockerfile for this language: %s`, lang)
	s.logger.Debugf("prompt: %s", prompt)

	// Fetch best practices content
	bestPractices, err := s.FetchBestPracticesMarkdown()
	if err != nil {
		s.logger.Errorf("Failed to fetch best practices: %v", err)
		return nil, err
	}

	cleanedBestPractices := strings.ReplaceAll(bestPractices, "\n", " ")

	// Construct the system prompt
	sysprompt := fmt.Sprintf(`You are an expert Dockerfile creator who strictly adheres to the following best practices: %s`, cleanedBestPractices)

	// Define the JSON schema for the response
	schema := createSchemaGpt()

	// Serialize the schema into JSON
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		s.logger.Errorf("Failed to serialize schema: %v", err)
		return nil, fmt.Errorf("failed to serialize schema: %w", err)
	}

	// Define the response format
	responseFormat := &openai.ChatCompletionResponseFormat{
		Type: "json_schema",
		JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
			Name:        "Dockerfile",
			Description: "Dockerfile for this language " + lang,
			Schema:      json.RawMessage(schemaBytes),
			Strict:      true,
		},
	}
	// Call OpenAI API
	resp, err := s.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: "gpt-4o-mini",
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "system",
				Content: sysprompt,
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		ResponseFormat: responseFormat,
	})
	if err != nil {
		s.logger.Errorf("Error communicating with OpenAI: %v", err)
		return nil, err
	}

	// Parse the response
	responseContent := resp.Choices[0].Message.Content
	s.logger.Debugf("OpenAI response: %s", responseContent)

	// Decode the JSON response into the DockerfileResponse structure
	var respDockerfile DockerfileResponse
	if err := json.Unmarshal([]byte(responseContent), &respDockerfile); err != nil {
		s.logger.Errorf("Failed to parse JSON response: %v", err)
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}
	return &respDockerfile, nil
}

func createSchemaGpt() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"issues": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"issue":    map[string]string{"type": "string"},
						"severity": map[string]string{"type": "string"},
						"advice":   map[string]string{"type": "string"},
					},
					"required":             []string{"issue", "severity", "advice"},
					"additionalProperties": false,
				},
			},
		},
		"required":             []string{"issues"},
		"additionalProperties": false,
	}
}
