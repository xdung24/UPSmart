// Package hardware provides optional, read-only PC telemetry. Nothing starts
// polling until a caller invokes Collect or Run; no UPS or application state is used.
package hardware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

const collectionTimeout = 8 * time.Second

// Measurement is unavailable when Value is nil; Reason then explains why.
// Source identifies the provider, not a claim that hardware has been validated.
type Measurement struct {
	Value  *float64 `json:"value"`
	Unit   string   `json:"unit"`
	Source string   `json:"source"`
	Reason string   `json:"reason,omitempty"`
}

type CPU struct {
	Utilization Measurement `json:"utilization"`
}

// GPU IDs are DXGI adapter LUIDs (session-local), optionally with a physical
// index. Counter-only adapters are retained without guessing an inventory match.
type GPU struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	InventoryReason string      `json:"inventory_reason,omitempty"`
	Utilization     Measurement `json:"utilization"`
	DedicatedUsed   Measurement `json:"dedicated_used"`
	DedicatedTotal  Measurement `json:"dedicated_total"`
	SharedUsed      Measurement `json:"shared_used"`
}

type Volume struct {
	ID    string      `json:"id"`
	Total Measurement `json:"total"`
	Free  Measurement `json:"free"`
	Used  Measurement `json:"used"`
	// Physical associations are not inferred from drive letters or names.
	MappingReason string `json:"mapping_reason"`
}

type Disk struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Total       Measurement `json:"total"`
	Utilization Measurement `json:"utilization"`
	ReadRate    Measurement `json:"read_rate"`
	WriteRate   Measurement `json:"write_rate"`
}

type Disks struct {
	Volumes        []Volume `json:"volumes"`
	Physical       []Disk   `json:"physical"`
	VolumesReason  string   `json:"volumes_reason,omitempty"`
	PhysicalReason string   `json:"physical_reason,omitempty"`
}

// Snapshot contains only this attempt's readings; failed sources never reuse
// old values. Availability is available, partial, or unavailable. Stale means
// collection was interrupted; consumers must also age SampledAt in their cache.
type Snapshot struct {
	SampledAt    time.Time `json:"sampled_at"`
	Stale        bool      `json:"stale"`
	Availability string    `json:"availability"`
	CPU          CPU       `json:"cpu"`
	GPUs         []GPU     `json:"gpus"`
	GPUsReason   string    `json:"gpus_reason,omitempty"`
	Disks        Disks     `json:"disks"`
	Warnings     []string  `json:"warnings"`
}

type cpuTimes struct{ idle, kernel, user uint64 }
type sources struct {
	cpu   func(context.Context) (cpuTimes, error)
	query func(context.Context) ([]byte, error)
}

// Collector owns a CPU delta baseline. New collectors report utilization as
// unavailable on their first successful CPU sample. Concurrent calls serialize.
type Collector struct {
	gate     chan struct{}
	sources  sources
	previous *cpuTimes
}

func New() *Collector { return newCollector(platformSources()) }

func newCollector(s sources) *Collector {
	return &Collector{gate: make(chan struct{}, 1), sources: s}
}

var defaultOnce sync.Once
var defaultCollector *Collector

// Collect uses a shared, serialized CPU baseline and an eight-second deadline,
// including time waiting for another collection. For a one-shot caller, it primes
// a missing CPU baseline using only the native CPU source, then waits 250ms
// before normal collection. Partial data accompanies errors.
func Collect(ctx context.Context) (Snapshot, error) {
	defaultOnce.Do(func() { defaultCollector = New() })
	return defaultCollector.collect(ctx, true)
}

func (c *Collector) Collect(ctx context.Context) (Snapshot, error) {
	return c.collect(ctx, false)
}

