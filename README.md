# Pisolo 🍺

**Pisolo never naps. Close the lid, your computer keeps working.**

*Pisolo* is Italian for "nap". A tiny tray / menu bar app for **macOS, Windows
and Linux** that keeps your computer awake — even with the lid closed — so
long-running jobs, servers, downloads and AI agents (Claude Code, remote
sessions, browser automation) keep working while you are away.

> **Status:** tested on macOS 26 (Apple Silicon). Windows, Linux and
> older macOS versions (13–15) are **in testing**: the CI checks every mode on
> real Windows and Linux machines, but not yet on physical laptops with a lid.
> Feedback and [issues](../../issues) are very welcome.

Three modes, one click:

| Mode | Screen | Computer |
|---|---|---|
| **Off** | normal | normal |
| **Always on** | stays on | stays on |
| **Always on, even with lid closed** | may sleep (idle timeout, or lid closed) | keeps running, like a server |

## Install

Download the package for your system from [Releases](../../releases).

### macOS (13 or later, Apple Silicon + Intel)

1. Unzip `Pisolo-macOS.zip` and move `Pisolo.app` to `/Applications`.
2. Pisolo is not notarized by Apple, so the first time macOS will refuse to
   open it. Either run

   ```sh
   xattr -dr com.apple.quarantine /Applications/Pisolo.app
   ```

   or try to open it once, then go to *System Settings → Privacy & Security*
   and click **Open Anyway**.
3. Click the cup in the menu bar and pick a mode.

### Windows (10 / 11, x64 + ARM64)

1. Unzip `pisolo-windows-amd64.zip` (or `-arm64`) anywhere, e.g.
   `%LOCALAPPDATA%\Pisolo`.
2. Run `pisolo.exe`. SmartScreen may warn you because the app is not signed:
   click *More info → Run anyway*.
3. The cup appears in the notification area (it may hide under the `^` arrow).

### Linux (x86_64 + ARM64)

```sh
tar -xzf pisolo-linux-amd64.tar.gz
./pisolo-linux-amd64/pisolo &
```

Needs systemd (logind) and a desktop with a system tray
(StatusNotifierItem): KDE, Xfce, Cinnamon, MATE, Budgie, and GNOME with the
*AppIndicator* extension (preinstalled on Ubuntu).

### Headless (Windows and Linux)

No tray, just hold a mode until Ctrl-C — handy on servers and in scripts:

```sh
pisolo --headless --mode 2        # Linux
pisolo-cli.exe --headless --mode 2   # Windows
```

## How it works

| | Always on | Always on, lid closed |
|---|---|---|
| **macOS** | IOKit assertions: no idle display sleep, no idle system sleep (like `caffeinate -di`) | system assertion + `pmset -a disablesleep 1` |
| **Windows** | `SetThreadExecutionState` (system + display) | system only + lid close action set to *Do nothing* via `powercfg` |
| **Linux** | logind inhibitor `idle:sleep` + desktop screensaver inhibit | logind inhibitor `idle:sleep:handle-lid-switch` |

- **macOS:** `pmset disablesleep` needs root. The first time, Pisolo asks for
  your admin password once and installs `/etc/sudoers.d/pisolo`, a rule that
  allows only `pmset -a disablesleep 0` and `pmset -a disablesleep 1` for your
  user. Remove it with `sudo rm /etc/sudoers.d/pisolo`.
- **Windows:** no admin rights needed. Pisolo saves your lid close settings
  before changing them and puts them back when you switch mode or quit. On PCs
  without a lid it just keeps the system awake.
- **Linux:** no root and nothing permanent: logind releases the inhibitor as
  soon as Pisolo exits, even if it crashes.

Safety nets, everywhere:

- normal sleep is restored when you switch mode or quit, and on the next launch
  if Pisolo ever crashed while the lid-closed mode was on;
- on battery below 10% Pisolo switches itself off;
- only one copy of Pisolo can run at a time.

⚠️ A laptop running with the lid closed inside a bag can get hot. Use the
lid-closed mode on a desk, ideally on power.

## Build from source

```sh
macos/build.sh      # → macos/build/Pisolo.app (Xcode Command Line Tools only)
desktop/build.sh    # → desktop/dist/ for Windows and Linux (Go 1.26+)
```

Windows and Linux are tested on real machines on every push
([workflow](.github/workflows/desktop.yml)).

## License

MIT licensed, beerware in spirit 🍺 Pisolo is free: if it saves your night,
you can [buy me a beer](https://www.paypal.me/davidegurgone).
See [LICENSE](LICENSE).
