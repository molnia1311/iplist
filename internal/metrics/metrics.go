// Package metrics provides a tiny Prometheus-compatible metrics registry.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// CounterVec is a counter with a fixed set of label names.
type CounterVec struct {
	mu     sync.Mutex
	name   string
	help   string
	labels []string
	values map[string]float64
}

// NewCounterVec creates a counter vector.
func NewCounterVec(opts Opts, labels []string) *CounterVec {
	return &CounterVec{
		name:   opts.Name,
		help:   opts.Help,
		labels: labels,
		values: make(map[string]float64),
	}
}

// WithLabelValues returns a counter for the given label values.
func (c *CounterVec) WithLabelValues(values ...string) *CounterVecChild {
	if len(values) != len(c.labels) {
		panic(fmt.Sprintf("label count mismatch for %s: got %d want %d", c.name, len(values), len(c.labels)))
	}
	key := strings.Join(values, "\x00")
	return &CounterVecChild{vec: c, key: key}
}

// CounterVecChild is a single counter instance.
type CounterVecChild struct {
	vec *CounterVec
	key string
}

// Inc increments the counter by one.
func (c *CounterVecChild) Inc() { c.Add(1) }

// Add increments the counter by n.
func (c *CounterVecChild) Add(n float64) {
	c.vec.mu.Lock()
	c.vec.values[c.key] += n
	c.vec.mu.Unlock()
}

// GaugeVec is a gauge with a fixed set of label names.
type GaugeVec struct {
	mu     sync.Mutex
	name   string
	help   string
	labels []string
	values map[string]float64
}

// NewGaugeVec creates a gauge vector.
func NewGaugeVec(opts Opts, labels []string) *GaugeVec {
	return &GaugeVec{
		name:   opts.Name,
		help:   opts.Help,
		labels: labels,
		values: make(map[string]float64),
	}
}

// WithLabelValues returns a gauge for the given label values.
func (g *GaugeVec) WithLabelValues(values ...string) *GaugeVecChild {
	if len(values) != len(g.labels) {
		panic(fmt.Sprintf("label count mismatch for %s: got %d want %d", g.name, len(values), len(g.labels)))
	}
	key := strings.Join(values, "\x00")
	return &GaugeVecChild{vec: g, key: key}
}

// GaugeVecChild is a single gauge instance.
type GaugeVecChild struct {
	vec *GaugeVec
	key string
}

// Set sets the gauge value.
func (g *GaugeVecChild) Set(v float64) {
	g.vec.mu.Lock()
	g.vec.values[g.key] = v
	g.vec.mu.Unlock()
}

// Opts describes a metric.
type Opts struct {
	Name string
	Help string
}

var (
	registryMu sync.Mutex
	registry   []metricVec
)

type metricVec interface {
	collect() (name, help, typ string, labels []string, values map[string]float64)
}

func (c *CounterVec) collect() (string, string, string, []string, map[string]float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	values := make(map[string]float64, len(c.values))
	for k, v := range c.values {
		values[k] = v
	}
	return c.name, c.help, "counter", c.labels, values
}

func (g *GaugeVec) collect() (string, string, string, []string, map[string]float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	values := make(map[string]float64, len(g.values))
	for k, v := range g.values {
		values[k] = v
	}
	return g.name, g.help, "gauge", g.labels, values
}

// Register adds a metric vector to the global registry.
func Register(m metricVec) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, m)
}

// PrefixesTotal tracks cached prefix counts.
var PrefixesTotal = NewGaugeVec(Opts{
	Name: "iplist_prefixes_total",
	Help: "Number of IPv4 prefixes cached for each allowlist.",
}, []string{"list"})

// FetchTotal tracks fetch outcomes.
var FetchTotal = NewCounterVec(Opts{
	Name: "iplist_fetch_total",
	Help: "Total number of fetch attempts per allowlist and outcome.",
}, []string{"list", "status"})

// LastSuccessTimestamp tracks the last successful fetch.
var LastSuccessTimestamp = NewGaugeVec(Opts{
	Name: "iplist_last_success_timestamp",
	Help: "Unix timestamp of the last successful fetch for each allowlist.",
}, []string{"list"})

// StalenessSeconds tracks how stale each allowlist is.
var StalenessSeconds = NewGaugeVec(Opts{
	Name: "iplist_staleness_seconds",
	Help: "Seconds since the last successful fetch for each allowlist.",
}, []string{"list"})

func init() {
	Register(PrefixesTotal)
	Register(FetchTotal)
	Register(LastSuccessTimestamp)
	Register(StalenessSeconds)
}

// Handler serves the metrics in Prometheus exposition format.
func Handler(w http.ResponseWriter, r *http.Request) {
	registryMu.Lock()
	metrics := make([]metricVec, len(registry))
	copy(metrics, registry)
	registryMu.Unlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	for _, m := range metrics {
		name, help, typ, labels, values := m.collect()
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)

		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, key := range keys {
			labelValues := strings.Split(key, "\x00")
			pairs := make([]string, len(labels))
			for i, ln := range labels {
				pairs[i] = fmt.Sprintf("%s=\"%s\"", ln, labelValues[i])
			}
			fmt.Fprintf(w, "%s{%s} %g\n", name, strings.Join(pairs, ","), values[key])
		}
	}
}
