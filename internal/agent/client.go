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

func (c *Client) SendBatch(metrics []models.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}

	if err := c.post("/updates/", models.MetricsList(metrics)); err != nil {
		return fmt.Errorf("send batch of %d: %w", len(metrics), err)
	}

	return nil
}

func (c *Client) post(path string, v easyjson.Marshaler) error {
	body, err := easyjson.Marshal(v)
	if err != nil {
		return fmt.Errorf("post %s: marshal: %w", path, err)
	}

	body, err = compress.Compress(body)
	if err != nil {
		return fmt.Errorf("post %s: compress: %w", path, err)
	}

	resp, err := c.rest.R().
		SetHeader("Content-Encoding", "gzip").
		SetBody(body).
		Post(path)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("post %s: unexpected status %d", path, resp.StatusCode())
	}

	return nil
}
