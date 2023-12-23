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

func (c *Client) ActiveWorkspace() (*Workspace, error) {
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
	var resp Workspace
	err = d.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *Client) Monitors() ([]Monitor, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", c.p, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("j/monitors"))
	if err != nil {
		return nil, err
	}

	d := json.NewDecoder(conn)
	var resp []Monitor
	err = d.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *Client) Windows() ([]Window, error) {
	conn, err := c.conn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", c.p, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte("j/clients"))
	if err != nil {
		return nil, err
	}

	d := json.NewDecoder(conn)
	var resp []Window
	err = d.Decode(&resp)
	if err != nil {
		return nil, err
	}

	return resp, nil
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

type Workspace struct {
	HasFullScreen   bool   `json:"hasfullscreen"`
	ID              int64  `json:"id"`
	LastWindow      string `json:"lastwindow"`
	LastWindowTitle string `json:"lastwindowtitle"`
	Monitor         string `json:"monitor"`
	MonitorID       int64  `json:"monitorID"`
	Name            string `json:"name"`
	Windows         int64  `json:"windows"`
}

type Window struct {
	Address        string        `json:"address"`
	At             []int64       `json:"at"`
	Class          string        `json:"class"`
	FakeFullscreen bool          `json:"fakeFullscreen"`
	Floating       bool          `json:"floating"`
	FocusHistoryID int64         `json:"focusHistoryID"`
	Fullscreen     bool          `json:"fullscreen"`
	FullscreenMode int64         `json:"fullscreenMode"`
	Grouped        []interface{} `json:"grouped"`
	Hidden         bool          `json:"hidden"`
	InitialClass   string        `json:"initialClass"`
	InitialTitle   string        `json:"initialTitle"`
	Mapped         bool          `json:"mapped"`
	Monitor        int64         `json:"monitor"`
	Pid            int64         `json:"pid"`
	Pinned         bool          `json:"pinned"`
	Size           []int64       `json:"size"`
	Swallowing     string        `json:"swallowing"`
	Title          string        `json:"title"`
	Workspace      struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"workspace"`
	Xwayland bool `json:"xwayland"`
}

type Monitor struct {
	ActiveWorkspace struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"activeWorkspace"`
	ActivelyTearing  bool    `json:"activelyTearing"`
	Description      string  `json:"description"`
	DpmsStatus       bool    `json:"dpmsStatus"`
	Focused          bool    `json:"focused"`
	Height           int64   `json:"height"`
	ID               int64   `json:"id"`
	Make             string  `json:"make"`
	Model            string  `json:"model"`
	Name             string  `json:"name"`
	RefreshRate      float64 `json:"refreshRate"`
	Reserved         []int64 `json:"reserved"`
	Scale            float64 `json:"scale"`
	Serial           string  `json:"serial"`
	SpecialWorkspace struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"specialWorkspace"`
	Transform int64 `json:"transform"`
	Vrr       bool  `json:"vrr"`
	Width     int64 `json:"width"`
	X         int64 `json:"x"`
	Y         int64 `json:"y"`
}
