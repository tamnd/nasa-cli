package nasa

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes nasa as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/nasa-cli/nasa"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// nasa:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone nasa binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the nasa driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "nasa",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "nasa",
			Short:  "A command line for NASA APIs.",
			Long: `A command line for NASA Open APIs.

nasa reads public NASA data — astronomy pictures, Earth imagery, Mars rover
photos, and near-Earth objects — over plain HTTPS, shapes it into clean
records, and prints output that pipes into the rest of your tools.

A free API key (1000 req/hr) is available at https://api.nasa.gov.
The default DEMO_KEY works at 30 req/hr.`,
			Site: "api.nasa.gov",
			Repo: "https://github.com/tamnd/nasa-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	// apod: Astronomy Picture of the Day for a single date.
	kit.Handle(app, kit.OpMeta{
		Name:    "apod",
		Group:   "read",
		Single:  true,
		Summary: "Fetch Astronomy Picture of the Day",
	}, getAPOD)

	// apod-range: a range of APODs by date window.
	kit.Handle(app, kit.OpMeta{
		Name:    "apod-range",
		Group:   "read",
		List:    true,
		Summary: "Fetch a range of Astronomy Pictures of the Day",
	}, getAPODRange)

	// epic: latest EPIC Earth images from DSCOVR satellite.
	kit.Handle(app, kit.OpMeta{
		Name:    "epic",
		Group:   "read",
		List:    true,
		Summary: "Fetch latest EPIC Earth images",
	}, getEPIC)

	// rover: Mars rover photos for a given sol.
	kit.Handle(app, kit.OpMeta{
		Name:    "rover",
		Group:   "read",
		List:    true,
		Summary: "Fetch Mars rover photos",
	}, getRoverPhotos)

	// neo: near-Earth objects for a date range.
	kit.Handle(app, kit.OpMeta{
		Name:    "neo",
		Group:   "read",
		List:    true,
		Summary: "Fetch near-Earth objects summary",
	}, getNEOFeed)
}

// newClient builds the Client from kit.Config, bridging APIKey from
// the host-resolved config so --key propagates correctly.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	dcfg := DefaultConfig()
	if cfg.UserAgent != "" {
		dcfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		dcfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		dcfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		dcfg.Timeout = cfg.Timeout
	}
	c := &Client{
		HTTP:      &http.Client{Timeout: dcfg.Timeout},
		UserAgent: dcfg.UserAgent,
		APIKey:    dcfg.APIKey,
		cfg:       dcfg,
		Rate:      dcfg.Rate,
		Retries:   dcfg.Retries,
	}
	return c, nil
}

// --- input structs ---

type apodInput struct {
	Date   string  `kit:"flag" help:"date YYYY-MM-DD (default: today)"`
	Key    string  `kit:"flag" name:"key" help:"NASA API key (default: DEMO_KEY)"`
	Client *Client `kit:"inject"`
}

type apodRangeInput struct {
	Start  string  `kit:"flag" name:"start" help:"start date YYYY-MM-DD"`
	End    string  `kit:"flag" name:"end" help:"end date YYYY-MM-DD"`
	Key    string  `kit:"flag" name:"key" help:"NASA API key (default: DEMO_KEY)"`
	Client *Client `kit:"inject"`
}

type epicInput struct {
	Key    string  `kit:"flag" name:"key" help:"NASA API key (default: DEMO_KEY)"`
	Client *Client `kit:"inject"`
}

type roverInput struct {
	Name   string  `kit:"flag" name:"name" help:"rover name: curiosity, opportunity, spirit, perseverance"`
	Sol    int     `kit:"flag" name:"sol" help:"Martian solar day"`
	Limit  int     `kit:"flag,inherit" help:"max photos to return"`
	Key    string  `kit:"flag" name:"key" help:"NASA API key (default: DEMO_KEY)"`
	Client *Client `kit:"inject"`
}

type neoInput struct {
	Start  string  `kit:"flag" name:"start" help:"start date YYYY-MM-DD"`
	End    string  `kit:"flag" name:"end" help:"end date YYYY-MM-DD"`
	Key    string  `kit:"flag" name:"key" help:"NASA API key (default: DEMO_KEY)"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getAPOD(ctx context.Context, in apodInput, emit func(*APOD) error) error {
	applyKey(in.Client, in.Key)
	a, err := in.Client.APOD(ctx, in.Date)
	if err != nil {
		return mapErr(err)
	}
	return emit(a)
}

func getAPODRange(ctx context.Context, in apodRangeInput, emit func(*APOD) error) error {
	if in.Start == "" || in.End == "" {
		return errs.Usage("--start and --end are required")
	}
	applyKey(in.Client, in.Key)
	items, err := in.Client.APODRange(ctx, in.Start, in.End)
	if err != nil {
		return mapErr(err)
	}
	for i := range items {
		if err := emit(&items[i]); err != nil {
			return err
		}
	}
	return nil
}

func getEPIC(ctx context.Context, in epicInput, emit func(*EPICImage) error) error {
	applyKey(in.Client, in.Key)
	images, err := in.Client.EPIC(ctx)
	if err != nil {
		return mapErr(err)
	}
	for i := range images {
		if err := emit(&images[i]); err != nil {
			return err
		}
	}
	return nil
}

func getRoverPhotos(ctx context.Context, in roverInput, emit func(*RoverPhoto) error) error {
	rover := in.Name
	if rover == "" {
		rover = "curiosity"
	}
	sol := in.Sol
	if sol == 0 {
		sol = 1000
	}
	applyKey(in.Client, in.Key)
	photos, err := in.Client.RoverPhotos(ctx, rover, sol, in.Limit)
	if err != nil {
		return mapErr(fmt.Errorf("rover %s sol %d: %w", rover, sol, err))
	}
	for i := range photos {
		if err := emit(&photos[i]); err != nil {
			return err
		}
	}
	return nil
}

func getNEOFeed(ctx context.Context, in neoInput, emit func(*NEOSummary) error) error {
	if in.Start == "" || in.End == "" {
		return errs.Usage("--start and --end are required")
	}
	applyKey(in.Client, in.Key)
	items, err := in.Client.NEOFeed(ctx, in.Start, in.End)
	if err != nil {
		return mapErr(err)
	}
	for i := range items {
		if err := emit(&items[i]); err != nil {
			return err
		}
	}
	return nil
}

// applyKey updates the client's API key from the --key flag if provided.
func applyKey(c *Client, key string) {
	if key != "" {
		c.APIKey = key
		c.cfg.APIKey = key
	}
}

// --- Resolver (URI driver) ---

// Classify turns any accepted input — a bare date or a full api.nasa.gov URL —
// into the canonical (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	id = nasaPath(input)
	if id == "" {
		return "", "", errs.Usage("unrecognized nasa reference: %q", input)
	}
	return "apod", id, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "apod":
		return "https://api.nasa.gov/planetary/apod?date=" + id, nil
	default:
		return "", errs.Usage("nasa has no resource type %q", uriType)
	}
}

// --- helpers ---

func nasaPath(input string) string {
	input = strings.TrimSpace(input)
	if u, err := url.Parse(input); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return strings.Trim(u.Path, "/")
	}
	return strings.Trim(input, "/")
}

func mapErr(err error) error {
	return err
}
