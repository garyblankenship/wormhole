// sdk-tour runs isolated, deterministic SDK workflows without provider credentials.
package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"time"

	wormhole "github.com/garyblankenship/wormhole/v3"
	"github.com/garyblankenship/wormhole/v3/discovery"
	"github.com/garyblankenship/wormhole/v3/types"
)

//go:embed fixtures/config.json
var fixtureData []byte

type settings struct {
	TimeoutSeconds   int     `json:"timeout_seconds"`
	AgentSteps       int     `json:"agent_steps"`
	BatchConcurrency int     `json:"batch_concurrency"`
	CacheCapacity    int     `json:"cache_capacity"`
	TTLSeconds       int     `json:"ttl_seconds"`
	Retries          int     `json:"retries"`
	RetryDelayMS     int     `json:"retry_delay_ms"`
	RatePerSecond    int     `json:"rate_per_second"`
	Temperature      float32 `json:"temperature"`
	TopP             float32 `json:"top_p"`
	Seed             int     `json:"seed"`
	MaxTokens        int     `json:"max_tokens"`
	PollMS           int     `json:"poll_ms"`
	SafetySteps      int     `json:"safety_steps"`
	IdleMS           int     `json:"idle_ms"`
	Voice            string  `json:"voice"`
	AudioFormat      string  `json:"audio_format"`
	ImageSize        string  `json:"image_size"`
}

type fixtureConfig struct {
	Prompt     string
	System     string
	Answer     string
	Structured person
	Models     map[string]string
	Inputs     []string
	Vectors    [][]float64
	Documents  []string
	Scores     []float64
	Tool       struct {
		Name        string
		Description string
		Args        addArgs
		Result      addResult
	}
	Image    string
	Audio    string
	FakeKeys []string `json:"fake_keys"`
	Settings settings
}

type person struct {
	Name string `json:"name" tool:"required"`
	Age  int    `json:"age" tool:"required"`
}
type addArgs struct {
	A int `json:"a" tool:"required"`
	B int `json:"b" tool:"required"`
}
type addResult struct {
	Sum int `json:"sum"`
}

type scenario struct {
	name string
	use  func(context.Context, *journey) error
}
type journey struct {
	cfg     fixtureConfig
	fixture *upstream
	server  *httptest.Server
	clients []*wormhole.Wormhole
}

func newJourney(name string) (*journey, error) {
	j := &journey{}
	if err := json.Unmarshal(fixtureData, &j.cfg); err != nil {
		return nil, err
	}
	j.fixture = &upstream{cfg: j.cfg, mode: name}
	j.server = httptest.NewServer(http.HandlerFunc(j.fixture.serve))
	return j, nil
}

func (j *journey) close() error {
	errs := make([]error, 0, len(j.clients))
	for _, client := range j.clients {
		errs = append(errs, client.Close())
	}
	j.server.Close()
	return errors.Join(errs...)
}

func (j *journey) config(prefix string) types.ProviderConfig {
	return types.ProviderConfig{
		BaseURL: j.server.URL + prefix,
		NoAuth:  true, DynamicModels: true,
		HTTPTimeout: durationPointer(time.Duration(j.cfg.Settings.TimeoutSeconds) * time.Second),
		MaxRetries:  intPointer(0),
	}
}

func (j *journey) client(opts ...wormhole.Option) *wormhole.Wormhole {
	all := make([]wormhole.Option, 0, 8+len(opts))
	all = append(all,
		wormhole.WithOpenAI("", j.config("/v1")),
		wormhole.WithOpenAICompatible("backup", j.server.URL+"/v1", j.config("/v1")),
		wormhole.WithDefaultProvider("openai"),
		wormhole.WithModelValidation(false),
		wormhole.WithDiscovery(false),
		wormhole.WithDiscoveryConfig(discovery.DiscoveryConfig{DisableFileCache: true, DisableBackgroundRefresh: true}),
		wormhole.WithTimeout(time.Duration(j.cfg.Settings.TimeoutSeconds)*time.Second),
		wormhole.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	all = append(all, opts...)
	client := wormhole.New(all...)
	j.clients = append(j.clients, client)
	return client
}

func intPointer(v int) *int                          { return &v }
func durationPointer(v time.Duration) *time.Duration { return &v }
func (j *journey) ttl() time.Duration                { return time.Duration(j.cfg.Settings.TTLSeconds) * time.Second }
func (j *journey) text(client *wormhole.Wormhole) *wormhole.TextRequestBuilder {
	return client.Text().Model(j.cfg.Models["text"]).Prompt(j.cfg.Prompt)
}
func (j *journey) expectText(resp *types.TextResponse, err error) error {
	if err != nil {
		return err
	}
	if resp == nil || resp.Content() != j.cfg.Answer || !resp.IsComplete() {
		return fmt.Errorf("text content/completion mismatch: %+v", resp)
	}
	return nil
}
func decodeFixture(s string) ([]byte, error) { return base64.StdEncoding.DecodeString(s) }
func equalJSON(got, want any) bool {
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	return slices.Equal(g, w)
}
func require(ok bool, message string) error {
	if !ok {
		return errors.New(message)
	}
	return nil
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 1 && args[0] == "env-child" {
		return environmentChild(ctx, out)
	}
	selected := "all"
	if len(args) > 1 {
		return errors.New("usage: sdk-tour [all|list|scenario]")
	}
	if len(args) == 1 {
		selected = args[0]
	}
	scenarios := tourScenarios()
	if selected == "list" {
		for _, s := range scenarios {
			if _, err := fmt.Fprintln(out, s.name); err != nil {
				return err
			}
		}
		return nil
	}
	matched := false
	for _, s := range scenarios {
		if selected != "all" && selected != s.name {
			continue
		}
		matched = true
		if err := runScenario(ctx, s); err != nil {
			return fmt.Errorf("FAIL %s: %w", s.name, err)
		}
		if _, err := fmt.Fprintf(out, "PASS %s\n", s.name); err != nil {
			return err
		}
	}
	if !matched {
		return fmt.Errorf("unknown scenario %q; use list", selected)
	}
	_, err := fmt.Fprintln(out, "OWNED SERVERS CLOSED")
	return err
}

func runScenario(ctx context.Context, s scenario) (err error) {
	j, err := newJourney(s.name)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, j.close(), j.fixture.errors()) }()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(j.cfg.Settings.TimeoutSeconds)*time.Second)
	defer cancel()
	if err = s.use(ctx, j); err != nil {
		return err
	}
	return j.fixture.errors()
}