func (c *Collector) collect(ctx context.Context, warmCPU bool) (Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, collectionTimeout)
	defer cancel()
	s := emptySnapshot()
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return interrupted(s, ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		return interrupted(s, err)
	}
	if warmCPU && c.previous == nil {
		if times, err := c.sources.cpu(ctx); err == nil {
			c.previous = &times
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				c.previous = nil
				return interrupted(s, ctx.Err())
			case <-timer.C:
			}
		}
	}
	// Timestamp actual collection, not queueing or native-only warm-up.
	s.SampledAt = time.Now().UTC()
	var failures []error
	times, err := c.sources.cpu(ctx)
	if err != nil {
		s.CPU.Utilization.Reason = err.Error()
		failures = append(failures, fmt.Errorf("CPU: %w", err))
		c.previous = nil
	} else {
		s.CPU.Utilization = cpuUsage(c.previous, times)
		c.previous = &times
	}
	data, queryErr := c.sources.query(ctx)
	if queryErr != nil {
		failures = append(failures, fmt.Errorf("Windows providers: %w", queryErr))
		s.GPUsReason = queryErr.Error()
		s.Disks.VolumesReason = queryErr.Error()
		s.Disks.PhysicalReason = queryErr.Error()
	} else if err := decodeProviders(data, &s); err != nil {
		failures = append(failures, err)
	}
	if err := ctx.Err(); err != nil {
		s.Stale = true
		failures = append(failures, err)
	}
	for _, err := range failures {
		s.Warnings = append(s.Warnings, err.Error())
	}
	finishSnapshot(&s)
	return s, errors.Join(failures...)
}

func interrupted(s Snapshot, err error) (Snapshot, error) {
	s.Stale = true
	s.Warnings = append(s.Warnings, err.Error())
	return s, err
}

func emptySnapshot() Snapshot {
	return Snapshot{
		SampledAt: time.Now().UTC(), Availability: "unavailable",
		CPU:  CPU{missing("%", "GetSystemTimes", "CPU baseline not yet sampled")},
		GPUs: []GPU{}, GPUsReason: "GPU providers not available",
		Disks: Disks{Volumes: []Volume{}, Physical: []Disk{},
			VolumesReason: "Volume provider not available", PhysicalReason: "Physical disk provider not available"},
		Warnings: []string{},
	}
}

func missing(unit, source, reason string) Measurement {
	return Measurement{Unit: unit, Source: source, Reason: reason}
}

func measured(v float64, unit, source string) Measurement {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return missing(unit, source, "Provider returned an invalid value")
	}
	return Measurement{Value: &v, Unit: unit, Source: source}
}

func cpuUsage(previous *cpuTimes, current cpuTimes) Measurement {
	const source = "GetSystemTimes"
	if previous == nil {
		return missing("%", source, "First CPU sample: waiting for delta baseline")
	}
	if current.idle < previous.idle || current.kernel < previous.kernel || current.user < previous.user {
		return missing("%", source, "CPU counters reset")
	}
	idle := current.idle - previous.idle
	kernel := current.kernel - previous.kernel
	user := current.user - previous.user
	// Windows kernel time includes idle time.
	total := float64(kernel) + float64(user)
	if total == 0 || float64(idle) > total {
		return missing("%", source, "CPU sampling interval or counters invalid")
	}
	return measured(100*(total-float64(idle))/total, "%", source)
}

func finishSnapshot(s *Snapshot) {
	available, absent := 0, 0
	add := func(m Measurement) {
		if m.Value != nil {
			available++
		} else {
			absent++
		}
	}
	add(s.CPU.Utilization)
	for _, gpu := range s.GPUs {
		add(gpu.Utilization)
		add(gpu.DedicatedUsed)
		add(gpu.DedicatedTotal)
	}
	for _, disk := range s.Disks.Physical {
		add(disk.Total)
		add(disk.Utilization)
		add(disk.ReadRate)
		add(disk.WriteRate)
	}
	for _, volume := range s.Disks.Volumes {
		add(volume.Total)
		add(volume.Free)
	}
	if available > 0 {
		s.Availability = "available"
		if absent > 0 || len(s.Warnings) > 0 || s.GPUsReason != "" || s.Disks.VolumesReason != "" || s.Disks.PhysicalReason != "" {
			s.Availability = "partial"
		}
	}
}

// Run polls immediately and then waits interval after each attempt. A separate
// collector avoids disturbing Collect's baseline. Calls and publish are serial;
// publish must return promptly and must not block waiting for Run to finish.
// Nonpositive intervals use five seconds; positive intervals below one second
// use one second. Every collection has an eight-second timeout.
func Run(ctx context.Context, interval time.Duration, publish func(Snapshot, error)) {
	run(ctx, interval, publish, New())
}

func run(ctx context.Context, interval time.Duration, publish func(Snapshot, error), c *Collector) {
	if publish == nil {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	} else if interval < time.Second {
		interval = time.Second
	}
	for ctx.Err() == nil {
		snapshot, err := c.Collect(ctx)
		if ctx.Err() != nil {
			return
		}
		publish(snapshot, err)
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
