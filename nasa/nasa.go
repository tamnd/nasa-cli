// Package nasa is the library behind the nasa command line:
// the HTTP client, request shaping, and the typed data models for NASA APIs.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// All endpoints require an API key; defaults to "DEMO_KEY" (30 req/hr).
// Register a free key at api.nasa.gov for 1000 req/hr.
package nasa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Config holds all tunable parameters for the NASA client.
type Config struct {
	BaseURL   string
	UserAgent string
	APIKey    string
	Rate      time.Duration
	Timeout   time.Duration
	Retries   int
}

// DefaultConfig returns sensible defaults for the NASA API client.
func DefaultConfig() Config {
	return Config{
		BaseURL:   "https://api.nasa.gov",
		UserAgent: "nasa-cli/0.1.0 (github.com/tamnd/nasa-cli)",
		APIKey:    "DEMO_KEY",
		Rate:      200 * time.Millisecond,
		Timeout:   30 * time.Second,
		Retries:   3,
	}
}

// Host is the primary domain this driver claims for URI routing.
const Host = "api.nasa.gov"

// Client talks to the NASA API over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	APIKey    string
	cfg       Config

	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with DefaultConfig settings.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		APIKey:    cfg.APIKey,
		cfg:       cfg,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

// APOD fetches the Astronomy Picture of the Day for a specific date (YYYY-MM-DD).
// If date is empty, the API returns today's APOD.
func (c *Client) APOD(ctx context.Context, date string) (*APOD, error) {
	u := c.buildURL("/planetary/apod", "date", date)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var a APOD
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("apod decode: %w", err)
	}
	return &a, nil
}

// APODRange fetches a range of APODs between start and end (YYYY-MM-DD).
func (c *Client) APODRange(ctx context.Context, start, end string) ([]APOD, error) {
	u := c.buildURL("/planetary/apod", "start_date", start, "end_date", end)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var out []APOD
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("apod-range decode: %w", err)
	}
	return out, nil
}

// EPIC fetches the latest natural-color EPIC Earth images.
func (c *Client) EPIC(ctx context.Context) ([]EPICImage, error) {
	u := c.buildURL("/EPIC/api/natural")
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw []wireEPICImage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("epic decode: %w", err)
	}
	out := make([]EPICImage, len(raw))
	for i, r := range raw {
		out[i] = EPICImage{
			Identifier: r.Identifier,
			Caption:    r.Caption,
			Date:       r.Date,
			Lat:        r.CentroidCoordinates.Lat,
			Lon:        r.CentroidCoordinates.Lon,
			ImageURL:   epicImageURL(r.Date, r.Image),
		}
	}
	return out, nil
}

// RoverPhotos fetches photos taken by a Mars rover on a specific Martian sol.
// limit <= 0 returns all photos on that sol (up to one API page, ~25 by default).
func (c *Client) RoverPhotos(ctx context.Context, rover string, sol, limit int) ([]RoverPhoto, error) {
	if rover == "" {
		rover = "curiosity"
	}
	u := c.buildURL(
		"/mars-photos/api/v1/rovers/"+rover+"/photos",
		"sol", fmt.Sprintf("%d", sol),
		"page", "1",
	)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw wireRoverResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("rover decode: %w", err)
	}
	out := make([]RoverPhoto, 0, len(raw.Photos))
	for i, p := range raw.Photos {
		if limit > 0 && i >= limit {
			break
		}
		out = append(out, RoverPhoto{
			ID:        p.ID,
			Sol:       p.Sol,
			Camera:    p.Camera.Name,
			ImageURL:  p.ImgSrc,
			EarthDate: p.EarthDate,
			RoverName: p.Rover.Name,
		})
	}
	return out, nil
}

