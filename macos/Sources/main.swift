// Pisolo — keeps your Mac awake, even with the lid closed.
// License: MIT (see LICENSE). Beerware in spirit.

import Cocoa
import IOKit.pwr_mgt
import IOKit.ps
import ServiceManagement

// MARK: - Modes

enum Mode: Int, CaseIterable {
    case off = 0
    case awake = 1        // screen and Mac stay on
    case lidClosed = 2    // Mac stays on even with the lid closed

    var symbol: String {
        switch self {
        case .off: return "cup.and.saucer"
        case .awake: return "cup.and.heat.waves.fill"
        case .lidClosed: return "laptopcomputer.and.arrow.down"
        }
    }
}

// MARK: - Strings (Italian / English)

struct L {
    static let it = Locale.preferredLanguages.first?.hasPrefix("it") ?? false
    static func t(_ en: String, _ itText: String) -> String { it ? itText : en }

    static func title(_ m: Mode) -> String {
        switch m {
        case .off: return t("Off", "Disattivato")
        case .awake: return t("Always on", "Sempre acceso")
        case .lidClosed: return t("Always on, even with lid closed", "Sempre acceso anche con schermo chiuso")
        }
    }
    static func hint(_ m: Mode) -> String? {
        switch m {
        case .off: return nil
        case .awake: return t("Screen and Mac stay on", "Schermo e Mac restano accesi")
        case .lidClosed: return t("The Mac keeps running; the screen may turn off", "Il Mac resta in funzione, lo schermo può spegnersi")
        }
    }
    static let launchAtLogin = t("Launch at login", "Avvia al login")
    static let about = t("About Pisolo (Beerware)", "Info su Pisolo (Beerware)")
    static let beer = t("Buy me a beer 🍺", "Offrimi una birra 🍺")
    static let later = t("Later", "Più tardi")
    static let noThanks = t("No, thanks", "No, grazie")
    static let nudgeTitle = t("Pisolo has kept your Mac awake for a week 🍺", "Pisolo tiene sveglio il tuo Mac da una settimana 🍺")
    static let nudgeText = t(
        "Pisolo is free and open source (MIT). If it is useful to you, you can buy me a beer: it takes a minute and keeps the project alive.",
        "Pisolo è gratuito e open source (MIT). Se ti è utile, puoi offrirmi una birra: ci vuole un minuto e aiuta il progetto ad andare avanti.")
    static let quit = t("Quit Pisolo", "Esci da Pisolo")
    static let batteryNote = t("Turned off: battery below 10%", "Disattivato: batteria sotto il 10%")
    static let setupTitle = t("One-time setup", "Configurazione iniziale")
    static let setupText = t(
        "To keep the Mac awake with the lid closed, Pisolo needs to run “pmset disablesleep”, which requires administrator rights.\n\nPisolo will install a small rule (/etc/sudoers.d/pisolo) that allows only that command, so you will be asked for your password just this once.",
        "Per tenere acceso il Mac con lo schermo chiuso, Pisolo deve eseguire “pmset disablesleep”, che richiede i permessi di amministratore.\n\nPisolo installerà una piccola regola (/etc/sudoers.d/pisolo) che consente solo quel comando: la password ti verrà chiesta solo questa volta.")
    static let setupContinue = t("Continue", "Continua")
    static let cancel = t("Cancel", "Annulla")
    static let failed = t("Could not enable lid-closed mode.", "Impossibile attivare la modalità con schermo chiuso.")
    static let aboutText = t(
        "Keeps your Mac awake so Claude and your other tools keep working.\n\nBeerware: if we meet some day and you think this is worth it, you can buy me a beer.",
        "Tiene acceso il Mac così Claude e gli altri strumenti continuano a lavorare.\n\nBeerware: se un giorno ci incontriamo e pensi che ne valga la pena, offrimi una birra.")
}

// MARK: - Sleep control

