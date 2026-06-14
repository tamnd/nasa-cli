package nasa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestClient returns a Client pointed at the given httptest server with
// no pacing and a TEST_KEY, so no real network calls are made.
func newTestClient(baseURL string) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.APIKey = "TEST_KEY"
	cfg.Rate = 0
	cfg.Retries = 0
	cfg.Timeout = 5 * time.Second
	return &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		APIKey:    cfg.APIKey,
		cfg:       cfg,
		Rate:      cfg.Rate,
		Retries:   cfg.Retries,
	}
}

func TestAPOD(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/planetary/apod" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("api_key") != "TEST_KEY" {
			t.Errorf("expected api_key=TEST_KEY, got %s", r.URL.Query().Get("api_key"))
		}
		if r.URL.Query().Get("date") != "2024-01-15" {
			t.Errorf("expected date=2024-01-15, got %s", r.URL.Query().Get("date"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"date": "2024-01-15",
			"title": "Horsehead Nebula",
			"explanation": "A dark nebula in Orion.",
			"media_type": "image",
			"url": "https://apod.nasa.gov/apod/image/2401/horsehead.jpg",
			"hdurl": "https://apod.nasa.gov/apod/image/2401/horsehead_hd.jpg",
			"copyright": "Jane Astronomer"
		}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	a, err := c.APOD(context.Background(), "2024-01-15")
	if err != nil {
		t.Fatalf("APOD: %v", err)
	}
	if a.Date != "2024-01-15" {
		t.Errorf("Date = %q, want 2024-01-15", a.Date)
	}
	if a.Title != "Horsehead Nebula" {
		t.Errorf("Title = %q, want Horsehead Nebula", a.Title)
	}
	if a.MediaType != "image" {
		t.Errorf("MediaType = %q, want image", a.MediaType)
	}
	if a.Copyright != "Jane Astronomer" {
		t.Errorf("Copyright = %q, want Jane Astronomer", a.Copyright)
	}
}

func TestAPODRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/planetary/apod" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("start_date") != "2024-01-01" {
			t.Errorf("expected start_date=2024-01-01")
		}
		if r.URL.Query().Get("end_date") != "2024-01-03" {
			t.Errorf("expected end_date=2024-01-03")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"date":"2024-01-01","title":"New Year Nebula","media_type":"image","url":"https://apod.nasa.gov/apod/image/2401/ny.jpg"},
			{"date":"2024-01-02","title":"January Comet","media_type":"image","url":"https://apod.nasa.gov/apod/image/2401/comet.jpg"},
			{"date":"2024-01-03","title":"Galaxy Ring","media_type":"image","url":"https://apod.nasa.gov/apod/image/2401/ring.jpg"}
		]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	items, err := c.APODRange(context.Background(), "2024-01-01", "2024-01-03")
	if err != nil {
		t.Fatalf("APODRange: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Date != "2024-01-01" {
		t.Errorf("items[0].Date = %q, want 2024-01-01", items[0].Date)
	}
	if items[2].Title != "Galaxy Ring" {
		t.Errorf("items[2].Title = %q, want Galaxy Ring", items[2].Title)
	}
}

