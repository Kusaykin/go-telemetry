package models

import (
	"fmt"
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
type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Hash  string   `json:"hash,omitempty"`
}

// ValueString возвращает значение метрики в текстовом виде — в том формате,
// в котором оно передаётся в URL и отдаётся сервером.
func (m Metrics) ValueString() (string, error) {
	switch m.MType {
	case Gauge:
		if m.Value == nil {
			return "", fmt.Errorf("gauge %s: value is not set", m.ID)
		}
		return FormatGauge(*m.Value), nil
	case Counter:
		if m.Delta == nil {
			return "", fmt.Errorf("counter %s: delta is not set", m.ID)
		}
		return FormatCounter(*m.Delta), nil
	default:
		return "", fmt.Errorf("metric %s: unknown type %q", m.ID, m.MType)
	}
}

// FormatGauge и FormatCounter — единый формат значений для агента и сервера.
func FormatGauge(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func FormatCounter(delta int64) string {
	return strconv.FormatInt(delta, 10)
}
