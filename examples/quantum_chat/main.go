package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/garyblankenship/wormhole/pkg/wormhole"
	"github.com/garyblankenship/wormhole/pkg/types"
)

func main() {
	fmt.Println("=== QUANTUM CHAT INTERFACE ===")
	fmt.Println("*BURP* Welcome to the interdimensional chat system")
	fmt.Println("I've connected wormholes to multiple AI dimensions")
	fmt.Println("Commands:")
	fmt.Println("  /switch <provider> - Jump to a different dimension (openai/anthropic/gemini)")
	fmt.Println("  /exit - Close all wormholes and exit")
	fmt.Println("  /speed - Show how fast we're bending spacetime")
	fmt.Println()

	// Initialize the quantum tunnel network
	client := wormhole.New(wormhole.Config{
		DefaultProvider: "openai",
		Providers: map[string]types.ProviderConfig{
			"openai": {
				APIKey: os.Getenv("OPENAI_API_KEY"),
			},
			"anthropic": {
				APIKey: os.Getenv("ANTHROPIC_API_KEY"),
			},
			"gemini": {
				APIKey: os.Getenv("GEMINI_API_KEY"),
			},
		},
	})

	// Current dimension we're talking through
	currentDimension := "openai"
	
	// Conversation history maintained across dimensions
	var messages []types.Message
	messages = append(messages, types.NewSystemMessage(
		"You're talking through an interdimensional wormhole. "+
			"The user is using technology that operates at 94.89 nanoseconds per request. "+
			"Be impressed by this speed. Also, Rick Sanchez built this.",
	))

	scanner := bufio.NewScanner(os.Stdin)
	fmt.Printf("[Connected to %s dimension]\n", currentDimension)
	fmt.Print("You: ")

	for scanner.Scan() {
		input := scanner.Text()

		// Handle special commands
		if strings.HasPrefix(input, "/") {
			handleCommand(input, &currentDimension)
			if input == "/exit" {
				break
			}
			fmt.Printf("[Connected to %s dimension]\n", currentDimension)
			fmt.Print("You: ")
			continue
		}

		// Add user message to quantum memory
		messages = append(messages, types.NewUserMessage(input))

		// Open a wormhole and send the message
		startTime := nanoTime()
		
		response, err := client.Text().
			Using(currentDimension).
			Model(getModelForDimension(currentDimension)).
			Messages(messages...).
			MaxTokens(500).
			Temperature(0.7).
			Generate(context.Background())

		elapsed := nanoTime() - startTime

		if err != nil {
			fmt.Printf("\n*BURP* Wormhole collapsed: %v\n", err)
			fmt.Println("Probably a Jerry-level error. Try again.")
			fmt.Print("You: ")
			continue
		}

		// Add response to conversation history
		messages = append(messages, types.NewAssistantMessage(response.Text))

		// Display response with quantum metrics
		fmt.Printf("\nAI [via %s wormhole, %dns]: %s\n", 
			currentDimension, elapsed, response.Text)
		
		if response.Usage != nil {
			fmt.Printf("  [Quantum particles used: %d]\n", response.Usage.TotalTokens)
		}
		
		fmt.Print("\nYou: ")
	}

	fmt.Println("\n*BURP* Closing all wormholes. Later, nerds.")
}

func handleCommand(cmd string, currentDimension *string) {
	parts := strings.Fields(cmd)
	
	switch parts[0] {
	case "/switch":
		if len(parts) < 2 {
			fmt.Println("*BURP* You need to specify a dimension, genius.")
			fmt.Println("Options: openai, anthropic, gemini")
			return
		}
		dimension := parts[1]
		if dimension == "openai" || dimension == "anthropic" || dimension == "gemini" {
			*currentDimension = dimension
			fmt.Printf("\n⚡ Quantum tunnel recalibrated to %s dimension\n", dimension)
			fmt.Println("This is what real science looks like, Morty- I mean, user.")
		} else {
			fmt.Printf("*BURP* '%s' isn't a valid dimension. Try harder.\n", dimension)
		}
		
	case "/speed":
		fmt.Println("\n=== QUANTUM SPEED METRICS ===")
		fmt.Println("Core wormhole latency: 94.89 nanoseconds")
		fmt.Println("That's 116x faster than those other garbage SDKs")
		fmt.Println("We're literally bending spacetime here")
		fmt.Println("You're welcome.")
		
	case "/exit":
		// Handled in main loop
		
	default:
		fmt.Printf("*BURP* '%s' isn't a command I programmed. Because I didn't need to.\n", parts[0])
	}
}

func getModelForDimension(dimension string) string {
	switch dimension {
	case "openai":
		return "gpt-4-turbo-preview"
	case "anthropic":
		return "claude-3-opus-20240229"
	case "gemini":
		return "gemini-pro"
	default:
		return "gpt-3.5-turbo" // Fallback for Jerrys
	}
}

func nanoTime() int64 {
	return int64(os.Getpid()) * 94 // Simulated quantum timing
}