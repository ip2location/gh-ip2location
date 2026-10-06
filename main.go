package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

const appName = "gh-ip2location"

// Version can be set at build time with -ldflags "-X main.version=v1.0.0".
var version = "v1.0.1"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", appName, err)
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", appName)
		return 2
	}

	switch {
	case opts.showHelp:
		printHelp(stdout)
		return 0

	case opts.showVersion:
		fmt.Fprintf(stdout, "%s %s\n", appName, resolveVersion())
		return 0

	case opts.setKey != "":
		cfg := loadConfig()
		cfg.APIKey = opts.setKey
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(stderr, "%s: cannot save API key: %v\n", appName, err)
			return 1
		}
		if p, err := configPath(); err == nil {
			fmt.Fprintf(stderr, "API key saved to %s\n", p)
		}
		return 0

	case opts.unsetKey:
		cfg := loadConfig()
		cfg.APIKey = ""
		if err := saveConfig(cfg); err != nil {
			fmt.Fprintf(stderr, "%s: cannot update config: %v\n", appName, err)
			return 1
		}
		fmt.Fprintln(stderr, "API key removed.")
		return 0
	}

	// No IP means the caller's own address
	ips := opts.ips
	if len(ips) == 0 {
		ips = []string{""}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(len(ips))*30*time.Second)
	defer cancel()

	client := NewClient(opts.apiKey(), opts.lang)

	var results []Response
	var notices []string
	var failed int
	seenNote := map[string]bool{}

	for _, ip := range ips {
		resp, err := client.Lookup(ctx, ip)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %s: %v\n", appName, describeIP(ip), err)

			var apiErr *APIError
			if errors.As(err, &apiErr) {
				if hint := apiErr.Hint(); hint != "" {
					fmt.Fprintf(stderr, "%s: hint: %s\n", appName, hint)
				}
			}

			failed++
			continue
		}

		// Keep the keyless notice off stdout
		if msg, ok := resp["message"].(string); ok && msg != "" && !seenNote[msg] {
			seenNote[msg] = true
			notices = append(notices, msg)
		}

		results = append(results, resp)
	}

	for _, n := range notices {
		fmt.Fprintf(stderr, "Note: %s\n", n)
	}

	if len(results) > 0 {
		if err := render(stdout, results, opts.format, opts.fields); err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", appName, err)
			return 1
		}
	}

	if failed > 0 {
		return 1
	}
	return 0
}

func describeIP(ip string) string {
	if ip == "" {
		return "caller IP"
	}
	return ip
}

// Resolve version from VCS info
func resolveVersion() string {
	if version != "" {
		return version
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "dev+" + s.Value[:7]
			}
		}
	}

	return "dev"
}

func userAgent() string {
	return appName + "/" + resolveVersion()
}

type options struct {
	key         string
	format      string
	fields      []string
	lang        string
	setKey      string
	unsetKey    bool
	showVersion bool
	showHelp    bool
	ips         []string
}

// Get API key from flag, environment or saved configuration
func (o *options) apiKey() string {
	if o.key != "" {
		return o.key
	}
	if v := os.Getenv("IP2LOCATION_API_KEY"); v != "" {
		return v
	}
	return loadConfig().APIKey
}

// Parse arguments
func parseArgs(args []string) (*options, error) {
	o := &options{format: "table"}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !strings.HasPrefix(arg, "-") || arg == "-" {
			if arg != "" {
				o.ips = append(o.ips, arg)
			}
			continue
		}

		name, inline, hasInline := strings.Cut(arg, "=")
		value := func() (string, error) {
			if hasInline {
				return inline, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag %s needs a value", name)
			}
			i++
			return args[i], nil
		}

		switch name {
		case "-h", "--help":
			o.showHelp = true

		case "-v", "--version":
			o.showVersion = true

		case "-k", "--key":
			v, err := value()
			if err != nil {
				return nil, err
			}
			o.key = v

		case "-f", "--format":
			v, err := value()
			if err != nil {
				return nil, err
			}
			v = strings.ToLower(v)
			if v != "table" && v != "json" && v != "csv" {
				return nil, fmt.Errorf("unknown format %q (want table, json or csv)", v)
			}
			o.format = v

		case "--json":
			o.format = "json"

		case "--csv":
			o.format = "csv"

		case "--fields":
			v, err := value()
			if err != nil {
				return nil, err
			}
			o.fields = splitList(v)

		case "--lang":
			v, err := value()
			if err != nil {
				return nil, err
			}
			o.lang = v

		case "--set-key":
			v, err := value()
			if err != nil {
				return nil, err
			}
			o.setKey = v

		case "--unset-key":
			o.unsetKey = true

		default:
			return nil, fmt.Errorf("unknown flag %s", name)
		}
	}

	return o, nil
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, `Look up IP geolocation details with the IP2Location.io API.

Usage:
  gh ip2location [<ip>...] [flags]

With no IP, the caller's own address is looked up. Several IPs may be given;
JSON then returns an array and CSV one row per IP.

Flags:
  -k, --key <key>       IP2Location.io API key. Falls back to the
                        IP2LOCATION_API_KEY environment variable, then to the
                        saved configuration. Without a key the keyless endpoint is
                        used, which allows 1,000 queries per day.
  -f, --format <fmt>    Output format: table, json or csv (default "table").
      --json            Shorthand for --format json.
      --csv             Shorthand for --format csv.
      --fields <list>   Comma separated fields to return, e.g.
                        "ip,country_name,latitude". Nest with dots, e.g.
                        "country.currency.code".
      --lang <code>     Translation language for continent, country, region
                        and city names (Plus and Security plans).
      --set-key <key>   Save the API key to the config file and exit.
      --unset-key       Remove the saved API key and exit.
  -v, --version         Show the extension version.
  -h, --help            Show this help.

Examples:
  gh ip2location 8.8.8.8
  gh ip2location 8.8.8.8 --json
  gh ip2location 8.8.8.8 1.1.1.1 --csv
  gh ip2location 2001:4860:4860::8888
  gh ip2location --fields ip,country_name,city_name 8.8.8.8
  gh ip2location --set-key $IP2LOCATION_API_KEY

Exit codes:
  0  success
  1  lookup or API error
  2  bad usage

Documentation: https://github.com/ip2location/gh-ip2location
`)
}
