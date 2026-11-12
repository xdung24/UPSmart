package state

import (
	"encoding/json"
	"fmt"
	"time"

	"upsmart/internal/hardware"
)

type HardwareStatus struct {
	Mode        string          `json:"mode"`
	GeneratedAt time.Time       `json:"generated_at"`
	SampledAt   *time.Time      `json:"sampled_at"`
	Stale       bool            `json:"stale"`
	Error       string          `json:"error,omitempty"`
	Data        json.RawMessage `json:"data"`
}

func (s *Store) EnableHardware(interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hardware.Mode = "waiting"
	s.hardwareStaleAfter = 2*interval + 10*time.Second
}

func (s *Store) UpdateHardware(now time.Time, sample hardware.Snapshot, collectErr error) {
	data, err := json.Marshal(sample)
	if err != nil {
		collectErr = fmt.Errorf("encode hardware snapshot: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.hardware.Data = data
		timestamp := now.UTC()
		s.hardware.SampledAt = &timestamp
		s.hardwareLastSample = now
	}
	s.hardware.Mode = "enabled"
	s.hardware.Error = ""
	if collectErr != nil {
		s.hardware.Mode = "degraded"
		s.hardware.Error = collectErr.Error()
	}
}

func (s *Store) HardwareSnapshot(now time.Time) HardwareStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.hardware
	if out.Mode == "" {
		out.Mode = "disabled"
	}
	out.GeneratedAt = now.UTC()
	out.SampledAt = copyPointer(out.SampledAt)
	out.Data = append(json.RawMessage(nil), out.Data...)
	out.Stale = s.hardwareLastSample.IsZero() || now.Sub(s.hardwareLastSample) >= s.hardwareStaleAfter
	return out
}
