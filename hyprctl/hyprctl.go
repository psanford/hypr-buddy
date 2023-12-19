package hyprctl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
)

type Client struct {
	p string
}

func New() (*Client, error) {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		return nil, errors.New("HYPRLAND_INSTANCE_SIGNATURE not set")
	}

	path := fmt.Sprintf("/tmp/hypr/%s/.socket.sock", sig)
	return NewFromPath(path)
}

func NewFromPath(path string) (*Client, error) {
	c := &Client{
		p: path,
	}

	socket, err := c.conn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", path, err)
	}
	socket.Close()

	return c, nil
}

func (c *Client) conn() (net.Conn, error) {
	return net.Dial("unix", c.p)
}

func (c *Client) ActiveWorkspace() (*ActiveWorkspace, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", c.p, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("j/activeworkspace"))
	if err != nil {
		return nil, err
	}

	d := json.NewDecoder(conn)
	var resp ActiveWorkspace
	err = d.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) DispatchRaw(args string) error {
	conn, err := c.conn()
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.p, err)
	}
	defer conn.Close()

	fmt.Fprintf(conn, "/dispatch %s", args)
	r := bufio.NewReader(conn)
	b, err := r.ReadBytes('\n')
	if err != nil {
		return err
	}
	if !bytes.Equal(b, []byte("ok")) {
		return fmt.Errorf("error result: %s", b)
	}
	return nil
}

type ActiveWorkspace struct {
	HasFullScreen   bool   `json:"hasfullscreen"`
	ID              int64  `json:"id"`
	LastWindow      string `json:"lastwindow"`
	LastWindowTitle string `json:"lastwindowtitle"`
	Monitor         string `json:"monitor"`
	MonitorID       int64  `json:"monitorID"`
	Name            string `json:"name"`
	Windows         int64  `json:"windows"`
}
