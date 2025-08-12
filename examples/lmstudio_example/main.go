package main

import (
	"context"
	"fmt"
	"log"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	// Create a new Prism client
	p := wormhole.New(wormhole.Config{})

	// Add LMStudio provider (default: http://localhost:1234/v1)
	p.WithLMStudio(types.ProviderConfig{})

	// You can also use a custom configuration
	// p.WithLMStudio(types.ProviderConfig{
	//     BaseURL: "http://192.168.1.100:1234/v1", // Custom LMStudio server
	//     Timeout: 60, // 60 seconds timeout
	// })

	// Example 1: Simple text generation
	fmt.Println("=== Simple Text Generation ===")
	response, err := p.Text().
		Using("lmstudio").
		Model("local-model"). // Use whatever model you have loaded in LMStudio
		Prompt("Write a short poem about AI").
		Temperature(0.7).
		MaxTokens(100).
		Generate(context.Background())

	if err != nil {
		log.Printf("Text generation error: %v", err)
	} else {
		fmt.Printf("Response: %s\n\n", response.Text)
	}

	// Example 2: Streaming text generation
	fmt.Println("=== Streaming Text Generation ===")
	stream, err := p.Text().
		Using("lmstudio").
		Model("local-model").
		Prompt("Tell me a story about a robot").
		Temperature(0.8).
		Stream(context.Background())

	if err != nil {
		log.Printf("Streaming error: %v", err)
	} else {
		fmt.Print("Streaming response: ")
		for chunk := range stream {
			if chunk.Error != nil {
				log.Printf("Stream error: %v", chunk.Error)
				break
			}
			fmt.Print(chunk.Text)
		}
		fmt.Println()
	}

	// Example 3: Function calling / Tools (if your model supports it)
	fmt.Println("=== Function Calling ===")
	weatherTool := types.NewTool(
		"get_weather",
		"Get the current weather for a location",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"location": map[string]interface{}{
					"type":        "string",
					"description": "The city name",
				},
				"unit": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"celsius", "fahrenheit"},
					"description": "Temperature unit",
				},
			},
			"required": []string{"location"},
		},
	)

	toolResponse, err := p.Text().
		Using("lmstudio").
		Model("local-model").
		Prompt("What's the weather like in San Francisco?").
		Tools(*weatherTool).
		Generate(context.Background())

	if err != nil {
		log.Printf("Tool calling error: %v", err)
	} else {
		fmt.Printf("Response: %s\n", toolResponse.Text)
		if len(toolResponse.ToolCalls) > 0 {
			for _, toolCall := range toolResponse.ToolCalls {
				fmt.Printf("Tool called: %s with arguments: %+v\n", toolCall.Name, toolCall.Arguments)
			}
		}
		fmt.Println()
	}

	// Example 4: Structured output (JSON mode)
	fmt.Println("=== Structured Output ===")
	type Person struct {
		Name    string   `json:"name"`
		Age     int      `json:"age"`
		City    string   `json:"city"`
		Hobbies []string `json:"hobbies"`
	}

	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Person's full name",
			},
			"age": map[string]interface{}{
				"type":        "integer",
				"description": "Person's age",
			},
			"city": map[string]interface{}{
				"type":        "string",
				"description": "City where the person lives",
			},
			"hobbies": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
				"description": "List of hobbies",
			},
		},
		"required": []string{"name", "age", "city"},
	}

	var person Person
	err = p.Structured().
		Using("lmstudio").
		Model("local-model").
		Prompt("Generate details for a fictional person who is a software engineer").
		Schema(schema).
		GenerateAs(context.Background(), &person)

	if err != nil {
		log.Printf("Structured generation error: %v", err)
	} else {
		fmt.Printf("Generated person: %+v\n\n", person)
	}

	// Example 5: Using conversation with messages
	fmt.Println("=== Conversation with Messages ===")
	messages := []types.Message{
		types.NewSystemMessage("You are a helpful coding assistant"),
		types.NewUserMessage("How do I reverse a string in Go?"),
	}

	conversationResponse, err := p.Text().
		Using("lmstudio").
		Model("local-model").
		Messages(messages...).
		MaxTokens(200).
		Generate(context.Background())

	if err != nil {
		log.Printf("Conversation error: %v", err)
	} else {
		fmt.Printf("Assistant: %s\n", conversationResponse.Text)
	}

	fmt.Println("\n=== LMStudio Example Complete ===")
}
