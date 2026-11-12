package hardware

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCPUDeltas(t *testing.T) {
	first := cpuTimes{idle: 200, kernel: 400, user: 100}
	if got := cpuUsage(nil, first); got.Value != nil || !strings.Contains(got.Reason, "First") {
		t.Fatalf("first sample: %+v", got)
	}
	got := cpuUsage(&first, cpuTimes{idle: 250, kernel: 500, user: 200})
	if got.Value == nil || *got.Value != 75 {
		t.Fatalf("kernel includes idle, expected 75%%: %+v", got)
	}
	for _, sample := range []cpuTimes{first, {idle: 100, kernel: 500, user: 200}, {idle: 900, kernel: 500, user: 200}} {
		got := cpuUsage(&first, sample)
		if got.Value != nil || got.Reason == "" {
			t.Fatalf("invalid interval/counters: %+v", got)
		}
	}
}

func TestOneShotWarmsOnlyCPUAndCollectsProvidersOnce(t *testing.T) {
	cpuCalls, queryCalls := 0, 0
	c := newCollector(sources{
		cpu: func(ctx context.Context) (cpuTimes, error) {
			cpuCalls++
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > collectionTimeout {
				t.Fatal("warm-up must share the collection deadline")
			}
			return cpuTimes{uint64(cpuCalls), uint64(cpuCalls * 2), uint64(cpuCalls * 3)}, nil
		},
		query: func(context.Context) ([]byte, error) {
			queryCalls++
			return []byte(`{"Errors":{}}`), nil
		},
	})
	start := time.Now()
	s, err := c.collect(context.Background(), true)
	if err != nil || cpuCalls != 2 || queryCalls != 1 || s.CPU.Utilization.Value == nil ||
		*s.CPU.Utilization.Value != 80 || time.Since(start) < 250*time.Millisecond {
		t.Fatalf("one-shot warm-up: cpu=%d query=%d snapshot=%+v err=%v", cpuCalls, queryCalls, s, err)
	}
	_, _ = c.collect(context.Background(), true)
	if cpuCalls != 3 || queryCalls != 2 {
		t.Fatal("existing baseline must not trigger another warm-up")
	}
}

