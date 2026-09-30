package agent

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/mailru/easyjson"

	"github.com/Kusaykin/go-telemetry/internal/compress"
	models "github.com/Kusaykin/go-telemetry/internal/model"
)

const (
	requestTimeout  = 5 * time.Second
	contentTypeJSON = "application/json"
)

type Client struct {
	rest *resty.Client
}

func NewClient(addr string) *Client {
	return &Client{
		rest: resty.New().
			SetBaseURL("http://"+addr).
			SetTimeout(requestTimeout).
			SetHeader("Content-Type", contentTypeJSON),
	}
}

func (c *Client) SendAll(metrics []models.Metrics) error {
	for _, m := range metrics {
		if err := c.Send(m); err != nil {
			return err
		}
	}

	return nil
}

func (c *Client) Send(m models.Metrics) error {
	if err := m.Validate(); err != nil {
		return err
	}

	body, err := easyjson.Marshal(m)
	if err != nil {
		return fmt.Errorf("send %s: %w", m.ID, err)
	}

	body, err = compress.Compress(body)
	if err != nil {
		return fmt.Errorf("send %s: %w", m.ID, err)
	}

	resp, err := c.rest.R().
		SetHeader("Content-Encoding", "gzip").
		SetBody(body).
		Post("/update")
	if err != nil {
		return fmt.Errorf("send %s: %w", m.ID, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("send %s: unexpected status %d", m.ID, resp.StatusCode())
	}

	return nil
}
