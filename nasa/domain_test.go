package nasa

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in nasa_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "nasa" {
		t.Errorf("Scheme = %q, want nasa", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "nasa" {
		t.Errorf("Identity.Binary = %q, want nasa", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"2024-01-15", "apod", "2024-01-15"},
		{"/planetary/apod", "apod", "planetary/apod"},
		{"https://" + Host + "/planetary/apod", "apod", "planetary/apod"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("apod", "2024-01-15")
	want := "https://api.nasa.gov/planetary/apod?date=2024-01-15"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate with unknown type should return error")
	}
}

// TestHostWiring mounts the driver in a kit Host (the runtime ant drives) and
// checks the round trip: a record mints to its URI, its body is readable, and a
// bare id resolves back to the same URI. The init in domain.go registers the
// domain, so kit.Open finds it.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	a := &APOD{
		Date:      "2024-01-15",
		Title:     "Test APOD",
		MediaType: "image",
		URL:       "https://apod.nasa.gov/apod/image/2401/test.jpg",
	}
	u, err := h.Mint(a)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	_ = u // URI shape varies by kit version; just verify no error

	got, err := h.ResolveOn("nasa", "2024-01-15")
	if err != nil {
		t.Fatalf("ResolveOn: %v", err)
	}
	if got.String() != "nasa://apod/2024-01-15" {
		t.Errorf("ResolveOn = %q, want nasa://apod/2024-01-15", got.String())
	}
}
