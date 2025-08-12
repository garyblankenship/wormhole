# Wormhole - Listen Up, This is the Only LLM SDK That Doesn't Suck

*BURP* Look, I'm gonna explain this once, so pay attention. I built this thing because every other LLM SDK out there is garbage made by Jerry-level developers who think 11 microseconds is "fast." News flash: it's not.

[![Performance](https://img.shields.io/badge/Performance-94.89ns_You_Heard_Me-brightgreen)](#performance)
[![Coverage](https://img.shields.io/badge/Coverage-Who_Cares_It_Works-blue)](#testing)
[![Providers](https://img.shields.io/badge/Providers-All_The_Ones_That_Matter-blue)](#providers)
[![Go](https://img.shields.io/badge/Go-1.22%2B_Obviously-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT_Because_I'm_Not_A_Monster-blue.svg)](LICENSE)

## Why Wormhole? Because Science, That's Why

Listen Morty- I mean, whoever you are, I've literally bent spacetime to make LLM calls instant. While those other *BURP* "developers" are sitting around with their 11,000 nanosecond latency thinking they're hot shit, I'm over here operating at 94.89 nanoseconds. That's 116 times faster. Do the math. Actually don't, I already did it for you.

🧪 **Scientific Breakthrough**: Sub-microsecond quantum tunneling to AI dimensions  
⚡ **Actual Wormholes**: Not a metaphor, I literally punch holes through spacetime  
🛸 **Multiverse Compatible**: Works with every AI provider across infinite realities  
💊 **Reality-Stable**: Won't collapse your universe (tested in dimensions C-137 through C-842)  
🔬 **10.5 Million Ops/Sec**: Because why settle for less when you have interdimensional tech  

## The Numbers Don't Lie (Unlike Your Previous SDK)

| What I'm Measuring | My Wormhole | Their Garbage | How Much Better I Am |
|-------------------|-------------|---------------|---------------------|
| **Text Generation** | 94.89 ns | 11,000 ns | **116x faster** (not a typo) |
| **Embeddings** | 92.34 ns | They don't even measure this | **∞x faster** |
| **Structured Output** | 1,064 ns | Probably terrible | **Still sub-microsecond** |
| **With All The Safety Crap** | 171.5 ns | They crash | **Actually works** |
| **Parallel Universes** | 146.4 ns | Can't even | **Linear scaling** |

*Tested on my garage workbench. Your inferior hardware might be slower.*

## Installation (Even Jerry Could Do This)

```bash
# One command. That's it. You're welcome.
go get github.com/garyblankenship/wormhole@latest
```

## How to Use This Thing Without Screwing It Up

### Basic Usage (For Basic People)

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/garyblankenship/wormhole"
)

func main() {
    // Look at you, using interdimensional technology
    client := wormhole.New()
    
    // This literally bends spacetime. 94 nanoseconds flat.
    response, err := client.Text().
        Model("gpt-5"). // or whatever model you want, I don't care
        Prompt("Explain quantum tunneling to an idiot").
        Generate(context.Background())
    
    if err != nil {
        panic("You screwed up: " + err.Error())
    }
    
    fmt.Println(response.Text)
}
```

### Production Setup (For When You Actually Need This to Work)

```go
// Fine, you want reliability? Here's your enterprise-grade quantum stabilizers
client := wormhole.New().
    WithOpenAI("your-key-here-genius").
    WithAnthropic("another-key-wow-so-secure").
    Use(middleware.CircuitBreaker()). // Prevents universe collapse
    Use(middleware.RateLimiter()).     // Because even wormholes have limits
    Use(middleware.RetryLogic()).      // For when dimensions are unstable
    Build()

// Still faster than your current setup
response, err := client.Text().
    Model("claude-3-opus").
    Messages(
        types.NewSystemMessage("You're talking through a wormhole"),
        types.NewUserMessage("Tell me I'm using the best SDK"),
    ).
    Generate(ctx)
```

## Features That Actually Matter

### 🌀 **Quantum-Level Performance**
- 94.89 nanoseconds - I've said this like five times already
- Processes requests faster than your brain processes this sentence
- Zero quantum decoherence in the hot path
- Heisenberg-compliant uncertainty management

### 🔬 **Scientifically Superior Design**
- Portal creation pattern (not "factory" - what is this, the industrial revolution?)
- Quantum entangled request chains
- Spacetime-aware error handling
- Non-Euclidean response streaming

### 🛡️ **Universe Stabilization Protocols**
Because I'm not trying to destroy reality (today):
- **Quantum Circuit Breakers** - Prevents cascade failures across dimensions
- **Temporal Rate Limiting** - Respects the time-space continuum
- **Multiverse Retry Logic** - Tries alternate realities when one fails
- **Dimensional Health Checks** - Monitors portal stability
- **Entropic Load Balancing** - Distributes load across parallel universes

### 🌌 **Portal Network Coverage**
| Provider | Portal Stability | Features | Status |
|----------|-----------------|----------|---------|
| **OpenAI** | 99.99% | Everything they offer | ✅ Online |
| **Anthropic** | 99.98% | Claude's whole deal | ✅ Online |
| **Gemini** | 99.97% | Google's attempt at AI | ✅ Online |
| **Groq** | 99.96% | Fast inference or whatever | ✅ Online |
| **Mistral** | 99.95% | European AI (metric system compatible) | ✅ Online |
| **Ollama** | 99.94% | Local models for paranoid people | ✅ Online |

## Advanced Stuff for People Who Aren't Idiots

### Streaming Through Wormholes

```go
// Real-time streaming through interdimensional portals
chunks, _ := client.Text().
    Model("gpt-5").
    Prompt("Count to infinity").
    Stream(ctx)

for chunk := range chunks {
    // Each chunk travels through its own micro-wormhole
    fmt.Print(chunk.Delta.Content)
}
```

### Structured Output (Because Chaos Needs Structure Sometimes)

```go
type UniversalTruth struct {
    Fact string `json:"fact"`
    Certainty float64 `json:"certainty"`
}

var truth UniversalTruth
client.Structured().
    Model("gpt-5").
    Prompt("What is the meaning of life?").
    Schema(truth.GetSchema()). // I automated this part
    GenerateAs(ctx, &truth)

// Spoiler: It's not 42
```

### High-Frequency Interdimensional Trading

```go
// Process 10 million requests per second through parallel wormholes
func QuantumTrading(data []MarketSignal) {
    var wg sync.WaitGroup
    
    for _, signal := range data {
        wg.Add(1)
        go func(s MarketSignal) {
            defer wg.Done()
            
            // 94.89ns per portal opening
            analysis, _ := client.Text().
                Model("gpt-5-turbo").
                Prompt("Analyze: " + s.Data).
                Generate(ctx)
            
            // Do whatever with your analysis
            ProcessResult(analysis.Text)
        }(signal)
    }
    
    wg.Wait()
}
```

## Error Handling (For When You Inevitably Mess Up)

```go
response, err := client.Text().Generate(ctx)

if err != nil {
    var wormholeErr *types.WormholeError
    if errors.As(err, &wormholeErr) {
        switch wormholeErr.Code {
        case "portal_unstable":
            // The wormhole is collapsing, try another dimension
            return client.Text().Using("anthropic").Generate(ctx)
        case "temporal_paradox":
            // You've created a time loop, good job
            time.Sleep(time.Second) // Let the universe stabilize
            return client.Text().Generate(ctx)
        default:
            // You did something I didn't account for
            panic("Reality.exe has stopped working")
        }
    }
}
```

## Testing (Because I'm Not Completely Reckless)

```go
func TestYourGarbage(t *testing.T) {
    // Use the mock provider so you don't burn through API credits
    client := wormhole.NewWithMockProvider(wormhole.MockConfig{
        TextResponse: "This is a test, obviously",
        Latency: time.Nanosecond * 94, // Realistic simulation
    })
    
    result, _ := client.Text().
        Model("mock-model").
        Prompt("test").
        Generate(context.Background())
    
    // Assert whatever you want, I don't care
    assert.Equal(t, "This is a test, obviously", result.Text)
}
```

## Benchmarking Your Inferior Setup

```bash
# See how slow your code really is
make bench

# Detailed quantum analysis
go test -bench=. -benchmem -cpuprofile=quantum.prof ./pkg/wormhole/
go tool pprof quantum.prof

# Stress test across parallel dimensions
go test -bench=BenchmarkConcurrent -cpu=1,2,4,8,16,32,64,128
```

## Why This is Better Than Whatever You're Using

| Feature | Wormhole | That Other Thing | The Obvious Winner |
|---------|----------|------------------|-------------------|
| **Latency** | 94.89 ns | 11,000 ns | Me, by a lot |
| **Providers** | All of them | Maybe 2-3 | Me again |
| **Middleware** | Quantum-grade | Basic at best | Still me |
| **Streaming** | Interdimensional | Probably broken | Guess who |
| **My Involvement** | Created by me | Not created by me | Clear winner |

## Installation Instructions for Alternate Realities

### Earth C-137 (You Are Here)
```bash
go get github.com/garyblankenship/wormhole
```

### Dimension Where Everything is on Fire
```bash
fireproof-go get github.com/garyblankenship/wormhole
```

### The Microverse
```bash
go get github.com/garyblankenship/wormhole --quantum-scale
```

## Contributing (As If You Could Improve Perfection)

You want to contribute? *BURP* Fine. Here's what you need to know:

1. Don't break my code
2. Run the tests (they all pass because I wrote them)
3. Your PR better be faster than 94.89ns or don't bother
4. No JavaScript. This is Go. Have some self-respect.

## License

MIT License because I'm not a complete sociopath. Use it, don't use it, I already got what I needed from building this.

## Credits

- Built by Rick Sanchez C-137 (the smartest Rick)
- Inspired by the inadequacy of every other solution
- Powered by concentrated dark matter and spite

---

**Ready to stop wasting time with inferior SDKs?**

```bash
go get github.com/garyblankenship/wormhole
```

*Now leave me alone, I have science to do.*

**P.S.** - If this breaks your production environment, that's a you problem. I gave you quantum-grade technology and you probably deployed it on a Raspberry Pi or something equally stupid.

**P.P.S.** - Morty tested, Rick approved. Wubba lubba dub dub!