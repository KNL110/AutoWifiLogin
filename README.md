# autowifilogin

Auto-relogin for the IIT Goa PAN-OS captive portal (`firewall.iitgoa.ac.in`).
Checks connectivity via `http://connectivitycheck.gstatic.com/generate_204`;
if the request gets redirected to the portal instead of returning `204`, it
parses the login form and re-submits your credentials automatically.

## How it works

The portal issues a fresh, single-use `token` (in the URL) and `preauthid`
(hidden form field) on every unauthenticated request, so nothing is
hardcoded — each run fetches the current login page, pulls the live hidden
fields out of the HTML, and POSTs your credentials against those.

## Layout

- `cmd/autowifilogin/main.go` — CLI entrypoint: reads credentials from the
  environment, orchestrates a check-then-login-if-needed cycle
- `internal/portal/portal.go` — the `Client` type: `Online()` connectivity
  check, `Login()` (fetch form → submit)
- `internal/portal/html.go` — generic `<input>` field scraper used to read
  the portal's hidden form fields (token/preauthid/etc.)
- `systemd/autowifilogin.service` — oneshot unit that runs the binary
- `systemd/autowifilogin.timer` — triggers the service on boot and hourly

## Setup

Credentials are read from environment variables, never committed to the
repo. Create `~/.config/autowifilogin/env` (and lock it down with
`chmod 600`):

```sh
mkdir -p ~/.config/autowifilogin
cat > ~/.config/autowifilogin/env <<'EOF'
WIFI_USER=your-username
WIFI_PASS=your-password
EOF
chmod 600 ~/.config/autowifilogin/env
```

Build and (re)install after any code change:

```sh
cd ~/Projects/autoWifiLogin
go build -o autowifilogin ./cmd/autowifilogin
cp systemd/*.service systemd/*.timer ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user restart autowifilogin.timer
```

Check it's running / see recent activity:

```sh
systemctl --user status autowifilogin.timer
journalctl --user -u autowifilogin.service -n 50 -f
```

Run it manually once (useful right after your session times out, to watch
it work):

```sh
set -a; source ~/.config/autowifilogin/env; set +a
./autowifilogin
```

## Notes

- The timer only runs while you have an active login session (`Linger=no`).
  If you want it to keep running even when logged out (e.g. on a headless
  box), run: `loginctl enable-linger $USER`.
- If the portal's HTML changes (field names, etc.), `parseInputFields` in
  `internal/portal/html.go` collects every `<input>` name/value pair
  generically, so most small changes won't need code updates — only a
  genuinely new required field would.