final class SleepController {
    private var displayAssertion: IOPMAssertionID?
    private var systemAssertion: IOPMAssertionID?
    private let pmset = "/usr/bin/pmset"
    private let sudoersPath = "/etc/sudoers.d/pisolo"

    /// Holds IOKit assertions that stop idle sleep of the display and of the system.
    func holdAssertions(system: Bool, display: Bool) {
        systemAssertion = toggle(systemAssertion, system, kIOPMAssertionTypePreventUserIdleSystemSleep)
        displayAssertion = toggle(displayAssertion, display, kIOPMAssertionTypePreventUserIdleDisplaySleep)
    }

    private func toggle(_ id: IOPMAssertionID?, _ on: Bool, _ type: String) -> IOPMAssertionID? {
        if on {
            if let id { return id }
            var new: IOPMAssertionID = 0
            let r = IOPMAssertionCreateWithName(type as CFString, IOPMAssertionLevel(kIOPMAssertionLevelOn),
                                                "Pisolo keeps the Mac awake" as CFString, &new)
            return r == kIOReturnSuccess ? new : nil
        }
        if let id { IOPMAssertionRelease(id) }
        return nil
    }

    /// True when the system-wide "SleepDisabled" flag is set (lid close won't sleep the Mac).
    var sleepDisabled: Bool {
        let out = run(pmset, ["-g"]).output
        for line in out.split(separator: "\n") where line.contains("SleepDisabled") {
            return line.split(whereSeparator: { $0 == " " || $0 == "\t" }).last == "1"
        }
        return false
    }

    var helperInstalled: Bool { FileManager.default.fileExists(atPath: sudoersPath) }

    /// Sets `pmset -a disablesleep` through the sudoers rule. Returns success.
    @discardableResult
    func setDisableSleep(_ on: Bool) -> Bool {
        let r = run("/usr/bin/sudo", ["-n", pmset, "-a", "disablesleep", on ? "1" : "0"])
        return r.status == 0 && sleepDisabled == on
    }

    /// Installs the sudoers rule after asking for the admin password once.
    func installHelper() -> Bool {
        let user = NSUserName()
        guard user.range(of: "^[A-Za-z0-9._-]+$", options: .regularExpression) != nil else { return false }
        let rule = "\(user) ALL=(root) NOPASSWD: \(pmset) -a disablesleep 1, \(pmset) -a disablesleep 0"
        let tmp = NSTemporaryDirectory() + "pisolo.sudoers"
        do { try (rule + "\n").write(toFile: tmp, atomically: true, encoding: .utf8) } catch { return false }
        let cmd = "/usr/sbin/visudo -cf '\(tmp)' && /usr/bin/install -m 0440 -o root -g wheel '\(tmp)' '\(sudoersPath)'"
        let script = "do shell script \"\(cmd)\" with administrator privileges"
        var err: NSDictionary?
        NSAppleScript(source: script)?.executeAndReturnError(&err)
        try? FileManager.default.removeItem(atPath: tmp)
        return err == nil && helperInstalled
    }

    private func run(_ path: String, _ args: [String]) -> (status: Int32, output: String) {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: path)
        p.arguments = args
        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = Pipe()
        do { try p.run() } catch { return (-1, "") }
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        return (p.terminationStatus, String(data: data, encoding: .utf8) ?? "")
    }
}

// MARK: - Battery

func batteryLow() -> Bool {
    guard let info = IOPSCopyPowerSourcesInfo()?.takeRetainedValue() else { return false }
    let source = IOPSGetProvidingPowerSourceType(info)?.takeUnretainedValue() as String?
    guard source == kIOPSBatteryPowerValue else { return false }
    let list = IOPSCopyPowerSourcesList(info)?.takeRetainedValue() as? [CFTypeRef] ?? []
    for ps in list {
        if let d = IOPSGetPowerSourceDescription(info, ps)?.takeUnretainedValue() as? [String: Any],
           let cap = d[kIOPSCurrentCapacityKey] as? Int, let max = d[kIOPSMaxCapacityKey] as? Int, max > 0 {
            return cap * 100 / max < 10
        }
    }
    return false
}

