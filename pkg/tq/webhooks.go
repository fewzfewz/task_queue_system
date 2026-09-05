package tq

import (
	"context"
	"net/url"
)

// RegisterWebhookReq holds data to register a webhook.
type RegisterWebhookReq struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret,omitempty"`
	Events []string `json:"events,omitempty"`
}

// RegisterWebhook creates a new global webhook registration.
func (c *Client) RegisterWebhook(ctx context.Context, req RegisterWebhookReq) (*Webhook, error) {
	var out Webhook
	if err := c.doReq(ctx, "POST", "/api/v1/webhooks", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListWebhooks returns all registered webhooks.
func (c *Client) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	var out []Webhook
	if err := c.doReq(ctx, "GET", "/api/v1/webhooks", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetWebhook returns a specific webhook by ID.
func (c *Client) GetWebhook(ctx context.Context, id string) (*Webhook, error) {
	var out Webhook
	if err := c.doReq(ctx, "GET", "/api/v1/webhooks/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteWebhook deletes a registered webhook.
func (c *Client) DeleteWebhook(ctx context.Context, id string) error {
	return c.doReq(ctx, "DELETE", "/api/v1/webhooks/"+url.PathEscape(id), nil, nil)
}
