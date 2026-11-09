package agent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand"
	"runtime"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

// метрики которых нет в runtime.MemStats.
const (
	PollCountName   = "PollCount"
	RandomValueName = "RandomValue"
)

const (
	TotalMemoryName        = "TotalMemory"
	FreeMemoryName         = "FreeMemory"
	CPUUtilizationPrefix   = "CPUutilization"
	systemGaugesWithoutCPU = 2
)

type Collector struct {
	mu        sync.Mutex
	runtime   map[string]float64
	system    map[string]float64
	pollCount int64
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) Poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	gauges := map[string]float64{
		"Alloc":         float64(m.Alloc),
		"BuckHashSys":   float64(m.BuckHashSys),
		"Frees":         float64(m.Frees),
		"GCCPUFraction": m.GCCPUFraction,
		"GCSys":         float64(m.GCSys),
		"HeapAlloc":     float64(m.HeapAlloc),
		"HeapIdle":      float64(m.HeapIdle),
		"HeapInuse":     float64(m.HeapInuse),
		"HeapObjects":   float64(m.HeapObjects),
		"HeapReleased":  float64(m.HeapReleased),
		"HeapSys":       float64(m.HeapSys),
		"LastGC":        float64(m.LastGC),
		"Lookups":       float64(m.Lookups),
		"MCacheInuse":   float64(m.MCacheInuse),
		"MCacheSys":     float64(m.MCacheSys),
		"MSpanInuse":    float64(m.MSpanInuse),
		"MSpanSys":      float64(m.MSpanSys),
		"Mallocs":       float64(m.Mallocs),
		"NextGC":        float64(m.NextGC),
		"NumForcedGC":   float64(m.NumForcedGC),
		"NumGC":         float64(m.NumGC),
		"OtherSys":      float64(m.OtherSys),
		"PauseTotalNs":  float64(m.PauseTotalNs),
		"StackInuse":    float64(m.StackInuse),
		"StackSys":      float64(m.StackSys),
		"Sys":           float64(m.Sys),
		"TotalAlloc":    float64(m.TotalAlloc),

		RandomValueName: rand.Float64(),
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.pollCount++
	c.runtime = gauges
}

func (c *Collector) PollSystem(ctx context.Context) error {
	var errs []error

	gauges := make(map[string]float64, systemGaugesWithoutCPU+runtime.NumCPU())

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		errs = append(errs, fmt.Errorf("poll system: memory: %w", err))
	} else {
		gauges[TotalMemoryName] = float64(vm.Total)
		gauges[FreeMemoryName] = float64(vm.Free)
	}

	percents, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		errs = append(errs, fmt.Errorf("poll system: cpu: %w", err))
	} else {
		for i, p := range percents {
			gauges[CPUUtilizationPrefix+strconv.Itoa(i+1)] = p
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.system == nil {
		c.system = make(map[string]float64, len(gauges))
	}
	maps.Copy(c.system, gauges)

	return errors.Join(errs...)
}

func (c *Collector) TakeSnapshot() ([]models.Metrics, int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := make([]models.Metrics, 0, len(c.runtime)+len(c.system)+1)

	for _, gauges := range []map[string]float64{c.runtime, c.system} {
		for name, value := range gauges {
			snapshot = append(snapshot, models.Metrics{
				ID:    name,
				MType: models.Gauge,
				Value: &value,
			})
		}
	}

	pollCount := c.pollCount
	c.pollCount = 0

	snapshot = append(snapshot, models.Metrics{
		ID:    PollCountName,
		MType: models.Counter,
		Delta: &pollCount,
	})

	return snapshot, pollCount
}

func (c *Collector) PollCount() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.pollCount
}

func (c *Collector) RestorePollCount(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.pollCount += delta
}
