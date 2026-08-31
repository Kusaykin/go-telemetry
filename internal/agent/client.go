package agent

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"

	models "github.com/Kusaykin/go-telemetry/internal/model"
)

const requestTimeout = 5 * time.Second

type Client struct {
	rest *resty.Client
}

func NewClient(addr string) *Client {
	return &Client{
		rest: resty.New().
			SetBaseURL("http://"+addr).
			SetTimeout(requestTimeout).
			SetHeader("Content-Type", "text/plain"),
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

	path := "/update/" + url.PathEscape(m.MType) + "/" + url.PathEscape(m.ID) + "/" + url.PathEscape(value)

	resp, err := c.rest.R().Post(path)
	if err != nil {
		return fmt.Errorf("send %s: %w", m.ID, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("send %s: unexpected status %d", m.ID, resp.StatusCode())
	}

	return nil
}

func metricValue(m models.Metrics) (string, error) {
	return m.ValueString()
}