func TestEPIC(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/EPIC/api/natural" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"identifier": "20240115003748",
				"caption": "This image was taken by NASA EPIC.",
				"date": "2024-01-15 00:37:48",
				"centroid_coordinates": {"lat": 12.3, "lon": -45.6},
				"image": "epic_1b_20240115003748"
			},
			{
				"identifier": "20240115083748",
				"caption": "Second image.",
				"date": "2024-01-15 08:37:48",
				"centroid_coordinates": {"lat": 5.1, "lon": -30.2},
				"image": "epic_1b_20240115083748"
			}
		]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	images, err := c.EPIC(context.Background())
	if err != nil {
		t.Fatalf("EPIC: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}
	img := images[0]
	if img.Identifier != "20240115003748" {
		t.Errorf("Identifier = %q, want 20240115003748", img.Identifier)
	}
	if img.Lat != 12.3 {
		t.Errorf("Lat = %v, want 12.3", img.Lat)
	}
	wantURL := "https://epic.gsfc.nasa.gov/archive/natural/2024/01/15/png/epic_1b_20240115003748.png"
	if img.ImageURL != wantURL {
		t.Errorf("ImageURL = %q, want %q", img.ImageURL, wantURL)
	}
}

func TestRoverPhotos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mars-photos/api/v1/rovers/curiosity/photos" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("sol") != "1000" {
			t.Errorf("expected sol=1000, got %s", r.URL.Query().Get("sol"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"photos": [
				{
					"id": 102693,
					"sol": 1000,
					"camera": {"name": "FHAZ", "full_name": "Front Hazard Avoidance Camera"},
					"img_src": "https://mars.nasa.gov/msl-raw-images/fhaz_001.jpg",
					"earth_date": "2015-05-30",
					"rover": {"name": "Curiosity", "status": "active"}
				},
				{
					"id": 102694,
					"sol": 1000,
					"camera": {"name": "RHAZ", "full_name": "Rear Hazard Avoidance Camera"},
					"img_src": "https://mars.nasa.gov/msl-raw-images/rhaz_001.jpg",
					"earth_date": "2015-05-30",
					"rover": {"name": "Curiosity", "status": "active"}
				}
			]
		}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	photos, err := c.RoverPhotos(context.Background(), "curiosity", 1000, 0)
	if err != nil {
		t.Fatalf("RoverPhotos: %v", err)
	}
	if len(photos) != 2 {
		t.Fatalf("got %d photos, want 2", len(photos))
	}
	p := photos[0]
	if p.ID != 102693 {
		t.Errorf("ID = %d, want 102693", p.ID)
	}
	if p.Sol != 1000 {
		t.Errorf("Sol = %d, want 1000", p.Sol)
	}
	if p.Camera != "FHAZ" {
		t.Errorf("Camera = %q, want FHAZ", p.Camera)
	}
	if p.RoverName != "Curiosity" {
		t.Errorf("RoverName = %q, want Curiosity", p.RoverName)
	}
	if p.EarthDate != "2015-05-30" {
		t.Errorf("EarthDate = %q, want 2015-05-30", p.EarthDate)
	}
}

func TestNEOFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/neo/rest/v1/feed" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("start_date") != "2024-01-01" {
			t.Errorf("expected start_date=2024-01-01")
		}
		if r.URL.Query().Get("end_date") != "2024-01-02" {
			t.Errorf("expected end_date=2024-01-02")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"element_count": 2,
			"near_earth_objects": {
				"2024-01-01": [
					{
						"name": "2022 YO1",
						"is_potentially_hazardous_asteroid": false,
						"estimated_diameter": {
							"kilometers": {
								"estimated_diameter_min": 0.0123,
								"estimated_diameter_max": 0.0275
							}
						}
					}
				],
				"2024-01-02": [
					{
						"name": "2021 AB3",
						"is_potentially_hazardous_asteroid": true,
						"estimated_diameter": {
							"kilometers": {
								"estimated_diameter_min": 0.4500,
								"estimated_diameter_max": 1.0062
							}
						}
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	items, err := c.NEOFeed(context.Background(), "2024-01-01", "2024-01-02")
	if err != nil {
		t.Fatalf("NEOFeed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// Results are sorted by date then name.
	if items[0].Date != "2024-01-01" {
		t.Errorf("items[0].Date = %q, want 2024-01-01", items[0].Date)
	}
	if items[0].Name != "2022 YO1" {
		t.Errorf("items[0].Name = %q, want 2022 YO1", items[0].Name)
	}
	if items[0].IsHazardous {
		t.Error("items[0].IsHazardous should be false")
	}
	if items[1].IsHazardous != true {
		t.Error("items[1].IsHazardous should be true")
	}
	if items[1].DiameterMinKm != 0.4500 {
		t.Errorf("items[1].DiameterMinKm = %v, want 0.45", items[1].DiameterMinKm)
	}
}

func TestEPICImageURL(t *testing.T) {
	cases := []struct {
		date, image, want string
	}{
		{
			"2024-01-15 00:37:48",
			"epic_1b_20240115003748",
			"https://epic.gsfc.nasa.gov/archive/natural/2024/01/15/png/epic_1b_20240115003748.png",
		},
		{
			"2023-12-25 12:00:00",
			"epic_1b_20231225120000",
			"https://epic.gsfc.nasa.gov/archive/natural/2023/12/25/png/epic_1b_20231225120000.png",
		},
	}
	for _, tc := range cases {
		got := epicImageURL(tc.date, tc.image)
		if got != tc.want {
			t.Errorf("epicImageURL(%q, %q) = %q, want %q", tc.date, tc.image, got, tc.want)
		}
	}
}

func TestRoverPhotosWithLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"photos": [
				{"id":1,"sol":500,"camera":{"name":"NAVCAM","full_name":"Navigation Camera"},"img_src":"https://mars.nasa.gov/1.jpg","earth_date":"2014-01-15","rover":{"name":"Curiosity","status":"active"}},
				{"id":2,"sol":500,"camera":{"name":"FHAZ","full_name":"Front Hazard"},"img_src":"https://mars.nasa.gov/2.jpg","earth_date":"2014-01-15","rover":{"name":"Curiosity","status":"active"}},
				{"id":3,"sol":500,"camera":{"name":"MAST","full_name":"Mast Camera"},"img_src":"https://mars.nasa.gov/3.jpg","earth_date":"2014-01-15","rover":{"name":"Curiosity","status":"active"}}
			]
		}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	photos, err := c.RoverPhotos(context.Background(), "curiosity", 500, 2)
	if err != nil {
		t.Fatalf("RoverPhotos: %v", err)
	}
	if len(photos) != 2 {
		t.Errorf("got %d photos with limit 2, want 2", len(photos))
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"date":"2024-01-15","title":"Test","media_type":"image","url":"https://example.com"}`))
	}))
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = srv.URL
	cfg.APIKey = "TEST_KEY"
	cfg.Rate = 0
	cfg.Retries = 5
	cfg.Timeout = 5 * time.Second
	c := &Client{
		HTTP:      &http.Client{Timeout: cfg.Timeout},
		UserAgent: cfg.UserAgent,
		APIKey:    cfg.APIKey,
		cfg:       cfg,
		Rate:      0,
		Retries:   5,
	}

	start := time.Now()
	_, err := c.APOD(context.Background(), "2024-01-15")
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}
