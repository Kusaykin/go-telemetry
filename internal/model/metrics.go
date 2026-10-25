package models

import (
	"errors"
	"fmt"
	"math"
	"strconv"
)

const (
	Counter = "counter"
	Gauge   = "gauge"
)

// NOTE: Не усложняем пример, вводя иерархическую вложенность структур.
// Органичиваясь плоской моделью.
// Delta и Value объявлены через указатели,
// что бы отличать значение "0", от не заданного значения
// и соответственно не кодировать в структуру.
//
//go:generate go run github.com/mailru/easyjson/easyjson -output_filename metrics_easyjson.gen.go metrics.go
//easyjson:json
type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
}

// MetricsList — батч метрик для POST /updates/.
//
//easyjson:json
type MetricsList []Metrics

var ErrEmptyID = errors.New("metric id is empty")

// Validate проверяет, что у метрики есть имя, тип известен и для него задано значение.
func (m Metrics) Validate() error {
	if m.ID == "" {
		return ErrEmptyID
	}

	switch m.MType {
	case Gauge:
		if m.Value == nil {
			return fmt.Errorf("gauge %s: value is not set", m.ID)
		}
		if math.IsNaN(*m.Value) || math.IsInf(*m.Value, 0) {
			return fmt.Errorf("gauge %s: value %v is not a finite number", m.ID, *m.Value)
		}
	case Counter:
		if m.Delta == nil {
			return fmt.Errorf("counter %s: delta is not set", m.ID)
		}
	default:
		return fmt.Errorf("metric %s: unknown type %q", m.ID, m.MType)
	}

	return nil
}

func (m Metrics) ValueString() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}

	if m.MType == Gauge {
		return FormatGauge(*m.Value), nil
	}

	return FormatCounter(*m.Delta), nil
}

func FormatGauge(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func FormatCounter(delta int64) string {
	return strconv.FormatInt(delta, 10)
}

var ErrCounterOverflow = errors.New("counter overflow")

func AddDelta(value, delta int64) (int64, error) {
	sum := value + delta
	if (delta > 0 && sum < value) || (delta < 0 && sum > value) {
		return 0, fmt.Errorf("%w: %d + %d", ErrCounterOverflow, value, delta)
	}

	return sum, nil
}

type InvalidMetricError struct {
	Index  int
	Metric Metrics
	Err    error
}

func (e *InvalidMetricError) Error() string {
	return fmt.Sprintf("metric %d: %v", e.Index, e.Err)
}

func (e *InvalidMetricError) Unwrap() error {
	return e.Err
}

func ValidateBatch(metrics []Metrics) error {
	for i, m := range metrics {
		if err := m.Validate(); err != nil {
			return &InvalidMetricError{Index: i, Metric: m, Err: err}
		}
	}

	return nil
}

func Collapse(metrics []Metrics) (map[string]float64, map[string]int64, error) {
	if err := ValidateBatch(metrics); err != nil {
		return nil, nil, err
	}

	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	for _, m := range metrics {
		switch m.MType {
		case Gauge:
			gauges[m.ID] = *m.Value
		case Counter:
			sum, err := AddDelta(counters[m.ID], *m.Delta)
			if err != nil {
				return nil, nil, fmt.Errorf("counter %s: %w", m.ID, err)
			}
			counters[m.ID] = sum
		}
	}

	return gauges, counters, nil
}
