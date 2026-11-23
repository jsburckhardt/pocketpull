package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
)

type Options struct {
	RawDir  string
	APIKey  string
	BaseURL string
	Client  *http.Client
	Log     *slog.Logger
}

type Summary struct {
	Discovered int
	Fetched    int
	Unchanged  int
	Pending    int
	Failed     int
}

var (
	safeID         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
	errAuth        = errors.New("authentication rejected")
	errIncomplete  = errors.New("required recording work failed")
	errInterrupted = errors.New("collection interrupted")
)

// Run hides list traversal, detail validation and snapshot coordination. Errors
// describe operational categories only, never HTTP bodies, URLs or OS paths.
func Run(ctx context.Context, options Options) (Summary, error) {
	var summary Summary
	if err := os.MkdirAll(options.RawDir, 0o700); err != nil {
		summary.Failed++
		return summary, errors.New("raw directory unavailable")
	}
	root, err := os.OpenRoot(options.RawDir)
	if err != nil {
		summary.Failed++
		return summary, errors.New("raw directory unavailable")
	}
	defer root.Close()
	client := *options.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	options.Client = &client
	seen := make(map[string]bool)
	for page := 1; ; page++ {
		body, err := request(ctx, options, "/public/recordings?page="+strconv.Itoa(page)+"&limit=100")
		if err != nil {
			summary.Failed++
			return summary, err
		}
		var listing struct {
			Success bool            `json:"success"`
			Data    json.RawMessage `json:"data"`
			Page    struct {
				HasMore *bool `json:"has_more"`
				Number  int   `json:"page"`
			} `json:"pagination"`
		}
		var records []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(body, &listing) != nil || !listing.Success || listing.Page.HasMore == nil ||
			listing.Page.Number != page || json.Unmarshal(listing.Data, &records) != nil || records == nil {
			summary.Failed++
			return summary, errors.New("invalid listing response")
		}
		for _, record := range records {
			if !safeID.MatchString(record.ID) {
				summary.Failed++
				options.Log.Error("unsafe recording ID rejected", "event", "recording_failed", "reason", "invalid_id")
				continue
			}
			if seen[record.ID] {
				continue
			}
			seen[record.ID] = true
			summary.Discovered++
			if ctx.Err() != nil {
				return summary, errInterrupted
			}
			data, pending, err := detail(ctx, options, record.ID)
			if err == nil {
				var changed bool
				changed, err = snapshot(ctx, root, record.ID, data)
				if err == nil {
					summary.Fetched++
					if !changed {
						summary.Unchanged++
					}
					if pending {
						summary.Pending++
					}
					options.Log.Info("recording fetched", "event", "recording_fetched", "recording_id", record.ID, "changed", changed, "pending", pending)
					continue
				}
			}
			summary.Failed++
			options.Log.Error("recording failed", "event", "recording_failed", "recording_id", record.ID, "reason", err.Error())
			if errors.Is(err, errAuth) || ctx.Err() != nil {
				return summary, err
			}
		}
		if !*listing.Page.HasMore {
			break
		}
	}
	if summary.Failed != 0 {
		return summary, errIncomplete
	}
	return summary, nil
}

func request(ctx context.Context, options Options, route string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, options.BaseURL+route, nil)
	if err != nil {
		return nil, errors.New("request setup failed")
	}
	req.Header.Set("Authorization", "Bearer "+options.APIKey)
	req.Header.Set("Accept", "application/json")
	response, err := options.Client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errInterrupted
		}
		return nil, errors.New("transport failed")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, errAuth
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", response.StatusCode)
	}
	const maxResponseBytes = 32 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.New("response read failed")
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("response too large")
	}
	return body, nil
}

func detail(ctx context.Context, options Options, id string) (json.RawMessage, bool, error) {
	body, err := request(ctx, options, "/public/recordings/"+id)
	if err != nil {
		return nil, false, err
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	var object map[string]json.RawMessage
	var detailID string
	if json.Unmarshal(body, &envelope) != nil || !envelope.Success ||
		json.Unmarshal(envelope.Data, &object) != nil || object == nil ||
		json.Unmarshal(object["id"], &detailID) != nil || detailID != id {
		return nil, false, errors.New("invalid detail response")
	}
	transcript := bytes.TrimSpace(object["transcript"])
	return envelope.Data, len(transcript) == 0 || bytes.Equal(transcript, []byte("null")), nil
}