// MARK: - App

final class AppDelegate: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private let statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
    private let sleep = SleepController()
    private let defaults = UserDefaults.standard
    private var mode: Mode = .off
    private var batteryTimer: Timer?
    private var signalSources: [DispatchSourceSignal] = []

    func applicationDidFinishLaunching(_ note: Notification) {
        // Only one copy: a second one would undo the first one's sleep settings.
        if let id = Bundle.main.bundleIdentifier,
           NSRunningApplication.runningApplications(withBundleIdentifier: id).count > 1 { exit(0) }
        // If a previous run died while lid-closed mode was on, restore normal sleep.
        if defaults.bool(forKey: "weDisabledSleep") && sleep.sleepDisabled {
            sleep.setDisableSleep(false)
        }
        defaults.set(false, forKey: "weDisabledSleep")

        let menu = NSMenu()
        menu.delegate = self
        statusItem.menu = menu

        handleTerminationSignals()
        batteryTimer = Timer.scheduledTimer(withTimeInterval: 60, repeats: true) { [weak self] _ in
            guard let self, self.mode != .off, batteryLow() else { return }
            self.apply(.off)
            self.statusItem.button?.toolTip = L.batteryNote
        }

        // Last used mode (or `-mode N` on the command line, handy for testing).
        apply(Mode(rawValue: defaults.integer(forKey: "mode")) ?? .off, interactive: false)
        DispatchQueue.main.asyncAfter(deadline: .now() + 10) { [weak self] in self?.maybeAskForBeer() }
    }

    func applicationWillTerminate(_ note: Notification) {
        sleep.holdAssertions(system: false, display: false)
        if mode == .lidClosed { sleep.setDisableSleep(false) }
        defaults.set(false, forKey: "weDisabledSleep")
    }

    // Rebuild the menu each time it opens so the checkmarks are current.
    func menuNeedsUpdate(_ menu: NSMenu) {
        menu.removeAllItems()
        for m in Mode.allCases {
            let item = NSMenuItem(title: L.title(m), action: #selector(pick(_:)), keyEquivalent: "")
            item.tag = m.rawValue
            item.state = (m == mode) ? .on : .off
            item.image = NSImage(systemSymbolName: m.symbol, accessibilityDescription: nil)
            item.toolTip = L.hint(m)
            item.target = self
            menu.addItem(item)
        }
        menu.addItem(.separator())
        let login = NSMenuItem(title: L.launchAtLogin, action: #selector(toggleLogin), keyEquivalent: "")
        login.state = SMAppService.mainApp.status == .enabled ? .on : .off
        login.target = self
        menu.addItem(login)
        let about = NSMenuItem(title: L.about, action: #selector(showAbout), keyEquivalent: "")
        about.target = self
        menu.addItem(about)
        let beer = NSMenuItem(title: L.beer, action: #selector(buyBeer), keyEquivalent: "")
        beer.target = self
        menu.addItem(beer)
        menu.addItem(.separator())
        menu.addItem(NSMenuItem(title: L.quit, action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q"))
    }

    @objc private func pick(_ sender: NSMenuItem) {
        apply(Mode(rawValue: sender.tag) ?? .off)
    }

    private func apply(_ new: Mode, interactive: Bool = true) {
        var target = new
        if target == .lidClosed && !enableLidClosed(interactive: interactive) {
            target = .awake   // fall back to the next best thing
        }
        if target != .lidClosed && sleep.sleepDisabled && mode == .lidClosed {
            sleep.setDisableSleep(false)
        }
        mode = target
        refreshAssertions()
        defaults.set(target.rawValue, forKey: "mode")
        defaults.set(target == .lidClosed, forKey: "weDisabledSleep")
        updateIcon()
    }

    /// "Always on" keeps the screen on too. Lid-closed mode keeps only the Mac running:
    /// the screen sleeps as usual (idle timeout with lid open, immediately when closed).
    private func refreshAssertions() {
        sleep.holdAssertions(system: mode != .off, display: mode == .awake)
    }

    private func enableLidClosed(interactive: Bool) -> Bool {
        if sleep.setDisableSleep(true) { return true }
        guard interactive else { return false }
        NSApp.activate(ignoringOtherApps: true)
        if !sleep.helperInstalled {
            let a = NSAlert()
            a.messageText = L.setupTitle
            a.informativeText = L.setupText
            a.addButton(withTitle: L.setupContinue)
            a.addButton(withTitle: L.cancel)
            guard a.runModal() == .alertFirstButtonReturn, sleep.installHelper() else { return false }
            if sleep.setDisableSleep(true) { return true }
        }
        let a = NSAlert()
        a.messageText = L.failed
        a.runModal()
        return false
    }

    private func updateIcon() {
        let img = NSImage(systemSymbolName: mode.symbol, accessibilityDescription: L.title(mode))
        img?.isTemplate = true
        statusItem.button?.image = img
        statusItem.button?.toolTip = "Pisolo — " + L.title(mode)
        statusItem.button?.appearsDisabled = (mode == .off)
    }

    @objc private func toggleLogin() {
        let s = SMAppService.mainApp
        do { s.status == .enabled ? try s.unregister() : try s.register() } catch { NSSound.beep() }
    }

    @objc private func showAbout() {
        NSApp.activate(ignoringOtherApps: true)
        let a = NSAlert()
        let v = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""
        a.messageText = "Pisolo \(v)"
        a.informativeText = L.aboutText
        a.icon = NSImage(systemSymbolName: "mug.fill", accessibilityDescription: nil)
        a.addButton(withTitle: "OK")
        a.addButton(withTitle: L.beer)
        if a.runModal() == .alertSecondButtonReturn { buyBeer() }
    }

    @objc private func buyBeer() {
        let link = Bundle.main.infoDictionary?["PisoloDonateURL"] as? String ?? ""
        if let url = URL(string: link) { NSWorkspace.shared.open(url) }
        defaults.set(true, forKey: "beerDone")
    }

    /// One gentle reminder after a week of use. "Later" asks again in 30 days, "No, thanks" never.
    private func maybeAskForBeer() {
        let now = Date().timeIntervalSince1970
        if defaults.double(forKey: "firstLaunch") == 0 { defaults.set(now, forKey: "firstLaunch") }
        let askAfter = max(defaults.double(forKey: "firstLaunch") + 7 * 86400, defaults.double(forKey: "beerSnooze"))
        guard !defaults.bool(forKey: "beerDone"), now >= askAfter else { return }
        NSApp.activate(ignoringOtherApps: true)
        let a = NSAlert()
        a.messageText = L.nudgeTitle
        a.informativeText = L.nudgeText
        a.icon = NSImage(systemSymbolName: "mug.fill", accessibilityDescription: nil)
        a.addButton(withTitle: L.beer)
        a.addButton(withTitle: L.later)
        a.addButton(withTitle: L.noThanks)
        switch a.runModal() {
        case .alertFirstButtonReturn: buyBeer()
        case .alertSecondButtonReturn: defaults.set(now + 30 * 86400, forKey: "beerSnooze")
        default: defaults.set(true, forKey: "beerDone")
        }
    }

    /// `kill` / Ctrl-C should still restore normal sleep settings.
    private func handleTerminationSignals() {
        for sig in [SIGTERM, SIGINT, SIGHUP] {
            signal(sig, SIG_IGN)
            let src = DispatchSource.makeSignalSource(signal: sig, queue: .main)
            src.setEventHandler { NSApp.terminate(nil) }
            src.resume()
            signalSources.append(src)
        }
    }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.accessory)
app.run()
