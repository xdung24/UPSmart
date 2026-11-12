package hardware

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxProviderOutput = 1024 * 1024

type providerData struct {
	GPUs []struct {
		ID, Name       string
		DedicatedTotal *float64
	}
	FallbackGPUs []struct{ Name string }
	Engines      []struct {
		Name                  string
		UtilizationPercentage *float64
	}
	GPUMemory []struct {
		Name                        string
		DedicatedUsage, SharedUsage *float64
	}
	Volumes []struct {
		DeviceID        string
		Size, FreeSpace *float64
	}
	Physical []struct {
		Index uint32
		Model string
		Size  *float64
	}
	Activity []struct {
		Name                                      string
		PercentIdleTime                           *float64
		DiskReadBytesPersec, DiskWriteBytesPersec *float64
	}
	Errors      map[string]string
	Performance []struct {
		Kind, Name string
		Value      *float64
	}
}

func number(v *float64, unit, source string) Measurement {
	if v == nil {
		return missing(unit, source, "Provider did not return this value")
	}
	return measured(*v, unit, source)
}

var gpuInstance = regexp.MustCompile(`(?i)luid_(0x[0-9a-f]+)_(0x[0-9a-f]+)_phys_([0-9]+)(?:_eng_([0-9]+))?`)

func gpuKey(name string) (string, string, bool) {
	match := gpuInstance.FindStringSubmatch(name)
	if match == nil {
		return "", "", false
	}
	high, e1 := strconv.ParseUint(match[1][2:], 16, 32)
	low, e2 := strconv.ParseUint(match[2][2:], 16, 32)
	phys, e3 := strconv.ParseUint(match[3], 10, 32)
	if e1 != nil || e2 != nil || e3 != nil {
		return "", "", false
	}
	return fmt.Sprintf("0x%08x_0x%08x/phys_%d", high, low, phys), match[4], true
}

func decodeProviders(data []byte, snapshot *Snapshot) error {
	if len(data) > maxProviderOutput {
		return errors.New("Windows providers: output exceeds limit")
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("Windows providers: invalid JSON: %w", err)
	}
	raw := make(map[string]json.RawMessage, len(envelope))
	for key, value := range envelope {
		raw[strings.ToLower(key)] = value
	}
	var p providerData
	if err := json.Unmarshal(raw["errors"], &p.Errors); err != nil || p.Errors == nil {
		return errors.New("Windows providers: missing provider status")
	}
	// Malformed optional groups do not discard another provider's good data.
	decodeGroup(raw["gpus"], &p.GPUs, 64, p.Errors, "gpu_inventory")
	decodeGroup(raw["fallbackgpus"], &p.FallbackGPUs, 64, p.Errors, "gpu_fallback_inventory")
	decodeGroup(raw["engines"], &p.Engines, 2048, p.Errors, "gpu_engines")
	decodeGroup(raw["gpumemory"], &p.GPUMemory, 128, p.Errors, "gpu_memory")
	decodeGroup(raw["volumes"], &p.Volumes, 128, p.Errors, "volumes")
	decodeGroup(raw["physical"], &p.Physical, 128, p.Errors, "physical")
	decodeGroup(raw["activity"], &p.Activity, 128, p.Errors, "disk_activity")
	decodeGroup(raw["performance"], &p.Performance, 2688, p.Errors, "performance")
	applyPerformance(&p)
	applyGPUs(p, snapshot)
	applyDisks(p, snapshot)
	keys := make([]string, 0, len(p.Errors))
	for key := range p.Errors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var errs []error
	for _, key := range keys {
		errs = append(errs, fmt.Errorf("%s: %s", key, p.Errors[key]))
	}
	return errors.Join(errs...)
}

