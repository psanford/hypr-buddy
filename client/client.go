package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/psanford/hypr-buddy/config"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return NewClientWithTimeout(config.SocketPath(), 30*time.Second)
}

func NewClientWithTimeout(sockPath string, tout time.Duration) *Client {
	dialer := net.Dialer{
		Timeout:   tout,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", sockPath)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
	}

	return &Client{
		httpClient: httpClient,
	}
}

var fakeHost = "http://example.com"

func (c *Client) Ping() error {
	body, err := c.plainRequest("/ping")
	if err != nil {
		return err
	}

	if body != "pong" {
		return fmt.Errorf("Bad response from server: %s", body)
	}
	return nil
}

func (c *Client) ToggleStack() error {
	_, err := c.plainRequest("/toggleStack")
	return err
}

func (c *Client) FocusNext() error {
	_, err := c.plainRequest("/focus?n=1")
	return err
}

func (c *Client) FocusPrev() error {
	_, err := c.plainRequest("/focus?n=-1")
	return err
}

func (c *Client) plainRequest(path string) (string, error) {
	resp, err := c.httpClient.Get(fakeHost + path)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Bad response from server: %d %s", resp.StatusCode, body)
	}

	return string(body), nil
}
