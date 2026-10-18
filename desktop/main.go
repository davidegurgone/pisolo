// Pisolo for Windows and Linux — keeps the computer awake, even with the lid closed.
// License: MIT (see LICENSE). Beerware in spirit.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
)

const (
	version   = "1.1.0"
	donateURL = "https://www.paypal.me/davidegurgone"
)

type Mode int

const (
	Off       Mode = 0
	Awake     Mode = 1 // screen and computer stay on
	LidClosed Mode = 2 // computer keeps running, even with the lid closed; the screen may sleep
)

// Platform-specific code lives in platform_windows.go and platform_linux.go.
// Each file implements:
//
//	applyMode(m Mode, s *State) error   // switch to m (Off releases everything)
//	recoverAfterCrash(s *State)         // undo system changes left by a previous run
//	batteryLow() bool                   // on battery and below 10%
//	autostartEnabled() bool
//	setAutostart(on bool) error
//	openURL(url string)
//	showInfo(title, text string)
//	askBeer(title, text string) int     // 0 beer, 1 later, 2 never, -1 no dialog available
//	acquireSingleInstance() bool
//	isItalian() bool

// State is saved to disk so the last mode survives restarts and a crash can be undone.
type State struct {
	Mode        Mode  `json:"mode"`
	FirstLaunch int64 `json:"firstLaunch"`
	BeerSnooze  int64 `json:"beerSnooze"`
	BeerDone    bool  `json:"beerDone"`
	// Windows only: original "lid close action" values, set while LidClosed mode is on.
	LidChanged bool `json:"lidChanged"`
	LidAC      int  `json:"lidAC"`
	LidDC      int  `json:"lidDC"`
}

var (
	state     State
	stateMu   sync.Mutex
	stateFile string
	current   = Off
)

func loadState() {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	stateFile = filepath.Join(dir, "Pisolo", "state.json")
	if b, err := os.ReadFile(stateFile); err == nil {
		_ = json.Unmarshal(b, &state)
	}
	if state.FirstLaunch == 0 {
		state.FirstLaunch = time.Now().Unix()
		saveState()
	}
}

func saveState() {
	_ = os.MkdirAll(filepath.Dir(stateFile), 0o755)
	b, _ := json.MarshalIndent(state, "", "  ")
	tmp := stateFile + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, stateFile)
	}
}

// setMode switches mode; lid-closed falls back to always-on when it cannot be enabled.
func setMode(m Mode) Mode {
	stateMu.Lock()
	defer stateMu.Unlock()
	if err := applyMode(m, &state); err != nil {
		fmt.Fprintln(os.Stderr, "pisolo:", err)
		if m == LidClosed {
			m = Awake
			_ = applyMode(m, &state)
		} else {
			m = Off
			_ = applyMode(Off, &state)
		}
	}
	current = m
	state.Mode = m
	saveState()
	return m
}

func shutdown() {
	stateMu.Lock()
	defer stateMu.Unlock()
	_ = applyMode(Off, &state)
	saveState()
}

func main() {
	modeFlag := flag.Int("mode", -1, "start in mode 0 (off), 1 (always on) or 2 (always on, lid closed)")
	headless := flag.Bool("headless", false, "no tray icon: hold the mode until Ctrl-C (servers, scripts)")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println("Pisolo", version)
		return
	}
	if !acquireSingleInstance() {
		fmt.Fprintln(os.Stderr, "pisolo: already running")
		os.Exit(1)
	}

	loadState()
	recoverAfterCrash(&state)
	saveState()

	start := state.Mode
	if *modeFlag >= 0 && *modeFlag <= 2 {
		start = Mode(*modeFlag)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	if *headless {
		m := setMode(start)
		fmt.Println("Pisolo:", title(m), "— Ctrl-C to stop")
		watchBattery(nil)
		<-sig
		shutdown()
		return
	}

	go func() { <-sig; systray.Quit() }()
	systray.Run(func() { onReady(start) }, shutdown)
}

// watchBattery turns Pisolo off when on battery below 10%.
func watchBattery(onOff func()) {
	go func() {
		for range time.Tick(time.Minute) {
			if current != Off && batteryLow() {
				setMode(Off)
				if onOff != nil {
					onOff()
				}
			}
		}
	}()
}

