// Command autowifilogin checks internet connectivity through the campus
// captive portal and logs back in automatically if the session timed out.
package main

import (
	"log"
	"os"
	"os/exec"
	"time"

	"autowifilogin/internal/portal"
)

func main() {

	client := portal.New()

	if client.Online() {
		log.Println("online, nothing to do")
		return
	}

	user := os.Getenv("WIFI_USER")
	pass := os.Getenv("WIFI_PASS")
	if user == "" || pass == "" {
		const msg = "WIFI_USER and WIFI_PASS must be set in the environment"
		notify("Wifi captive portal", "Login failed: "+msg)
		log.Fatal(msg)
	}

	log.Println("not online, attempting captive portal login")
	notify("Wifi captive portal", "Session timed out — logging back in...")
	if err := client.Login(user, pass); err != nil {
		notify("Wifi captive portal", "Login failed: "+err.Error())
		log.Fatal(err)
	}

	// Give the firewall a moment to register the session, then confirm.
	time.Sleep(2 * time.Second)
	if client.Online() {
		log.Println("login succeeded, online again")
		notify("Wifi captive portal", "Logged in successfully")
		return
	}
	const msg = "login submitted but still not online"
	notify("Wifi captive portal", "Login failed: "+msg)
	log.Fatal(msg)
}

// notify shows a desktop notification via notify-send. Best-effort: a
// missing binary or no notification daemon shouldn't fail the login run.
func notify(title, body string) {
	if err := exec.Command("notify-send", title, body).Run(); err != nil {
		log.Printf("notify-send failed: %v", err)
	}
}
