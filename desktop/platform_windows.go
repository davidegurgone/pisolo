//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procSetThreadExecutionState = kernel32.NewProc("SetThreadExecutionState")
	procGetSystemPowerStatus    = kernel32.NewProc("GetSystemPowerStatus")
	procGetUserDefaultUILang    = kernel32.NewProc("GetUserDefaultUILanguage")
	procMessageBoxW             = user32.NewProc("MessageBoxW")
)

const (
	esContinuous      = 0x80000000
	esSystemRequired  = 0x00000001
	esDisplayRequired = 0x00000002
)

// SetThreadExecutionState is per thread, and Go moves goroutines between threads:
// one goroutine locked to its own OS thread owns the execution state.
var execState = make(chan uintptr)

func init() {
	go func() {
		runtime.LockOSThread()
		for flags := range execState {
			procSetThreadExecutionState.Call(flags)
		}
	}()
}

func applyMode(m Mode, s *State) error {
	if m == LidClosed {
		if err := lidDoNothing(s); err != nil {
			return err
		}
	} else if s.LidChanged {
		restoreLid(s)
	}
	switch m {
	case Off:
		execState <- esContinuous
	case Awake:
		execState <- esContinuous | esSystemRequired | esDisplayRequired
	case LidClosed:
		execState <- esContinuous | esSystemRequired
	}
	return nil
}

func recoverAfterCrash(s *State) {
	if s.LidChanged {
		restoreLid(s)
	}
}

// ---------- Lid close action (powercfg) ----------

var (
	hexValue = regexp.MustCompile(`0x[0-9a-fA-F]{8}`)
	errNoLid = errors.New("no lid close action on this PC")
)

func powercfg(args ...string) (string, error) {
	cmd := exec.Command("powercfg", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("powercfg %v: %v: %s", args, err, out)
	}
	return string(out), nil
}

// lidValues reads the current AC and DC lid close actions. The labels are localized,
// so we take the last two hex values: "Current AC ..." then "Current DC ...".
func lidValues() (ac, dc int, err error) {
	out, err := powercfg("/query", "SCHEME_CURRENT", "SUB_BUTTONS", "LIDACTION")
	if err != nil {
		return 0, 0, errNoLid // the setting does not exist (desktop PCs, servers)
	}
	return parseLidValues(out)
}

func parseLidValues(out string) (ac, dc int, err error) {
	v := hexValue.FindAllString(out, -1)
	if len(v) < 2 {
		return 0, 0, errNoLid
	}
	a, _ := strconv.ParseInt(v[len(v)-2][2:], 16, 32)
	d, _ := strconv.ParseInt(v[len(v)-1][2:], 16, 32)
	return int(a), int(d), nil
}

func setLid(ac, dc int) error {
	if _, err := powercfg("/setacvalueindex", "SCHEME_CURRENT", "SUB_BUTTONS", "LIDACTION", strconv.Itoa(ac)); err != nil {
		return err
	}
	if _, err := powercfg("/setdcvalueindex", "SCHEME_CURRENT", "SUB_BUTTONS", "LIDACTION", strconv.Itoa(dc)); err != nil {
		return err
	}
	_, err := powercfg("/setactive", "SCHEME_CURRENT")
	return err
}

func lidDoNothing(s *State) error {
	if s.LidChanged {
		return nil
	}
	ac, dc, err := lidValues()
	if err == errNoLid {
		return nil // no lid on this PC: keeping the system awake is all it takes
	}
	if err != nil {
		return err
	}
	// Save the originals first, so a crash can still restore them.
	s.LidAC, s.LidDC, s.LidChanged = ac, dc, true
	saveState()
	if err := setLid(0, 0); err != nil {
		restoreLid(s)
		return err
	}
	return nil
}

func restoreLid(s *State) {
	if setLid(s.LidAC, s.LidDC) == nil {
		s.LidChanged = false
	}
}

// ---------- Battery ----------

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

func batteryLow() bool {
	var st systemPowerStatus
	r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return false
	}
	return st.ACLineStatus == 0 && st.BatteryLifePercent != 255 && st.BatteryLifePercent < 10
}

// ---------- Autostart (HKCU Run key) ----------

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

func autostartEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue("Pisolo")
	return err == nil
}

func setAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		return k.DeleteValue("Pisolo")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue("Pisolo", `"`+exe+`"`)
}

// ---------- UI helpers ----------

func openURL(url string) {
	verb, _ := windows.UTF16PtrFromString("open")
	u, _ := windows.UTF16PtrFromString(url)
	_ = windows.ShellExecute(0, verb, u, nil, nil, windows.SW_SHOWNORMAL)
}

const (
	mbOK          = 0x0
	mbYesNoCancel = 0x3
	mbIconInfo    = 0x40
	mbTopmost     = 0x40000
	idYes         = 6
	idNo          = 7
)

func messageBox(title, text string, flags uintptr) int {
	t16, _ := windows.UTF16PtrFromString(title)
	x16, _ := windows.UTF16PtrFromString(text)
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(x16)), uintptr(unsafe.Pointer(t16)), flags)
	return int(r)
}

func showInfo(title, text string) { messageBox(title, text, mbOK|mbIconInfo|mbTopmost) }

func askBeer(title, text string) int {
	text += t("\n\nYes = buy a beer · No = later · Cancel = don't ask again",
		"\n\nSì = offri una birra · No = più tardi · Annulla = non chiedere più")
	switch messageBox(title, text, mbYesNoCancel|mbIconInfo|mbTopmost) {
	case idYes:
		return 0
	case idNo:
		return 1
	default:
		return 2
	}
}

// ---------- Misc ----------

func acquireSingleInstance() bool {
	name, _ := windows.UTF16PtrFromString(`Local\PisoloSingleInstance`)
	_, err := windows.CreateMutex(nil, false, name)
	return err != windows.ERROR_ALREADY_EXISTS
}

func isItalian() bool {
	r, _, _ := procGetUserDefaultUILang.Call()
	return r&0x3ff == 0x10 // LANG_ITALIAN
}
