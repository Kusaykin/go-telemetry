package agent

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

const requestTimeout = 5 * time.Second

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(addr string) *Client {
	return &Client{
		baseURL: "http://" + addr,
		http:    &http.Client{Timeout: requestTimeout},
	}
}

func (c *Client) SendAll(metrics []models.Metrics) error {
	var errs []error

	for _, m := range metrics {
		if err := c.Send(m); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (c *Client) Send(m models.Metrics) error {
	value, err := metricValue(m)
	if err != nil {
		return err
	}

	endpoint := c.baseURL + "/update/" + url.PathEscape(m.MType) + "/" + url.PathEscape(m.ID) + "/" + url.PathEscape(value)

	req, err := http.NewRequest(http.MethodPost, endpoint, http.NoBody)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", m.ID, err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("send %s: %w", m.ID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send %s: unexpected status %d", m.ID, resp.StatusCode)
	}

	return nil
}

func metricValue(m models.Metrics) (string, error) {
	switch m.MType {
	case models.Gauge:
		if m.Value == nil {
			return "", fmt.Errorf("gauge %s: value is not set", m.ID)
		}
		return strconv.FormatFloat(*m.Value, 'f', -1, 64), nil
	case models.Counter:
		if m.Delta == nil {
			return "", fmt.Errorf("counter %s: delta is not set", m.ID)
		}
		return strconv.FormatInt(*m.Delta, 10), nil
	default:
		return "", fmt.Errorf("metric %s: unknown type %q", m.ID, m.MType)
	}
}
