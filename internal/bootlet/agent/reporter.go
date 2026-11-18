package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

type Client struct {
	httpClient        *http.Client
	baseURL           string
	requestMiddleware []func(*http.Request) error
}

func BearerAuth(token string) func(*http.Request) error {
	return func(r *http.Request) error {
		r.Header.Add("Authorization", "Bearer "+token)
		return nil
	}
}

func NewClient(baseURL string, middleware ...func(r *http.Request) error) *Client {
	return &Client{
		httpClient:        http.DefaultClient,
		baseURL:           baseURL,
		requestMiddleware: middleware,
	}
}

type Report struct {
	Phase   string `json:"phase"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func (c *Client) Report(ctx context.Context, report *Report) error {
	data, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/agents/report", c.baseURL), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating report request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	for _, middleware := range c.requestMiddleware {
		if err := middleware(req); err != nil {
			return err
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("report request failed: %d (%s)", resp.StatusCode, string(data))
	}
	return nil
}

type reporterState struct {
	phase   string
	status  string
	message string
}

type Reporter struct {
	mu    sync.RWMutex
	state reporterState
	reqs  chan struct{}

	log      logr.Logger
	client   *Client
	interval time.Duration
}

func NewReporter(logger logr.Logger, c *Client) *Reporter {
	if logger.IsZero() {
		logger = logf.Log.WithName("agent.reporter")
	}
	r := &Reporter{
		log: logger,
		state: reporterState{
			phase:  PhaseInitial,
			status: StatusPending,
		},
		reqs:     make(chan struct{}, 1),
		client:   c,
		interval: 5 * time.Second,
	}
	return r
}

func (r *Reporter) report(ctx context.Context) error {
	r.mu.RLock()
	state := r.state
	r.mu.RUnlock()

	return r.client.Report(ctx, &Report{
		Phase:   state.phase,
		Status:  state.status,
		Message: state.message,
	})
}

type TransitionOption func(s *reporterState)

func WithStatus(status string) TransitionOption {
	return func(s *reporterState) {
		s.status = status
	}
}

func WithMessage(message string) TransitionOption {
	return func(s *reporterState) {
		s.message = message
	}
}

func (r *Reporter) Transition(phase string, opts ...TransitionOption) {
	newState := reporterState{
		phase: phase,
	}
	for _, opt := range opts {
		opt(&newState)
	}
	if newState.status == "" {
		newState.status = StatusPending
	}

	r.mu.Lock()
	r.state = newState
	r.mu.Unlock()

	select {
	case r.reqs <- struct{}{}:
	default:
	}
}

func (r *Reporter) Start(ctx context.Context) {
	if err := r.report(ctx); err != nil {
		r.log.Error(err, "Error doing initial report")
	}

	t := time.NewTicker(r.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := r.report(ctx); err != nil {
				r.log.Error(err, "Error doing report")
			}
		case <-r.reqs:
			if err := r.report(ctx); err != nil {
				r.log.Error(err, "Error doing report")
			}
			t.Reset(r.interval)
		}
	}
}
