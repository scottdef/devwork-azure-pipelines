# macOS Ventura 13.7.6 → Sonoma 14
## CLI Upgrade Scripts

> Rob Pike + Ken Thompson approved. No clicking. No banners. No Sequoia bleed-through.

---

## Prerequisites

- macBook running **macOS Ventura 13.7.x**
- **35 GB** free disk space
- AC power connected (laptop)
- Time Machine backup completed
- Internet connection (Apple CDN must be reachable)

---

## Usage

### Option A — Run everything in sequence

```bash
chmod +x *.sh
./run-all.sh
```

After the machine reboots into Sonoma, log back in and run:

```bash
./04-validate-sonoma.sh
```

---

### Option B — Run phases individually

```bash
chmod +x *.sh

# Phase 0: checks hardware, disk space, power, backups
./00-pre-flight.sh

# Phase 1: downloads macOS Sonoma 14 full installer (~13GB)
./01-download-sonoma.sh

# Phase 2: (optional) burns installer to USB for recovery
./02-make-bootable-usb.sh

# Phase 3: launches upgrade — machine WILL reboot
./03-install-sonoma.sh

# Phase 4: run AFTER reboot — confirms Sonoma 14 + system health
./04-validate-sonoma.sh
```

---

## Script Reference

| Script | What it does |
|--------|--------------|
| `00-pre-flight.sh` | OS version, disk space, hardware, Time Machine, SIP, power, network |
| `01-download-sonoma.sh` | `softwareupdate --fetch-full-installer --full-installer-version 14` |
| `02-make-bootable-usb.sh` | `createinstallmedia` to USB — erases drive, creates recovery media |
| `03-install-sonoma.sh` | `startosinstall --agreetolicense --nointeraction` — triggers reboot |
| `04-validate-sonoma.sh` | OS version, SIP, disk health, updates, Homebrew, Rosetta 2, Xcode CLT |
| `run-all.sh` | Orchestrates phases 0→3, reminds you to run 04 post-reboot |

---

## Key Facts

| Item | Value |
|------|-------|
| Source OS | Ventura 13.7.6 (22H625) |
| Target OS | Sonoma 14 (latest: 14.8.7 as of June 2026) |
| Installer size | ~13 GB |
| Upgrade time | 30–60 min |
| Reboots during upgrade | 2–3 (normal) |
| Ventura support status | **Unsupported** as of Sep 15, 2025 |
| Sonoma support | Security updates until ~Sep 2026 |

---

## The Apple UI Trap

If you ever open System Preferences → Software Update, Apple will show a big
Sequoia/Tahoe banner. **Ignore it.** Look for "Also available" → "More Info…"
to find Sonoma 14 specifically.

These scripts use the CLI exclusively and pin to version 14. Sequoia will not
be offered.

---

## After Upgrade

If you use Homebrew:
```bash
brew update && brew upgrade && brew cleanup
```

If on Apple Silicon and Rosetta 2 needs reinstall:
```bash
softwareupdate --install-rosetta --agree-to-license
```

If Xcode CLI tools need re-accepting:
```bash
xcode-select --install
```
