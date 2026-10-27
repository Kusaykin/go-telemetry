package handler

import (
	"html/template"
	"maps"
	"net/http"
	"slices"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

type metric struct {
	Name  string
	Value string
}

var indexTemplate = template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Метрики</title></head>
<body>
<h1>Метрики</h1>
<h2>Gauge</h2>
<ul>{{range .Gauges}}
    <li>{{.Name}}: {{.Value}}</li>{{end}}
</ul>
<h2>Counter</h2>
<ul>{{range .Counters}}
    <li>{{.Name}}: {{.Value}}</li>{{end}}
</ul>
</body>
</html>
`))

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	gauges, err := h.store.Gauges(r.Context())
	if err != nil {
		h.storageError(w, r, err)
		return
	}

	counters, err := h.store.Counters(r.Context())
	if err != nil {
		h.storageError(w, r, err)
		return
	}

	page := struct {
		Gauges   []metric
		Counters []metric
	}{
		Gauges:   make([]metric, 0, len(gauges)),
		Counters: make([]metric, 0, len(counters)),
	}

	// сортируем имена, чтобы порядок в выдаче был стабильным
	for _, name := range slices.Sorted(maps.Keys(gauges)) {
		page.Gauges = append(page.Gauges, metric{Name: name, Value: models.FormatGauge(gauges[name])})
	}

	for _, name := range slices.Sorted(maps.Keys(counters)) {
		page.Counters = append(page.Counters, metric{Name: name, Value: models.FormatCounter(counters[name])})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if err := indexTemplate.Execute(w, page); err != nil {
		return
	}
}
