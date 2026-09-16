package portal

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// CheckURL is the well-known connectivity-check endpoint the portal
// intercepts when the session is unauthenticated.
const CheckURL = "http://connectivitycheck.gstatic.com/generate_204"

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
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
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
	fields["escapeUser"] = user
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

	fields, err := parseInputFields(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if _, ok := fields["preauthid"]; !ok {
		return nil, nil, fmt.Errorf("preauthid field not found in login page (portal page may have changed)")
	}

	return resp.Request.URL, fields, nil
}

// submitLogin POSTs the login form fields to loginURL, mirroring the
// browser's captive-portal login submission.
func (c *Client) submitLogin(loginURL *url.URL, fields map[string]string) error {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}

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
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("login POST returned status %d", resp.StatusCode)
	}
	return nil
}