func applyPerformance(p *providerData) {
	activity := map[string]int{}
	memory := map[string]int{}
	for _, raw := range p.Performance {
		switch raw.Kind {
		case "gpu_engine":
			p.Engines = append(p.Engines, struct {
				Name                  string
				UtilizationPercentage *float64
			}{raw.Name, raw.Value})
		case "gpu_dedicated", "gpu_shared":
			index, ok := memory[raw.Name]
			if !ok {
				index = len(p.GPUMemory)
				memory[raw.Name] = index
				p.GPUMemory = append(p.GPUMemory, struct {
					Name                        string
					DedicatedUsage, SharedUsage *float64
				}{Name: raw.Name})
			}
			if raw.Kind == "gpu_dedicated" {
				p.GPUMemory[index].DedicatedUsage = raw.Value
			} else {
				p.GPUMemory[index].SharedUsage = raw.Value
			}
		case "disk_idle", "disk_read", "disk_write":
			index, ok := activity[raw.Name]
			if !ok {
				index = len(p.Activity)
				activity[raw.Name] = index
				p.Activity = append(p.Activity, struct {
					Name                                      string
					PercentIdleTime                           *float64
					DiskReadBytesPersec, DiskWriteBytesPersec *float64
				}{Name: raw.Name})
			}
			switch raw.Kind {
			case "disk_idle":
				p.Activity[index].PercentIdleTime = raw.Value
			case "disk_read":
				p.Activity[index].DiskReadBytesPersec = raw.Value
			case "disk_write":
				p.Activity[index].DiskWriteBytesPersec = raw.Value
			}
		}
	}
}

func decodeGroup[T any](raw json.RawMessage, target *[]T, limit int, failures map[string]string, key string) {
	if len(raw) == 0 {
		return
	}
	if err := json.Unmarshal(raw, target); err != nil {
		*target = nil
		failures[key] = "Invalid provider response: " + err.Error()
	} else if len(*target) > limit {
		*target = nil
		failures[key] = "Provider exceeded bounded record limit"
	}
}

func newGPU(id, name string) GPU {
	return GPU{ID: id, Name: name,
		Utilization:    missing("%", "WDDM GPU engine counters", "No matching supported GPU engine counters"),
		DedicatedUsed:  missing("bytes", "WDDM GPU adapter memory counters", "No matching supported memory counters"),
		DedicatedTotal: missing("bytes", "DXGI", "DXGI inventory unavailable; Win32_VideoController.AdapterRAM is not reliable"),
		SharedUsed:     missing("bytes", "WDDM GPU adapter memory counters", "No matching supported memory counters")}
}

func applyGPUs(p providerData, s *Snapshot) {
	indices := map[string]int{}
	for _, raw := range p.GPUs {
		key := strings.ToLower(raw.ID) + "/phys_0"
		gpu := newGPU(key, raw.Name)
		gpu.DedicatedTotal = number(raw.DedicatedTotal, "bytes", "DXGI")
		indices[key] = len(s.GPUs)
		s.GPUs = append(s.GPUs, gpu)
	}
	if len(p.GPUs) == 0 {
		for i, raw := range p.FallbackGPUs {
			gpu := newGPU(fmt.Sprintf("inventory:%d", i), raw.Name)
			gpu.InventoryReason = "CIM inventory fallback; cannot associate WDDM LUID counters reliably"
			s.GPUs = append(s.GPUs, gpu)
		}
	}
	ensure := func(key string) int {
		if index, ok := indices[key]; ok {
			return index
		}
		gpu := newGPU(key, "WDDM adapter "+key)
		gpu.InventoryReason = "Counter adapter has no matching DXGI inventory; no guessed device association"
		indices[key] = len(s.GPUs)
		s.GPUs = append(s.GPUs, gpu)
		return len(s.GPUs) - 1
	}
	// Per-process engine usage is additive for the same engine, not across
	// independent engines. Report the busiest engine (Task Manager convention).
	engineTotals := map[string]map[string]float64{}
	invalid := map[string]bool{}
	for _, raw := range p.Engines {
		key, engine, ok := gpuKey(raw.Name)
		if !ok || engine == "" {
			continue
		}
		ensure(key)
		value := number(raw.UtilizationPercentage, "%", "WDDM")
		if value.Value == nil || *value.Value > 100 {
			invalid[key] = true
			continue
		}
		if engineTotals[key] == nil {
			engineTotals[key] = map[string]float64{}
		}
		engineTotals[key][engine] += *value.Value
	}
	for key, totals := range engineTotals {
		busiest := 0.0
		for _, value := range totals {
			if value > busiest {
				busiest = value
			}
		}
		index := indices[key]
		if invalid[key] || busiest > 100 {
			s.GPUs[index].Utilization.Reason = "GPU engine counters invalid or inconsistent; not clamped"
		} else {
			s.GPUs[index].Utilization = measured(busiest, "%", "WDDM busiest engine (sum of processes per engine)")
		}
	}
	for _, raw := range p.GPUMemory {
		key, _, ok := gpuKey(raw.Name)
		if !ok {
			continue
		}
		index := ensure(key)
		s.GPUs[index].DedicatedUsed = number(raw.DedicatedUsage, "bytes", "WDDM GPU adapter memory counters")
		s.GPUs[index].SharedUsed = number(raw.SharedUsage, "bytes", "WDDM GPU adapter memory counters")
	}
	if len(s.GPUs) > 0 {
		s.GPUsReason = ""
	} else {
		s.GPUsReason = reason(p, "gpu_inventory", "No GPU inventory or WDDM counters returned")
	}
}

