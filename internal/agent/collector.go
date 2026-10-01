package agent

import (
	"math/rand"
	"runtime"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

// метрики которых нет в runtime.MemStats.
const (
	PollCountName   = "PollCount"
	RandomValueName = "RandomValue"
)

type Collector struct {
	gauges    map[string]float64
	pollCount int64
}

func NewCollector() *Collector {
	return &Collector{}
}

func (c *Collector) Poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	c.pollCount++
	c.gauges = map[string]float64{
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
}

func (c *Collector) Snapshot() []models.Metrics {
	snapshot := make([]models.Metrics, 0, len(c.gauges)+1)

	for name, value := range c.gauges {
		snapshot = append(snapshot, models.Metrics{
			ID:    name,
			MType: models.Gauge,
			Value: &value,
		})
	}

	pollCount := c.pollCount
	snapshot = append(snapshot, models.Metrics{
		ID:    PollCountName,
		MType: models.Counter,
		Delta: &pollCount,
	})

	return snapshot
}

func (c *Collector) PollCountDelta() int64 {
	return c.pollCount
}

func (c *Collector) AckPollCount(delta int64) {
	c.pollCount -= delta
}