func TestOneShotWarmupCancellationDoesNotQueryProviders(t *testing.T) {
	cpuCalls, queryCalls := 0, 0
	c := newCollector(sources{
		cpu: func(context.Context) (cpuTimes, error) {
			cpuCalls++
			return cpuTimes{1, 2, 3}, nil
		},
		query: func(context.Context) ([]byte, error) {
			queryCalls++
			return nil, errors.New("unexpected query")
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	s, err := c.collect(ctx, true)
	if !errors.Is(err, context.DeadlineExceeded) || !s.Stale || cpuCalls != 1 || queryCalls != 0 ||
		c.previous != nil || time.Since(start) > time.Second {
		t.Fatalf("warm-up cancellation leaked work: cpu=%d query=%d snapshot=%+v err=%v", cpuCalls, queryCalls, s, err)
	}
}

const providerFixture = `{
  "GPUs":[{"ID":"0x00000000_0x000012ab","Name":"Adapter A","DedicatedTotal":8589934592},
          {"ID":"0x00000000_0x000012ac","Name":"Adapter B","DedicatedTotal":0}],
  "Engines":[
    {"Name":"pid_1_luid_0x00000000_0x000012AB_phys_0_eng_0_engtype_3D","UtilizationPercentage":20},
    {"Name":"pid_2_luid_0x00000000_0x000012ab_phys_0_eng_0_engtype_3D","UtilizationPercentage":30},
    {"Name":"pid_2_luid_0x00000000_0x000012ab_phys_0_eng_1_engtype_Copy","UtilizationPercentage":40},
    {"Name":"pid_7_luid_0x00000000_0x00009999_phys_0_eng_0_engtype_3D","UtilizationPercentage":15}],
  "GPUMemory":[{"Name":"luid_0x00000000_0x000012ab_phys_0","DedicatedUsage":1234567890,"SharedUsage":100}],
  "Volumes":[{"DeviceID":"C:","Size":1000,"FreeSpace":300},{"DeviceID":"D:","Size":100,"FreeSpace":120}],
  "Physical":[{"Index":0,"Model":"SSD","Size":2000},{"Index":1,"Model":"HDD","Size":4000}],
  "Activity":[{"Name":"0 C:","PercentIdleTime":70,"DiskReadBytesPersec":1024,"DiskWriteBytesPersec":2048},
              {"Name":"_Total","PercentIdleTime":0,"DiskReadBytesPersec":999}],
  "Errors":{}
}`

func TestProviderParsingAndAssociation(t *testing.T) {
	s := emptySnapshot()
	if err := decodeProviders([]byte(providerFixture), &s); err != nil {
		t.Fatal(err)
	}
	if len(s.GPUs) != 3 || s.GPUs[0].Name != "Adapter A" || *s.GPUs[0].Utilization.Value != 50 {
		t.Fatalf("GPU aggregation/association: %+v", s.GPUs)
	}
	if *s.GPUs[0].DedicatedTotal.Value != 8589934592 || *s.GPUs[0].DedicatedUsed.Value != 1234567890 {
		t.Fatalf("GPU memory must not truncate to uint32: %+v", s.GPUs[0])
	}
	if s.GPUs[1].Utilization.Value != nil || s.GPUs[1].Utilization.Reason == "" ||
		s.GPUs[2].InventoryReason == "" || *s.GPUs[2].Utilization.Value != 15 {
		t.Fatal("missing counters and unmatched adapters must remain explicit")
	}
	if len(s.Disks.Volumes) != 2 || *s.Disks.Volumes[0].Used.Value != 700 ||
		s.Disks.Volumes[1].Used.Value != nil || s.Disks.Volumes[1].Free.Value != nil {
		t.Fatalf("volume calculations: %+v", s.Disks.Volumes)
	}
	disk := s.Disks.Physical[0]
	if *disk.Utilization.Value != 30 || *disk.ReadRate.Value != 1024 || *disk.WriteRate.Value != 2048 {
		t.Fatalf("physical rates: %+v", disk)
	}
	if s.Disks.Physical[1].ReadRate.Value != nil || s.Disks.Physical[1].ReadRate.Reason == "" {
		t.Fatal("missing activity must not become zero")
	}
}

func TestMissingSourcesDoNotLoseValidData(t *testing.T) {
	c := newCollector(sources{
		cpu: func(context.Context) (cpuTimes, error) { return cpuTimes{1, 2, 3}, nil },
		query: func(context.Context) ([]byte, error) {
			return []byte(`{"Volumes":[{"DeviceID":"C:","Size":200,"FreeSpace":100}],"Errors":{"gpu_inventory":"access denied"}}`), nil
		},
	})
	s, err := c.Collect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "access denied") || len(s.Disks.Volumes) != 1 {
		t.Fatalf("partial results: %+v, %v", s, err)
	}
	if s.Availability != "partial" || s.CPU.Utilization.Value != nil || len(s.Warnings) != 1 {
		t.Fatalf("quality: %+v", s)
	}
	encoded, err := json.Marshal(s)
	if err != nil || !strings.Contains(string(encoded), `"value":null`) {
		t.Fatalf("JSON null values required: %s, %v", encoded, err)
	}
}

func TestMalformedProviderGroupRetainsOtherSources(t *testing.T) {
	s := emptySnapshot()
	err := decodeProviders([]byte(`{
		"Errors":{},
		"GPUs":[{"ID":"adapter","DedicatedTotal":"malformed"}],
		"Volumes":[{"DeviceID":"C:","Size":200,"FreeSpace":100}]
	}`), &s)
	if err == nil || !strings.Contains(err.Error(), "gpu_inventory") ||
		len(s.Disks.Volumes) != 1 || *s.Disks.Volumes[0].Used.Value != 100 {
		t.Fatalf("malformed GPU group discarded valid volumes: %+v, %v", s, err)
	}
}

func TestCompleteHardwareAvailabilityWithoutTemperatureMonitoring(t *testing.T) {
	s := emptySnapshot()
	s.CPU.Utilization = measured(10, "%", "GetSystemTimes")
	err := decodeProviders([]byte(`{
		"Errors":{},
		"GPUs":[{"ID":"0x00000000_0x00000001","Name":"GPU","DedicatedTotal":1000}],
		"Engines":[{"Name":"pid_1_luid_0x0_0x1_phys_0_eng_0_engtype_3D","UtilizationPercentage":20}],
		"GPUMemory":[{"Name":"luid_0x0_0x1_phys_0","DedicatedUsage":250,"SharedUsage":100}],
		"Volumes":[{"DeviceID":"C:","Size":1000,"FreeSpace":300}],
		"Physical":[{"Index":0,"Model":"SSD","Size":1000}],
		"Activity":[{"Name":"0 C:","PercentIdleTime":70,"DiskReadBytesPersec":1024,"DiskWriteBytesPersec":2048}]
	}`), &s)
	if err != nil {
		t.Fatal(err)
	}
	finishSnapshot(&s)
	if s.Availability != "available" || len(s.Warnings) != 0 {
		t.Fatalf("removed temperature sources still degrade valid metrics: %+v", s)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["temperatures"]; ok {
		t.Fatal("hardware API still exposes temperature monitoring")
	}
}

func TestProviderRecordLimitRetainsOtherGroups(t *testing.T) {
	input := `{"Errors":{},"Volumes":[` + strings.Repeat(`{"DeviceID":"C:"},`, 128) +
		`{"DeviceID":"D:"}],"Physical":[{"Index":0,"Model":"SSD","Size":1000}]}`
	s := emptySnapshot()
	err := decodeProviders([]byte(input), &s)
	if err == nil || !strings.Contains(err.Error(), "record limit") ||
		len(s.Disks.Volumes) != 0 || len(s.Disks.Physical) != 1 {
		t.Fatalf("record bound discarded unrelated source: %+v, %v", s, err)
	}
}

func TestPDHSamplesAndPartialCounterFailure(t *testing.T) {
	s := emptySnapshot()
	err := decodeProviders([]byte(`{
		"Errors":{"gpu_memory":"PDH counter unavailable"},
		"GPUs":[{"ID":"0x00000000_0x00000001","Name":"GPU","DedicatedTotal":1000}],
		"Physical":[{"Index":0,"Model":"SSD","Size":1000}],
		"Performance":[
			{"Kind":"disk_idle","Name":"0 C:","Value":20},
			{"Kind":"disk_read","Name":"0 C:","Value":1024},
			{"Kind":"disk_write","Name":"0 C:","Value":2048},
			{"Kind":"gpu_engine","Name":"pid_1_luid_0x0_0x1_phys_0_eng_0_engtype_3D","Value":30},
			{"Kind":"gpu_engine","Name":"pid_2_luid_0x0_0x1_phys_0_eng_0_engtype_3D","Value":20},
			{"Kind":"gpu_dedicated","Name":"luid_0x0_0x1_phys_0","Value":500}
		]
	}`), &s)
	if err == nil || !strings.Contains(err.Error(), "PDH counter unavailable") {
		t.Fatal("partial PDH error must remain visible")
	}
	if *s.Disks.Physical[0].Utilization.Value != 80 ||
		*s.Disks.Physical[0].ReadRate.Value != 1024 ||
		*s.Disks.Physical[0].WriteRate.Value != 2048 {
		t.Fatalf("PDH disk values: %+v", s.Disks.Physical)
	}
	gpu := s.GPUs[0]
	if *gpu.Utilization.Value != 50 || *gpu.DedicatedUsed.Value != 500 ||
		gpu.SharedUsed.Value != nil || gpu.SharedUsed.Reason == "" {
		t.Fatalf("PDH GPU values: %+v", gpu)
	}
}

func TestDiskWriteCounterFailurePreservesReadAndUtilization(t *testing.T) {
	s := emptySnapshot()
	err := decodeProviders([]byte(`{
		"Errors":{"disk_activity.disk_write":"PDH_CSTATUS_NO_OBJECT (0xc0000bb8)"},
		"Physical":[{"Index":0,"Model":"SSD","Size":1000}],
		"Performance":[
			{"Kind":"disk_idle","Name":"0 C:","Value":20},
			{"Kind":"disk_read","Name":"0 C:","Value":1024}
		]
	}`), &s)
	if err == nil || !strings.Contains(err.Error(), "disk_activity.disk_write") {
		t.Fatal("write-counter failure must remain visible")
	}
	disk := s.Disks.Physical[0]
	if disk.Utilization.Value == nil || *disk.Utilization.Value != 80 ||
		disk.ReadRate.Value == nil || *disk.ReadRate.Value != 1024 ||
		disk.WriteRate.Value != nil || !strings.Contains(disk.WriteRate.Reason, "PDH_CSTATUS_NO_OBJECT") {
		t.Fatalf("one counter failure discarded or fabricated another counter: %+v", disk)
	}
}

func TestAllDiskCounterFailuresRetainIndividualReasons(t *testing.T) {
	s := emptySnapshot()
	err := decodeProviders([]byte(`{
		"Errors":{
			"disk_activity.disk_idle":"idle object unavailable",
			"disk_activity.disk_read":"read object unavailable",
			"disk_activity.disk_write":"write object unavailable"
		},
		"Physical":[{"Index":0,"Model":"SSD","Size":1000}]
	}`), &s)
	disk := s.Disks.Physical[0]
	if err == nil || disk.Utilization.Reason != "idle object unavailable" ||
		disk.ReadRate.Reason != "read object unavailable" || disk.WriteRate.Reason != "write object unavailable" ||
		disk.Total.Value == nil {
		t.Fatalf("individual counter errors or valid capacity lost: %+v, %v", disk, err)
	}
}

func TestCollectorTimeoutRetainsCPUAndResetsFailedBaseline(t *testing.T) {
	var calls int
	failCPU := false
	c := newCollector(sources{
		cpu: func(context.Context) (cpuTimes, error) {
			calls++
			if failCPU {
				return cpuTimes{}, errors.New("CPU unavailable")
			}
			return cpuTimes{uint64(calls), uint64(calls * 2), uint64(calls * 3)}, nil
		},
		query: func(ctx context.Context) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() },
	})
	collect := func() (Snapshot, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		return c.Collect(ctx)
	}
	_, _ = collect()
	start := time.Now()
	s, err := collect()
	if !errors.Is(err, context.DeadlineExceeded) || !s.Stale || s.CPU.Utilization.Value == nil || time.Since(start) > time.Second {
		t.Fatalf("timeout must preserve measured CPU: %+v, %v", s, err)
	}
	failCPU = true
	_, _ = collect()
	failCPU = false
	s, _ = collect()
	if s.CPU.Utilization.Value != nil || !strings.Contains(s.CPU.Utilization.Reason, "First") {
		t.Fatal("CPU failure must clear baseline")
	}
}

func TestCollectorCancellationWhileWaitingForSerialGate(t *testing.T) {
	var called atomic.Int32
	c := newCollector(sources{
		cpu: func(context.Context) (cpuTimes, error) { called.Add(1); return cpuTimes{}, nil },
	})
	c.gate <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s, err := c.Collect(ctx)
	if !errors.Is(err, context.Canceled) || !s.Stale || called.Load() != 0 {
		t.Fatalf("canceled waiter executed source: %+v, %v", s, err)
	}
	<-c.gate
}

func TestRunSerialAndCancellation(t *testing.T) {
	var active, calls atomic.Int32
	c := newCollector(sources{
		cpu: func(ctx context.Context) (cpuTimes, error) {
			if active.Add(1) != 1 {
				t.Error("overlapping sampling")
			}
			defer active.Add(-1)
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 10*time.Second {
				t.Error("sampling lacks bounded timeout")
			}
			return cpuTimes{}, nil
		},
		query: func(context.Context) ([]byte, error) { return []byte(`{"Errors":{}}`), nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run(ctx, time.Hour, func(Snapshot, error) {
		calls.Add(1)
		cancel()
	}, c)
	if calls.Load() != 1 {
		t.Fatalf("expected exactly one initial publication, got %d", calls.Load())
	}
}

func TestInvalidProviderValuesAndBounds(t *testing.T) {
	for _, input := range []string{
		`not json`, `{}`, `{"Errors":{},"Physical":[{"Index":-1}]}`,
		strings.Repeat("x", maxProviderOutput+1),
	} {
		s := emptySnapshot()
		if err := decodeProviders([]byte(input), &s); err == nil {
			t.Fatalf("accepted invalid provider input")
		}
	}
	for _, v := range []float64{-1, math.NaN(), math.Inf(1)} {
		if got := measured(v, "%", "test"); got.Value != nil || got.Reason == "" {
			t.Fatalf("invalid measurement accepted: %+v", got)
		}
	}
	s := emptySnapshot()
	err := decodeProviders([]byte(`{"Errors":{},"Engines":[
		{"Name":"pid_1_luid_0x0_0x1_phys_0_eng_0_engtype_3D","UtilizationPercentage":70},
		{"Name":"pid_2_luid_0x0_0x1_phys_0_eng_0_engtype_3D","UtilizationPercentage":60}]}`), &s)
	if err != nil || s.GPUs[0].Utilization.Value != nil || !strings.Contains(s.GPUs[0].Utilization.Reason, "not clamped") {
		t.Fatal("invalid aggregated GPU utilization must not be clamped to fabricated success")
	}
}
