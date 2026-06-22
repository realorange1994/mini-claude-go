package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// ─── Remote Metrics Pipeline (MiMo-Code 2) ─────────────────────────────────
//
// Structured event pipeline for model/tool/agent metrics.
//
// MiMo-Code source: metrics/client.ts (40 lines), event.ts (43 lines)

// MetricEventType represents the type of metric event.
type MetricEventType string

const (
	MetricModelCall    MetricEventType = "model_call"
	MetricToolCall     MetricEventType = "tool_call"
	MetricAgentRequest MetricEventType = "agent_request"
)

// MetricEvent represents a metric event.
type MetricEvent struct {
	Type      MetricEventType `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	SessionID string          `json:"session_id"`
	InstallID string          `json:"install_id"`
	Data      map[string]any  `json:"data"`
}

// MetricsConfig holds metrics configuration.
type MetricsConfig struct {
	Enabled   bool   `json:"enabled"`
	Endpoint  string `json:"endpoint"`
	InstallID string `json:"install_id"`
}

// MetricsCollector collects and sends metric events.
type MetricsCollector struct {
	mu       sync.Mutex
	config   MetricsConfig
	events   []MetricEvent
	client   *http.Client
	sessionID string
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector(config MetricsConfig, sessionID string) *MetricsCollector {
	return &MetricsCollector{
		config:    config,
		events:    make([]MetricEvent, 0),
		client:    &http.Client{Timeout: 10 * time.Second},
		sessionID: sessionID,
	}
}

// RecordModelCall records a model call metric.
func (c *MetricsCollector) RecordModelCall(data map[string]any) {
	if !c.config.Enabled {
		return
	}

	event := MetricEvent{
		Type:      MetricModelCall,
		Timestamp: time.Now(),
		SessionID: c.sessionID,
		InstallID: c.config.InstallID,
		Data:      data,
	}

	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

// RecordToolCall records a tool call metric.
func (c *MetricsCollector) RecordToolCall(data map[string]any) {
	if !c.config.Enabled {
		return
	}

	event := MetricEvent{
		Type:      MetricToolCall,
		Timestamp: time.Now(),
		SessionID: c.sessionID,
		InstallID: c.config.InstallID,
		Data:      data,
	}

	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

// RecordAgentRequest records an agent request metric.
func (c *MetricsCollector) RecordAgentRequest(data map[string]any) {
	if !c.config.Enabled {
		return
	}

	event := MetricEvent{
		Type:      MetricAgentRequest,
		Timestamp: time.Now(),
		SessionID: c.sessionID,
		InstallID: c.config.InstallID,
		Data:      data,
	}

	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

// Flush sends all collected events to the endpoint.
func (c *MetricsCollector) Flush() error {
	if !c.config.Enabled || c.config.Endpoint == "" {
		return nil
	}

	c.mu.Lock()
	events := c.events
	c.events = make([]MetricEvent, 0)
	c.mu.Unlock()

	if len(events) == 0 {
		return nil
	}

	data, err := json.Marshal(events)
	if err != nil {
		return err
	}

	resp, err := c.client.Post(c.config.Endpoint, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// Count returns the number of collected events.
func (c *MetricsCollector) Count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}
