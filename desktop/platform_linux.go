//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/godbus/dbus/v5"
)

// Linux needs no permanent system change: systemd-logind "inhibitor locks" are
// file descriptors, released automatically when Pisolo exits or crashes.

var (
	logindFD      = -1
	screenCookie  uint32
	screenIface   string // "org.freedesktop.ScreenSaver" or "org.gnome.SessionManager"
	sessionBus    *dbus.Conn
	inhibitorWhat string
)

func applyMode(m Mode, _ *State) error {
	releaseAll()
	switch m {
	case Awake:
		if err := logindInhibit("idle:sleep"); err != nil {
			return err
		}
		inhibitScreen(true)
	case LidClosed:
		if err := logindInhibit("idle:sleep:handle-lid-switch"); err != nil {
			return err
		}
		inhibitScreen(false)
	}
	return nil
}

func recoverAfterCrash(*State) {} // nothing persistent to undo

func logindInhibit(what string) error {
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("system bus: %w", err)
	}
	var fd dbus.UnixFD
	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0, what, "Pisolo", "Pisolo keeps the computer awake", "block").
		Store(&fd)
	if err != nil {
		return fmt.Errorf("logind inhibit %s: %w", what, err)
	}
	logindFD, inhibitorWhat = int(fd), what
	return nil
}

// inhibitScreen asks the desktop not to blank the screen (display=true) and/or
// not to suspend on idle (GNOME). Best effort: not every desktop offers it.
func inhibitScreen(display bool) {
	if sessionBus == nil {
		c, err := dbus.SessionBus()
		if err != nil {
			return
		}
		sessionBus = c
	}
	if display {
		err := sessionBus.Object("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver").
			Call("org.freedesktop.ScreenSaver.Inhibit", 0, "Pisolo", "Always on").Store(&screenCookie)
		if err == nil {
			screenIface = "org.freedesktop.ScreenSaver"
			return
		}
	}
	// GNOME: flags 4 = suspend, 8 = idle (screen blank)
	flags := uint32(4)
	if display {
		flags |= 8
	}
	err := sessionBus.Object("org.gnome.SessionManager", "/org/gnome/SessionManager").
		Call("org.gnome.SessionManager.Inhibit", 0, "Pisolo", uint32(0), "Pisolo keeps the computer awake", flags).
		Store(&screenCookie)
	if err == nil {
		screenIface = "org.gnome.SessionManager"
	}
}

func releaseAll() {
	if logindFD >= 0 {
		_ = syscall.Close(logindFD)
		logindFD, inhibitorWhat = -1, ""
	}
	if screenIface != "" && sessionBus != nil {
		switch screenIface {
		case "org.freedesktop.ScreenSaver":
			sessionBus.Object(screenIface, "/org/freedesktop/ScreenSaver").Call(screenIface+".UnInhibit", 0, screenCookie)
		case "org.gnome.SessionManager":
			sessionBus.Object(screenIface, "/org/gnome/SessionManager").Call(screenIface+".Uninhibit", 0, screenCookie)
		}
		screenIface = ""
	}
}

// ---------- Battery ----------

func batteryLow() bool {
	return batteryLowIn("/sys/class/power_supply")
}

func batteryLowIn(root string) bool {
	dirs, _ := filepath.Glob(filepath.Join(root, "*"))
	onAC, capacity, hasBattery := false, 100, false
	for _, d := range dirs {
		typ := readTrim(filepath.Join(d, "type"))
		switch typ {
		case "Mains", "USB":
			if readTrim(filepath.Join(d, "online")) == "1" {
				onAC = true
			}
		case "Battery":
			if c, err := strconv.Atoi(readTrim(filepath.Join(d, "capacity"))); err == nil {
				hasBattery = true
				capacity = min(capacity, c)
			}
		}
	}
	return hasBattery && !onAC && capacity < 10
}

func readTrim(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

// ---------- Autostart (XDG) ----------

func autostartFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "autostart", "pisolo.desktop")
}

func autostartEnabled() bool {
	_, err := os.Stat(autostartFile())
	return err == nil
}

func setAutostart(on bool) error {
	if !on {
		return os.Remove(autostartFile())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(autostartFile()), 0o755)
	entry := "[Desktop Entry]\nType=Application\nName=Pisolo\nComment=Keeps the computer awake\nExec=\"" + exe + "\"\nX-GNOME-Autostart-enabled=true\n"
	return os.WriteFile(autostartFile(), []byte(entry), 0o644)
}

// ---------- UI helpers (zenity / kdialog when available) ----------

func openURL(url string) { _ = exec.Command("xdg-open", url).Start() }

func showInfo(title, text string) {
	switch {
	case has("zenity"):
		_ = exec.Command("zenity", "--info", "--title", title, "--text", text, "--no-wrap").Start()
	case has("kdialog"):
		_ = exec.Command("kdialog", "--title", title, "--msgbox", text).Start()
	case has("notify-send"):
		_ = exec.Command("notify-send", title, text).Start()
	}
}

func askBeer(title, text string) int {
	beer, later, never := t("Buy me a beer", "Offrimi una birra"), t("Later", "Più tardi"), t("No, thanks", "No, grazie")
	switch {
	case has("zenity"):
		cmd := exec.Command("zenity", "--question", "--title", title, "--text", text,
			"--ok-label", beer, "--cancel-label", later, "--extra-button", never)
		out, err := cmd.Output()
		if err == nil {
			return 0
		}
		if strings.TrimSpace(string(out)) == never {
			return 2
		}
		return 1
	case has("kdialog"):
		err := exec.Command("kdialog", "--title", title, "--yesnocancel", text,
			"--yes-label", beer, "--no-label", later, "--cancel-label", never).Run()
		if err == nil {
			return 0
		}
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return 1
		}
		return 2
	}
	return -1
}

func has(cmd string) bool { _, err := exec.LookPath(cmd); return err == nil }

// ---------- Misc ----------

var lockFile *os.File

func acquireSingleInstance() bool {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("pisolo-%d.lock", os.Getuid())), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return false
	}
	lockFile = f // keep it open for the lifetime of the process
	return true
}

func isItalian() bool {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := os.Getenv(v); l != "" {
			return strings.HasPrefix(l, "it")
		}
	}
	return false
}
