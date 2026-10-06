# gh-ip2location

A [GitHub CLI](https://cli.github.com) extension that looks up IP geolocation data from [IP2Location.io](https://www.ip2location.io) without leaving the terminal.

```bash
$ gh ip2location 8.8.8.8
IP Address    8.8.8.8
Country Code  US
Country       United States of America
Region        California
City          Mountain View
Latitude      37.38605
Longitude     -122.08385
ZIP Code      94043
Time Zone     -07:00
ASN           15169
AS            Google LLC
Is Proxy      false
```

Works without an API key. If you need higher quotas or the extra fields that come with a paid plan, sign up a free account from [https://www.ip2location.io](https://www.ip2location.io/?utm_source=gh).



## Installation

```bash
gh extension install ip2location/gh-ip2location
```

Update later with:

```bash
gh extension upgrade ip2location
```



## Usage

```bash
gh ip2location [<ip>...] [flags]
```

Give it no IP and it reports the caller's own address. Give it several and it looks each one up in turn.

```bash
gh ip2location                          # your own IP
gh ip2location 8.8.8.8                  # IPv4
gh ip2location 2001:4860:4860::8888     # IPv6
gh ip2location 8.8.8.8 1.1.1.1 --csv    # several at once
```



### API keys

Without a key the extension uses the keyless endpoint, which allows 1,000 queries per day. A key raises that limit and unlocks the fields included in your plan.

Resolution order, highest first:

1. `--key <key>`
2. the `IP2LOCATION_API_KEY` environment variable
3. the saved config file

Save a key once so you don't have to pass it every time:

```bash
gh ip2location --set-key $IP2LOCATION_API_KEY
gh ip2location --unset-key
```

The configuration file lives at `gh-ip2location/config.json` inside your user config directory (`%AppData%` on Windows, `~/.config` elsewhere). The environment variable is usually the better choice on shared or CI machines.



### Output formats

| Flag | Format |
| --- | --- |
| `--format table` | Aligned, human readable (Default) |
| `--json` | Raw API response, indented |
| `--csv` | Header row plus one row per IP |

`--format json` and `--format csv` are equivalent to the shorthands.

JSON returns a single object for one lookup and an array when several IPs are given, so `jq` pipelines stay simple:

```
gh ip2location 8.8.8.8 --json --fields ip,country_code | jq -r .country_code
```

CSV flattens nested objects into dotted columns (`country.currency.code`), joins arrays with `; `, and unions the columns across all requested IPs:

```bash
$ gh ip2location 8.8.8.8 1.1.1.1 --csv
ip,country_code,country_name,region_name,city_name,latitude,longitude,zip_code,time_zone,asn,as,is_proxy
8.8.8.8,US,United States of America,California,Mountain View,37.38605,-122.08385,94043,-07:00,15169,Google LLC,false
1.1.1.1,AU,Australia,Queensland,Brisbane,-27.46754,153.02809,4000,+10:00,13335,CloudFlare Inc,false
```



### Limiting fields

`--fields` trims the response to the fields you name. Nest with dots.

```bash
gh ip2location 8.8.8.8 --fields ip,country_name,city_name
gh ip2location 8.8.8.8 --csv --fields ip,country.currency.code
```

In table output the friendly labels on the left are the top-level field names, and anything nested is shown under its dotted path — so a value you see as `country.currency.code` can be pasted straight into `--fields`.



### Translations

`--lang` requests translated continent, country, region and city names. This requires a **Plus** or **Security** plan.

```bash
gh ip2location 8.8.8.8 --lang ko
```



### All flags

```
-k, --key <key>       IP2Location.io API key
-f, --format <fmt>    table, json or csv (default "table")
    --json            Shorthand for --format json
    --csv             Shorthand for --format csv
    --fields <list>   Comma separated fields, nested with dots
    --lang <code>     Translation language
    --set-key <key>   Save the API key and exit
    --unset-key       Remove the saved API key and exit
-v, --version         Show the version
-h, --help            Show help
```



## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Lookup failed, or the API returned an error |
| 2 | Bad usage |

Errors and the keyless quota notice go to **stderr**, so stdout stays clean for piping:

```
$ gh ip2location 8.8.8.8 --json 2>/dev/null | jq -r .city_name
Mountain View
```

A failed lookup in a batch is reported on stderr and sets the exit code to 1, but the IPs that succeeded are still printed.



## Fields

The keyless endpoint returns `ip`, `country_code`, `country_name`, `region_name`, `city_name`, `latitude`, `longitude`, `zip_code`, `time_zone`, `asn`, `as` and `is_proxy`.

A key also unlocks `isp`, `domain`, `net_speed`, `idd_code`, `area_code`, `weather_station_code`, `weather_station_name`, `mcc`, `mnc`, `mobile_brand`, `elevation`, `usage_type`, `address_type`, `category`, `district`, `as_domain`, `as_cidr`, `as_usage_type`, and the nested `continent`, `country`, `region`, `city`, `time_zone_info` and `geotargeting` objects. Security plans add `fraud_score` and `proxy`.

The full reference is at <https://www.ip2location.io/ip2location-documentation>.



## Building from source

Requires Go 1.23 or newer.

```bash
git clone https://github.com/ip2location/gh-ip2location
cd gh-ip2location
go test ./...
go build
gh extension install .
```