// ---------- Tray ----------

func onReady(start Mode) {
	systray.SetTitle("Pisolo")
	items := map[Mode]*systray.MenuItem{}
	for _, m := range []Mode{Off, Awake, LidClosed} {
		items[m] = systray.AddMenuItemCheckbox(title(m), hint(m), false)
	}
	systray.AddSeparator()
	login := systray.AddMenuItemCheckbox(t("Launch at login", "Avvia al login"), "", autostartEnabled())
	about := systray.AddMenuItem(t("About Pisolo", "Info su Pisolo"), "")
	beer := systray.AddMenuItem(t("Buy me a beer 🍺", "Offrimi una birra 🍺"), donateURL)
	systray.AddSeparator()
	quit := systray.AddMenuItem(t("Quit Pisolo", "Esci da Pisolo"), "")

	refresh := func() {
		for m, it := range items {
			if m == current {
				it.Check()
			} else {
				it.Uncheck()
			}
		}
		systray.SetIcon(trayIcon(current))
		systray.SetTooltip("Pisolo — " + title(current))
	}
	setMode(start)
	refresh()
	watchBattery(refresh)
	time.AfterFunc(10*time.Second, maybeAskForBeer)

	for m, it := range items {
		go func(m Mode, it *systray.MenuItem) {
			for range it.ClickedCh {
				setMode(m)
				refresh()
			}
		}(m, it)
	}
	go func() {
		for {
			select {
			case <-login.ClickedCh:
				if err := setAutostart(!autostartEnabled()); err == nil {
					if autostartEnabled() {
						login.Check()
					} else {
						login.Uncheck()
					}
				}
			case <-about.ClickedCh:
				showInfo("Pisolo "+version, t(
					"Keeps your computer awake so Claude and your other tools keep working.\n\nFree and open source (MIT), beerware in spirit: if it is useful to you, you can buy me a beer.",
					"Tiene acceso il computer così Claude e gli altri strumenti continuano a lavorare.\n\nGratuito e open source (MIT), beerware nello spirito: se ti è utile, puoi offrirmi una birra."))
			case <-beer.ClickedCh:
				buyBeer()
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func buyBeer() {
	openURL(donateURL)
	stateMu.Lock()
	state.BeerDone = true
	saveState()
	stateMu.Unlock()
}

// maybeAskForBeer shows one gentle reminder after a week. "Later" asks again in 30 days.
func maybeAskForBeer() {
	now := time.Now().Unix()
	stateMu.Lock()
	askAfter := max(state.FirstLaunch+7*86400, state.BeerSnooze)
	done := state.BeerDone
	stateMu.Unlock()
	if done || now < askAfter {
		return
	}
	answer := askBeer(
		t("Pisolo has kept your computer awake for a week 🍺", "Pisolo tiene sveglio il tuo computer da una settimana 🍺"),
		t("Pisolo is free and open source. If it is useful to you, you can buy me a beer: it takes a minute and keeps the project alive.",
			"Pisolo è gratuito e open source. Se ti è utile, puoi offrirmi una birra: ci vuole un minuto e aiuta il progetto ad andare avanti."))
	stateMu.Lock()
	defer stateMu.Unlock()
	switch answer {
	case 0:
		openURL(donateURL)
		state.BeerDone = true
	case 1:
		state.BeerSnooze = now + 30*86400
	case 2:
		state.BeerDone = true
	default: // no dialog tool available: try again in 30 days
		state.BeerSnooze = now + 30*86400
	}
	saveState()
}

// ---------- Strings ----------

func t(en, it string) string {
	if isItalian() {
		return it
	}
	return en
}

func title(m Mode) string {
	switch m {
	case Awake:
		return t("Always on", "Sempre acceso")
	case LidClosed:
		return t("Always on, even with lid closed", "Sempre acceso anche con schermo chiuso")
	}
	return t("Off", "Disattivato")
}

func hint(m Mode) string {
	switch m {
	case Awake:
		return t("Screen and computer stay on", "Schermo e computer restano accesi")
	case LidClosed:
		return t("The computer keeps running; the screen may turn off", "Il computer resta in funzione, lo schermo può spegnersi")
	}
	return ""
}
