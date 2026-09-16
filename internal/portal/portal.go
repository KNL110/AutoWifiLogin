package portal

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// CheckURL is the well-known connectivity-check endpoint the portal
// intercepts when the session is unauthenticated.
const CheckURL = "http://connectivitycheck.gstatic.com/generate_204"

// The login page's preauthid hidden <input> is always rendered empty
// (value=""); the real value is written into it by inline JavaScript
// (`thisForm.preauthid.value = "..."`) only when a real browser submits
// the form. Since this program doesn't execute JavaScript, the real value
// has to be read directly out of that script instead of the <input> tag.
var preauthidJSRe = regexp.MustCompile(`preauthid\.value\s*=\s*"([^"]*)"`)

// Client checks connectivity and performs captive-portal logins.
type Client struct {
	http *http.Client
}

// New returns a Client ready to use. The firewall serves its login page
// over TLS with an internal CA, so certificate verification is disabled.
func New() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http: &http.Client{
			Timeout: 10 * time.Second,
			Jar:     jar,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // firewall uses an internal CA
			},
		},
	}
}

// Online reports whether CheckURL returns the expected 204 with no
// redirect. Any redirect means the captive portal intercepted the request.
func (c *Client) Online() bool {
	resp, err := c.http.Get(CheckURL)
	if err != nil {
		log.Printf("connectivity check: request failed: %v", err)
		return false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		log.Printf("connectivity check: got %d from %s (want 204): %s",
			resp.StatusCode, resp.Request.URL, snippet(body))
	}
	return resp.StatusCode == http.StatusNoContent
}

// Login fetches the current login form, fills in the given credentials,
// and submits it. The portal issues a fresh single-use token and preauthid
// on every unauthenticated request, so both are read from the live page
// rather than hardcoded.
func (c *Client) Login(user, pass string) error {
	loginURL, fields, err := c.fetchLoginForm()
	if err != nil {
		return fmt.Errorf("fetch login form: %w", err)
	}

	fields["user"] = user
	// Mirrors the login page's own JS: thisForm.escapeUser.value =
	// thisForm.user.value.replace(/\\/g, "\\\\")
	fields["escapeUser"] = strings.ReplaceAll(user, `\`, `\\`)
	fields["passwd"] = pass
	fields["ok"] = "Login"
	if _, ok := fields["inputStr"]; !ok {
		fields["inputStr"] = ""
	}

	if err := c.submitLogin(loginURL, fields); err != nil {
		return fmt.Errorf("submit login: %w", err)
	}
	return nil
}

// fetchLoginForm follows the captive-portal redirect chain from CheckURL,
// then parses the returned HTML for the login form's hidden input fields.
func (c *Client) fetchLoginForm() (*url.URL, map[string]string, error) {
	resp, err := c.http.Get(CheckURL)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil, fmt.Errorf("already online, no login form to fetch")
	}

	log.Printf("login page: got %d from %s", resp.StatusCode, resp.Request.URL)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}

	fields, err := parseInputFields(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	if _, ok := fields["preauthid"]; !ok {
		return nil, nil, fmt.Errorf("preauthid field not found in login page (portal page may have changed)")
	}

	// The <input> tag's preauthid value is always empty — the real value
	// only gets set by JavaScript on submit. Pull it from that script.
	m := preauthidJSRe.FindSubmatch(body)
	if m == nil || len(m[1]) == 0 {
		return nil, nil, fmt.Errorf("preauthid value not found in login page script (portal page may have changed)")
	}
	fields["preauthid"] = string(m[1])

	log.Printf("login page: found %d form fields", len(fields))

	return resp.Request.URL, fields, nil
}

// submitLogin POSTs the login form fields to loginURL, mirroring the
// browser's captive-portal login submission.
func (c *Client) submitLogin(loginURL *url.URL, fields map[string]string) error {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}

	log.Printf("login POST: submitting fields: %s", redactedFields(fields))

	req, err := http.NewRequest(http.MethodPost, loginURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", loginURL.String())
	req.Header.Set("User-Agent", "autowifilogin/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	log.Printf("login POST: got %d: %s", resp.StatusCode, snippet(body))

	// The response shares the same HTML shell on every page (success or
	// failure), so a short snippet can't tell them apart. Re-parse it for a
	// preauthid field instead: its presence means the portal re-rendered
	// the login form, i.e. the submission was rejected.
	if respFields, err := parseInputFields(bytes.NewReader(body)); err == nil {
		if _, stillLoginForm := respFields["preauthid"]; stillLoginForm {
			log.Printf("login POST: response still contains the login form (preauthid present) — submission was likely rejected")
		} else {
			log.Printf("login POST: response no longer contains the login form")
		}
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("login POST returned status %d", resp.StatusCode)
	}
	return nil
}

// redactedFields renders form fields as key=value pairs for diagnostics,
// with the password masked and keys sorted for stable output.
func redactedFields(fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := fields[k]
		if k == "passwd" {
			v = "<redacted>"
		}
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, " ")
}

// snippet returns a short, single-line, log-friendly preview of a response
// body for diagnostics.
func snippet(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	const max = 600
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
