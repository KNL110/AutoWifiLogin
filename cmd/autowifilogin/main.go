// Command autowifilogin checks internet connectivity through the campus
// captive portal and logs back in automatically if the session timed out.
package main

import (
	"log"
	"os"
	"time"

	"autowifilogin/internal/portal"
)

func main() {
	user := os.Getenv("WIFI_USER")
	pass := os.Getenv("WIFI_PASS")
	if user == "" || pass == "" {
		log.Fatal("WIFI_USER and WIFI_PASS must be set in the environment")
	}

	c := portal.New()

	if c.Online() {
		log.Println("online, nothing to do")
		return
	}

	log.Println("not online, attempting captive portal login")
	if err := c.Login(user, pass); err != nil {
		log.Fatal(err)
	}

	// Give the firewall a moment to register the session, then confirm.
	time.Sleep(2 * time.Second)
	if c.Online() {
		log.Println("login succeeded, online again")
		return
	}
	log.Fatal("login submitted but still not online")
}
