package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/mailru/easyjson"

	"github.com/Kusaykin/go-telemetry/internal/compress"
	models "github.com/Kusaykin/go-telemetry/internal/model"
	"github.com/Kusaykin/go-telemetry/internal/retry"
)

const (
	requestTimeout  = 15 * time.Second
	dialTimeout     = 2 * time.Second
	contentTypeJSON = "application/json"
)

type Client struct {
	rest   *resty.Client
	delays []time.Duration
}

func NewClient(addr string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext

	return &Client{
		rest: resty.New().
			SetTransport(transport).
			SetBaseURL("http://"+addr).
			SetTimeout(requestTimeout).
			SetHeader("Content-Type", contentTypeJSON),
		delays: retry.Delays(),
	}
}

func (c *Client) SendBatch(ctx context.Context, metrics []models.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}

	if err := c.post(ctx, "/updates/", models.MetricsList(metrics)); err != nil {
		return fmt.Errorf("send batch of %d: %w", len(metrics), err)
	}

	return nil
}

func (c *Client) post(ctx context.Context, path string, v easyjson.Marshaler) error {
	body, err := easyjson.Marshal(v)
	if err != nil {
		return fmt.Errorf("post %s: marshal: %w", path, err)
	}

	body, err = compress.Compress(body)
	if err != nil {
		return fmt.Errorf("post %s: compress: %w", path, err)
	}

	var resp *resty.Response
	err = retry.Do(ctx, c.delays, isConnError, func() error {
		resp, err = c.rest.R().
			SetContext(ctx).
			SetHeader("Content-Encoding", "gzip").
			SetBody(body).
			Post(path)
		return err
	})
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("post %s: unexpected status %d", path, resp.StatusCode())
	}

	return nil
}

func isConnError(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}