// NEOFeed fetches near-Earth objects between start and end dates (YYYY-MM-DD).
// The NASA API limits the window to 7 days.
func (c *Client) NEOFeed(ctx context.Context, start, end string) ([]NEOSummary, error) {
	u := c.buildURL("/neo/rest/v1/feed", "start_date", start, "end_date", end)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var raw wireNEOResp
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("neo decode: %w", err)
	}

	// Flatten the date-keyed map into a sorted slice.
	var out []NEOSummary
	for date, objects := range raw.NearEarthObjects {
		for _, o := range objects {
			out = append(out, NEOSummary{
				Date:          date,
				Name:          o.Name,
				IsHazardous:   o.IsPotentiallyHazardousAsteroid,
				DiameterMinKm: o.EstimatedDiameter.Kilometers.Min,
				DiameterMaxKm: o.EstimatedDiameter.Kilometers.Max,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date < out[j].Date
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// buildURL assembles a full request URL with api_key and optional key=value pairs.
// Pairs with empty values are skipped.
func (c *Client) buildURL(path string, params ...string) string {
	apiKey := c.APIKey
	if apiKey == "" {
		apiKey = "DEMO_KEY"
	}
	sb := strings.Builder{}
	sb.WriteString(c.cfg.BaseURL)
	sb.WriteString(path)
	sb.WriteString("?api_key=")
	sb.WriteString(apiKey)
	for i := 0; i+1 < len(params); i += 2 {
		if params[i+1] != "" {
			sb.WriteString("&")
			sb.WriteString(params[i])
			sb.WriteString("=")
			sb.WriteString(params[i+1])
		}
	}
	return sb.String()
}

// get fetches url with pacing and retries. The body is fully read and closed.
func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// epicImageURL constructs the GSFC archive PNG URL for an EPIC image.
// date format: "2024-01-15 00:37:48" → "2024/01/15"
func epicImageURL(date, image string) string {
	parts := strings.SplitN(date, " ", 2)
	d := strings.ReplaceAll(parts[0], "-", "/")
	return "https://epic.gsfc.nasa.gov/archive/natural/" + d + "/png/" + image + ".png"
}

// --- Public data types ---

// APOD is the Astronomy Picture of the Day record.
type APOD struct {
	Date        string `json:"date"        kit:"id"`
	Title       string `json:"title"`
	Explanation string `json:"explanation,omitempty" kit:"body"`
	MediaType   string `json:"media_type"`
	URL         string `json:"url"`
	HDURL       string `json:"hd_url,omitempty"`
	Copyright   string `json:"copyright,omitempty"`
}

// EPICImage is one Earth image from the DSCOVR EPIC camera.
type EPICImage struct {
	Identifier string  `json:"identifier"`
	Caption    string  `json:"caption,omitempty"`
	Date       string  `json:"date"`
	Lat        float64 `json:"lat,omitempty"`
	Lon        float64 `json:"lon,omitempty"`
	ImageURL   string  `json:"image_url,omitempty"`
}

// RoverPhoto is one photo taken by a Mars rover.
type RoverPhoto struct {
	ID        int    `json:"id"`
	Sol       int    `json:"sol"`
	Camera    string `json:"camera"`
	ImageURL  string `json:"image_url"`
	EarthDate string `json:"earth_date"`
	RoverName string `json:"rover"`
}

// NEOSummary is a flattened record for one near-Earth object on one date.
type NEOSummary struct {
	Date          string  `json:"date"`
	Name          string  `json:"name"`
	IsHazardous   bool    `json:"is_potentially_hazardous"`
	DiameterMinKm float64 `json:"diameter_min_km,omitempty"`
	DiameterMaxKm float64 `json:"diameter_max_km,omitempty"`
}

// --- Wire types (internal, not exported) ---

type wireRoverResp struct {
	Photos []struct {
		ID     int `json:"id"`
		Sol    int `json:"sol"`
		Camera struct {
			Name     string `json:"name"`
			FullName string `json:"full_name"`
		} `json:"camera"`
		ImgSrc    string `json:"img_src"`
		EarthDate string `json:"earth_date"`
		Rover     struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"rover"`
	} `json:"photos"`
}

type wireEPICImage struct {
	Identifier          string `json:"identifier"`
	Caption             string `json:"caption"`
	Date                string `json:"date"`
	CentroidCoordinates struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"centroid_coordinates"`
	Image string `json:"image"`
}

type wireNEOResp struct {
	NearEarthObjects map[string][]struct {
		Name                           string `json:"name"`
		IsPotentiallyHazardousAsteroid bool   `json:"is_potentially_hazardous_asteroid"`
		EstimatedDiameter              struct {
			Kilometers struct {
				Min float64 `json:"estimated_diameter_min"`
				Max float64 `json:"estimated_diameter_max"`
			} `json:"kilometers"`
		} `json:"estimated_diameter"`
	} `json:"near_earth_objects"`
}
