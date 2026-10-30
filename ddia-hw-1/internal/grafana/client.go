package grafana

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	URL      string
	Token    string
	Password string
	HTTP     *http.Client
}

func New(url string) *Client {
	return &Client{URL: strings.TrimRight(url, "/"), HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Do(ctx context.Context, method, path string, body, result any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else if c.Password != "" {
		req.SetBasicAuth("admin", c.Password)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s returned HTTP %d", method, path, resp.StatusCode)
	}
	if result != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(result)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func InitPassword(path string) error {
	if data, err := os.ReadFile(path); err == nil {
		if len(strings.TrimSpace(string(data))) < 24 {
			return fmt.Errorf("existing password file is invalid")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	return writeSecret(path, base64.RawURLEncoding.EncodeToString(secret))
}

func ReadSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("empty secret file: %s", path)
	}
	return secret, nil
}

func writeSecret(path, value string) error {
	return os.WriteFile(path, []byte(value), 0444)
}

func (c *Client) Bootstrap(ctx context.Context, tokenPath string) error {
	if token, err := ReadSecret(tokenPath); err == nil {
		check := *c
		check.Token = token
		check.Password = ""
		if check.Do(ctx, "GET", "/api/org", nil, nil) == nil {
			return nil
		}
	}
	const name = "ddia-dashboard-generator"
	var accounts struct {
		Accounts []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"serviceAccounts"`
	}
	if err := c.Do(ctx, "GET", "/api/serviceaccounts/search?query="+name, nil, &accounts); err != nil {
		return err
	}
	id := 0
	for _, account := range accounts.Accounts {
		if account.Name == name {
			id = account.ID
		}
	}
	if id == 0 {
		var created struct {
			ID int `json:"id"`
		}
		if err := c.Do(ctx, "POST", "/api/serviceaccounts", map[string]any{"name": name, "role": "Editor"}, &created); err != nil {
			return err
		}
		id = created.ID
	}
	if id == 0 {
		return fmt.Errorf("Grafana returned an empty service account ID")
	}
	path := fmt.Sprintf("/api/serviceaccounts/%d/tokens", id)
	var tokens []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if err := c.Do(ctx, "GET", path, nil, &tokens); err != nil {
		return err
	}
	for _, token := range tokens {
		if token.Name == "compose-generator" {
			if err := c.Do(ctx, "DELETE", fmt.Sprintf("%s/%d", path, token.ID), nil, nil); err != nil {
				return err
			}
		}
	}
	var token struct {
		Key string `json:"key"`
	}
	if err := c.Do(ctx, "POST", path, map[string]any{"name": "compose-generator", "secondsToLive": 0}, &token); err != nil {
		return err
	}
	if token.Key == "" {
		return fmt.Errorf("Grafana returned an empty token")
	}
	return writeSecret(tokenPath, token.Key)
}