func reason(p providerData, key, fallback string) string {
	if err := p.Errors[key]; err != "" {
		return err
	}
	return fallback
}

func diskCounterReason(p providerData, counter, fallback string) string {
	return reason(p, "disk_activity."+counter, reason(p, "disk_activity", fallback))
}

func applyDisks(p providerData, s *Snapshot) {
	for _, raw := range p.Volumes {
		v := Volume{ID: raw.DeviceID, Total: number(raw.Size, "bytes", "Win32_LogicalDisk"),
			Free:          number(raw.FreeSpace, "bytes", "Win32_LogicalDisk"),
			Used:          missing("bytes", "Win32_LogicalDisk", "Capacity values missing or inconsistent"),
			MappingReason: "Volume-to-physical mapping not queried; volumes can span multiple devices"}
		if v.Total.Value != nil && v.Free.Value != nil && *v.Free.Value <= *v.Total.Value {
			v.Used = measured(*v.Total.Value-*v.Free.Value, "bytes", "Win32_LogicalDisk")
		} else if v.Total.Value != nil && v.Free.Value != nil {
			v.Free = missing("bytes", "Win32_LogicalDisk", "Free capacity exceeds total")
		}
		s.Disks.Volumes = append(s.Disks.Volumes, v)
	}
	activity := map[string]int{}
	for _, raw := range p.Physical {
		id := strconv.FormatUint(uint64(raw.Index), 10)
		activity[id] = len(s.Disks.Physical)
		s.Disks.Physical = append(s.Disks.Physical, Disk{
			ID: id, Name: raw.Model, Total: number(raw.Size, "bytes", "Win32_DiskDrive"),
			Utilization: missing("%", "PhysicalDisk performance counters", diskCounterReason(p, "disk_idle", "No matching physical disk counter")),
			ReadRate:    missing("bytes/s", "PhysicalDisk performance counters", diskCounterReason(p, "disk_read", "No matching physical disk counter")),
			WriteRate:   missing("bytes/s", "PhysicalDisk performance counters", diskCounterReason(p, "disk_write", "No matching physical disk counter")),
		})
	}
	for _, raw := range p.Activity {
		fields := strings.Fields(raw.Name)
		if len(fields) == 0 {
			continue
		}
		index, ok := activity[fields[0]]
		if !ok {
			continue
		}
		disk := &s.Disks.Physical[index]
		idle := number(raw.PercentIdleTime, "%", "PhysicalDisk performance counters")
		if idle.Value != nil && *idle.Value <= 100 {
			disk.Utilization = measured(100-*idle.Value, "%", "PhysicalDisk 100 - PercentIdleTime")
		} else {
			disk.Utilization.Reason = diskCounterReason(p, "disk_idle", "Missing or invalid physical disk idle counter")
		}
		disk.ReadRate = number(raw.DiskReadBytesPersec, "bytes/s", "PhysicalDisk performance counters")
		disk.WriteRate = number(raw.DiskWriteBytesPersec, "bytes/s", "PhysicalDisk performance counters")
		if disk.ReadRate.Value == nil {
			disk.ReadRate.Reason = diskCounterReason(p, "disk_read", disk.ReadRate.Reason)
		}
		if disk.WriteRate.Value == nil {
			disk.WriteRate.Reason = diskCounterReason(p, "disk_write", disk.WriteRate.Reason)
		}
	}
	if len(s.Disks.Volumes) > 0 {
		s.Disks.VolumesReason = ""
	} else {
		s.Disks.VolumesReason = reason(p, "volumes", "No fixed logical volumes returned")
	}
	if len(s.Disks.Physical) > 0 {
		s.Disks.PhysicalReason = ""
	} else {
		s.Disks.PhysicalReason = reason(p, "physical", "No physical disk inventory returned")
	}
}
