package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

type Options struct {
	RawDir       string
	APIKey       string
	BaseURL      string
	Client       *http.Client
	Log          *slog.Logger
	StateDir     string
	RefreshAfter time.Duration
	Now          func() time.Time
	Sleep        func(context.Context, time.Duration) error
	retries      *int
}

type Summary struct {
	Discovered int
	Fetched    int
	Unchanged  int
	Cached     int
	Pending    int
	Failed     int
	Retries    int
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
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Sleep == nil {
		options.Sleep = sleep
	}
	options.retries = &summary.Retries
	store, err := coordinate(options)
	if err != nil {
		summary.Failed++
		return summary, err
	}
	defer store.close()
	client := http.Client{}
	if options.Client != nil {
		client = *options.Client
	}
	if client.Timeout <= 0 || client.Timeout > requestTimeout {
		client.Timeout = requestTimeout
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	options.Client = &client
	seen := make(map[string]bool)
	for page := 1; ; page++ {
		options.Log.Debug("listing page", "event", "page_requested", "page", page)
		body, err := request(ctx, options, "/public/recordings?page="+strconv.Itoa(page)+"&limit=100", "")
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
		var unique []string
		for _, record := range records {
			if seen[record.ID] {
				continue
			}
			seen[record.ID] = true
			summary.Discovered++
			unique = append(unique, record.ID)
		}
		for _, id := range unique {
			if ctx.Err() != nil {
				summary.Failed++
				return summary, errInterrupted
			}
			if !safeID.MatchString(id) {
				summary.Failed++
				options.Log.Error("unsafe recording ID rejected", "event", "recording_failed", "reason", "invalid_id")
				continue
			}
			if store.fresh(id, options.Now(), options.RefreshAfter) {
				summary.Cached++
				options.Log.Debug("recording cached", "event", "recording_cached", "recording_id", id)
				continue
			}
			started := time.Now()
			data, pending, err := detail(ctx, options, id)
			if err == nil {
				completed := options.Now()
				var changed bool
				changed, err = snapshot(ctx, store.raw, id, data)
				if err == nil {
					summary.Fetched++
					if !changed {
						summary.Unchanged++
					}
					if pending {
						summary.Pending++
					}
					options.Log.Info("recording fetched", "event", "recording_fetched", "recording_id", id,
						"elapsed_ms", time.Since(started).Milliseconds(), "changed", changed, "pending", pending)
					err = store.remember(ctx, id, completed)
					if err == nil {
						continue
					}
				}
			}
			summary.Failed++
			options.Log.Error("recording failed", "event", "recording_failed", "recording_id", id,
				"elapsed_ms", time.Since(started).Milliseconds(), "reason", err.Error())
			if errors.Is(err, errAuth) || ctx.Err() != nil {
				return summary, err
			}
		}
		if !*listing.Page.HasMore {
			break
		}
	}
	if ctx.Err() != nil {
		summary.Failed++
		return summary, errInterrupted
	}
	if summary.Failed != 0 {
		return summary, errIncomplete
	}
	return summary, nil
}

func detail(ctx context.Context, options Options, id string) (json.RawMessage, bool, error) {
	body, err := request(ctx, options, "/public/recordings/"+id, id)
	if err != nil {
		return nil, false, err
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || !envelope.Success {
		return nil, false, errors.New("invalid detail response")
	}
	pending, valid := validRecording(envelope.Data, id)
	if !valid {
		return nil, false, errors.New("invalid detail response")
	}
	return envelope.Data, pending, nil
}

func validRecording(data []byte, id string) (pending, valid bool) {
	var object map[string]json.RawMessage
	var detailID string
	if json.Unmarshal(data, &object) != nil || object == nil ||
		json.Unmarshal(object["id"], &detailID) != nil || detailID != id {
		return false, false
	}
	transcript := bytes.TrimSpace(object["transcript"])
	return len(transcript) == 0 || bytes.Equal(transcript, []byte("null")), true
}
