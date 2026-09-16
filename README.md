# AutoWifiLogin

A small Go daemon that automatically re-authenticates against IIT Goa's
PAN-OS-based wifi captive portal (`firewall.iitgoa.ac.in`) whenever a
session times out.

## How it works

The daemon polls `http://connectivitycheck.gstatic.com/generate_204`, the
same endpoint browsers use for captive-portal detection. A `204` response
means the connection is authenticated; any other response (a redirect to
the portal) means the session has expired.

On expiry, it fetches the portal's login page and parses the current
`token` and `preauthid` values from the HTML. These are single-use and
regenerated on every unauthenticated request, so they are read live rather
than hardcoded. It then submits the login form with the configured
credentials, replicating the request a browser would send.

## Protocol reference

Reverse-engineered from a browser's actual login flow (captured via DevTools).
Values below are examples; `token`, `preauthid`, and the session cookie are
single-use and change on every request.

### 1. Connectivity probe

```http
GET http://connectivitycheck.gstatic.com/generate_204
```

A `204` with no redirect means the session is authenticated. Any other
response means the portal intercepted the request.

### 2. Portal redirect

An unauthenticated probe gets redirected to the login page, with a
freshly-issued single-use token embedded in the URL:

```http
GET https://firewall.iitgoa.ac.in:6082/php/uid.php?vsys=1&rule=1&token=<token>&url=<original-url>
Set-Cookie: SESSID=<session-cookie>
```

The response body is the login form's HTML, containing hidden fields —
notably `preauthid`, also single-use and tied to this session.

### 3. Login submission

The form POSTs back to the same URL, with the session cookie from step 2:

```http
POST https://firewall.iitgoa.ac.in:6082/php/uid.php?vsys=1&rule=1&token=<token>&url=<original-url>
Content-Type: application/x-www-form-urlencoded
Cookie: SESSID=<session-cookie>

inputStr=&escapeUser=<username>&preauthid=<preauthid>&user=<username>&passwd=<password>&ok=Login
```

| Field | Meaning |
| --- | --- |
| `inputStr` | Always empty in observed requests |
| `escapeUser` | Username, duplicated |
| `preauthid` | Session-bound value read from the login page's hidden field |
| `user` | Username |
| `passwd` | Password |
| `ok` | Literal `Login`, the submit button's value |

### 4. Confirmation

Repeat step 1 — a `204` confirms the login succeeded.

`internal/portal/portal.go` implements steps 1–4 (`Online`, `fetchLoginForm`,
`submitLogin`); `internal/portal/html.go` implements the generic hidden-field
extraction used in step 2.

## Project layout

| Path | Description |
| --- | --- |
| `cmd/autowifilogin/main.go` | Entrypoint: reads credentials from the environment and runs one check/login cycle |
| `internal/portal/portal.go` | `Client` type implementing `Online()` and `Login()` |
| `internal/portal/html.go` | Generic `<input>` field parser used to extract the portal's hidden form fields |
| `systemd/autowifilogin.service` | Oneshot unit that runs the binary, retrying up to 5 times on failure |
| `systemd/autowifilogin.timer` | Triggers the service on boot and every 30 minutes thereafter |

## Configuration

Credentials are supplied via environment variables (`WIFI_USER`,
`WIFI_PASS`), never hardcoded or committed. When run via the provided
systemd unit, they are loaded from an `EnvironmentFile`:

```sh
mkdir -p ~/.config/autowifilogin
cat > ~/.config/autowifilogin/env <<'EOF'
WIFI_USER=your-username
WIFI_PASS=your-password
EOF
chmod 600 ~/.config/autowifilogin/env
```

## Build

```sh
go build -o autowifilogin ./cmd/autowifilogin
```

## Installation (systemd user timer)

```sh
cp systemd/*.service systemd/*.timer ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now autowifilogin.timer
```

Status and logs:

```sh
systemctl --user status autowifilogin.timer
journalctl --user -u autowifilogin.service -n 50 -f
```

The timer only runs while an active login session exists (`Linger=no` by
default). To keep it running while logged out, enable lingering:

```sh
loginctl enable-linger $USER
```

## Manual run

```sh
WIFI_USER=... WIFI_PASS=... ./autowifilogin
```

## Notes

- `parseInputFields` collects every `<input>` name/value pair generically
  rather than targeting specific field names, so minor portal page changes
  typically don't require code changes.
- TLS certificate verification is disabled for the portal host, which
  serves its login page from an internal CA.

## License

MIT — see [LICENSE](LICENSE).
