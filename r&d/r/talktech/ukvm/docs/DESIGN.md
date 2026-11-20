# Chief of Staff: a $650 voice-and-vision universal KVM

Design, build plan and working prototype. Prepared 20 November 2026.

## 1. Summary

The system is a small rack of boards that plugs into computers as their
keyboard, mouse and monitor, and takes orders from one person by voice,
touch or text. One board, the chief of staff, receives every order, decides
whether it may run, hands each step to the device that should carry it out,
and keeps the records. It installs nothing on the computers it operates.

The decisions that shape the build:

| Decision | Choice | Why |
|---|---|---|
| Chief of staff | Raspberry Pi 5, 4 GB ($110) | The three speech and vision models total under 70 MB, and the design borrows heavy work from other machines. The 8 GB board now lists at $175, which buys nothing this design uses. |
| Hands (keyboard and mouse) | One CH9329 USB cable per target (about $12) | A hardware HID device driven over a serial port. It makes any board with a USB host port able to type on a target, which matters because the Pi 3 B+ cannot act as a USB device itself. |
| Eyes (display) | One UVC HDMI capture dongle per target ($12 to $20) | The computer sees a monitor; the staff node sees a camera. |
| Existing boards | Pi 3 B+ and Coral become satellites holding tethers; the Pico i.MX7D kit becomes the 5-inch front panel; the Screen Crab becomes an independent console recorder | Each does a job its hardware suits, and none sits where its age can stop the system. |
| Cost | $544.90 in parts, $588.49 with an 8% allowance for tax and shipping | $61.51 of the $650 is left unallocated. |

What exists today is a software prototype: one static Go binary (`cos`,
standard library only, about 3,300 lines plus 1,500 lines of tests) with 49
passing tests and a scripted demonstration that runs the whole staff against
simulated computers. It cross-compiles for all three board architectures.

What has not been done: no real cable, capture device, microphone or speech
engine has been attached. Section 11 lists every unverified item.

Use the system only on computers you own or administer.

## 2. How the staff is organised

The brief asked for a chief of staff that takes dictated or written orders,
passes tasks to subordinates, keeps reporting, monitoring, tracking and
troubleshooting to itself, and leaves reorganisation and direction to the
human. The design maps onto that directly.

| Role | Who | Does | Does not |
|---|---|---|---|
| Principal | You | Give orders; confirm anything that changes a computer; edit the policy; approve runbooks; hold the arm switch | |
| Chief of staff | Pi 5 running `cos serve` | Intake (speech, text, panel), gating, placement, job tracking, device monitoring, reports, audit ledger, refusing unsafe situations | Decide policy. It cannot add a command to its own vocabulary. |
| Satellites | Pi 3 B+, Coral | Hold the cables for targets the chief cannot reach; type and look when told | Take orders from anyone but the chief |
| Lenders | Any reachable computer with SSH | Lend a shell, PDF conversion or OCR. No agent installed. | |
| Targets | The computers being operated | Receive keystrokes and show a display | Run any of this software |

Terms used below: a **tether** is the pair of cables on one target, hands
(CH9329) and eyes (capture dongle). **Armed** means actions are really
performed; when not armed, which is the default, every action is gated,
logged and described but nothing is sent.

## 3. The parts on hand

| Part | What it is | Role in this build | Limits |
|---|---|---|---|
| Raspberry Pi 3 Model B+ | Quad Cortex-A53, 1 GB, four USB 2.0 host ports, Ethernet. It is not among the boards Raspberry Pi lists as supporting USB gadget mode; its USB controller sits behind an on-board hub. | Satellite for a third target (capture dongle and CH9329 cable on its host ports); helper for OCR and PDF work | USB 2.0 only, so it gets a USB 2.0 capture dongle |
| Coral dev board (2 GB) | Two boards answer to this name. The Dev Board Mini has 2 GB, a MediaTek MT8167S, Wi-Fi, no Ethernet jack and USB-C. The original Dev Board is an NXP i.MX 8M with Ethernet, whose module was later offered with 2 GB. An Ethernet jack tells them apart. | Satellite for a fourth target, and its Edge TPU is offered to the broker as a `tpu` capability for a later wake-word or screen-change detector | Mendel Linux is frozen on Debian 10. A static Go binary does not care, but the speech engine is untested there, so nothing critical runs on it. The Mini needs a USB-C hub with Ethernet ($15, in phase 3). |
| Android Things Pico i.MX7D kit | NXP i.MX7 Dual board with a 5-inch 800×480 capacitive touch LCD, camera, Gigabit Ethernet and eMMC (no SD slot). Android Things is discontinued. TechNexion publishes Yocto and Ubuntu images with the 800×480 LCD enabled, and one forum report has display and touch working on mainline Linux 5.6. | The front panel: status, CONFIRM and CANCEL. Its camera can photograph paper or another screen for image-to-text. | Needs reflashing over USB. Wi-Fi under mainline Linux is reported as troublesome, so it goes on Ethernet. Two Cortex-A7 cores will not run a heavy browser. A $40 fallback display for the Pi 5 is budgeted in case the LCD cannot be brought up. |
| Screen Crab | An inline HDMI device that saves screenshots or video to a microSD card on a timer, configured by a text file. Its only documented live view is through Hak5's Cloud C2 service over Wi-Fi. HDMI 1.4, 1080p. | An independent recorder on the most sensitive target, between that computer and its capture dongle. Its stills are a record the chief's software cannot alter. | It has no local live interface, so it is not the live video path. Fit it only to consoles you administer, and tell the people who use them. |
| 5-port Gigabit switch | | The staff network | Four boards and one spare port fill it |

## 4. Architecture

```
                               PRINCIPAL
              speaks      taps        types        edits policy (git)
                │           │           │
     USB speakerphone   5-inch panel   laptop or phone
     (mute = push to    (Pico i.MX7D,  (CLI, API, same panel
      talk)              or fallback)   page in a browser)
                │           │           │
        ┌───────▼───────────▼───────────▼──────┐      site LAN (USB NIC)
        │  CHIEF OF STAFF   Raspberry Pi 5      ├────► lenders: reachable
        │  cos serve                            │      computers over SSH
        │  intake → gate → broker → ledger      │      (shell, PDF, OCR)
        │  holds tethers for targets 1 and 2    │
        └───────┬───────────────────────────────┘
                │  staff LAN 10.77.0.0/24  (existing switch; not routed to the site LAN)
      ┌─────────┼──────────────┬───────────────────┐
      ▼         ▼              ▼                   ▼
   Pi 3 B+    Coral          Pico i.MX7D        spare port
   satellite  satellite      front panel        (maintenance,
   tether 3   tether 4       + camera            optional pod)
              + Edge TPU

   One tether, per target:
     target HDMI out ─► [Screen Crab, one target only] ─► capture dongle ─► staff node USB   eyes
     target USB port ◄─ CH9329 cable ◄─ staff node USB                                       hands
```

### The tether

The CH9329 is a chip in a USB cable. One end is a serial port on a staff
node; the other enumerates on the target as an ordinary USB keyboard and
mouse, including a boot-protocol keyboard, so it works in firmware setup
screens. The node writes short frames (14 bytes for a key report) at 9600
baud. The chip can also
report whether the target has enumerated it, which gives the chief a cheap
check that the cable and the target's USB stack are alive.

Three reasons the Pi 5 does not act as the USB device itself, although it
can: its USB-C port is also its power input, so gadget mode needs a powered
hub or splitter; one port serves one target; and a chief that only ever
hosts USB cannot be back-powered or browned out by a target.

### Networks

The staff LAN is the existing switch with the Pi 5 at 10.77.0.1 and no
route to anything else. The Pi 5 reaches the site LAN through a USB
Ethernet adapter (or its Wi-Fi) and does not forward packets. The API listens
only on the staff LAN; the panel page listens on loopback for the local
display.

### A spoken order, end to end

1. The speakerphone is unmuted (push to talk). `arecord` pipes 16 kHz audio
   into `cos listen`, which cuts it into utterances with an energy gate.
2. One call to the Cactus runner loads Whistle (speech to text) and Needle
   (text to tool calls) and returns the transcript, the calls and a
   confidence value. Needle can only emit calls from the schema it is given,
   and that schema is generated from the policy file.
3. Phantom transcripts that small speech models produce from noise ("Thank
   you.") are dropped.
4. The gate decides: refuse, wait for confirmation, or run.
5. The broker picks who does each step. Each action is written to the
   ledger before it is performed.
6. The result (command output, or the text read back off the screen) is
   stored with the job and shown on the panel.

A written order skips steps 1 to 3 and enters at the gate with full
confidence.

### A display, an image or a PDF, to text and operations

- **Display.** A frame is grabbed from the capture device and read with
  Tesseract. `read screen` returns the text. `click "Restart"` finds the
  label's bounding box and clicks its centre with the absolute pointer.
- **Image.** Any image file can stand in for a display: a Screen Crab still,
  a photo from the Pico's camera, a screenshot.
- **PDF.** Each page goes through `pdftotext`; a page with no text layer is
  rasterised and read with Tesseract. The text is saved as a file.

Text obtained this way is data. It is stored, shown and reported, and no
code path passes it to the order parser.

## 5. Required capabilities and where each stands

| Capability | Mechanism | State |
|---|---|---|
| Speech to text, commands, scripts, input and UI operations | Microphone pipe, utterance segmenter, one Whistle + Needle call, the gate, then hands. "Scripts" are runbooks. | Segmenter, WAV framing, engine wrapper and voice endpoint are written and tested against a simulated engine. The real engine and microphone are phase 1. |
| Images, screenshots, PDFs and displays to text, commands, input and UI operations | Tesseract for text and word boxes; `read screen`, `click "label"`, `click X Y`; poppler then OCR for PDFs | Tested with the real Tesseract and poppler on fixture images and PDFs. Grabbing from a real dongle needs hardware. Mirage (for icons and screen state that have no text) is phase 3. |
| Connect to computers as keyboard, mouse and display | CH9329 cable; capture dongle (the computer sees a monitor); optional Pi Zero 2 W pod that also presents a USB drive | Frame encoding is tested against the vendor's worked examples. Device I/O needs hardware. |
| Create and manage automation by voice or direct UI, across reachable devices, drives, sensors and displays | Runbooks: named lists of orders, checked against the policy when saved, approved by the principal, with approval pinned to the file's SHA-256 and withdrawn by any edit. systemd timers run them. Devices are reached by tether, satellite or SSH. | Runbooks, approval, panel, CLI and API are tested. Timer units are provided. Tools for specific drives and sensors are not written; each would be a new entry in the closed vocabulary. |
| Track multiple devices, processes and information | Registry with liveness (probe and heartbeat), every job with each step's output, a notes store, and the hash-chained ledger | Tested |
| Reporting, monitoring, tracking, troubleshooting stay with the chief; direction stays with the human | `status`, the probe loop, job history, and guards such as refusing to type at a login prompt. Policy, approvals and arming are files and switches only the human changes. | Tested |
| Find resources elsewhere before using the system's own | The broker's three tiers (section 6) | Tested |

## 6. The resource rule

Each job states what it needs, and the broker chooses among live devices
that offer it:

1. a **lender**: a reachable computer or device that has the resource;
2. a **staff board**: a satellite;
3. the **chief's own OS**, only when nobody else can do the job.

Within a tier the least loaded device wins. A device that has not been seen
for 45 seconds is treated as down. Borrowed work travels over SSH, so a
staff board lends its CPU the same way an outside computer does; it is
simply asked after the outside computers.

Two consequences are visible in the prototype. `read pdf` streams the file
over SSH to a lender's `pdftotext` and gets text back on standard output,
and converts locally only if no lender is up. `run uptime` uses the target's
own shell over SSH when the target lends one, because a shell returns exact
text, and types on the tether and reads the screen when it does not.

Speech recognition is offered as a capability like any other, but it is
pinned to the chief in the reference configuration: a network round trip in
the middle of a spoken command costs more than the Pi 5's CPU time is worth.

## 7. Safety model

A device that types on other computers when it hears speech is a serious
tool. These are the controls, all implemented and tested in the prototype
unless marked.

| Risk | Control |
|---|---|
| An order nobody intended (TV audio, a phantom transcript, a mumble) | Push to talk on the microphone. Phantom transcripts are dropped. Below 0.50 confidence an order is refused; below 0.85 it waits for confirmation. |
| An order that should not exist | A closed vocabulary. Targets, commands and chords are lists in the policy file; anything else is refused before any device is touched. |
| An order that changes a computer | Typing, clicking, and any command or chord marked `confirm` wait for CONFIRM on the panel, which shows the decoded calls and not just the words heard. An unconfirmed order expires after five minutes. A spoken "confirm" is refused unless explicitly enabled. |
| Instructions hidden in a screen or a document | Text read from displays and PDFs is never parsed as an order. A test feeds the chief a screen full of orders and checks that nothing happens. On the panel it is escaped. |
| Automation drifting from what was approved | Runbook approval is bound to the file's hash; any edit withdraws it. Runbooks cannot start other runbooks or confirm themselves. |
| Typing into the wrong place | Before typing a command the chief reads the screen and refuses if it shows a login or password prompt. A failed step stops the job, and nothing is retried. |
| The system acting when it should be idle | It starts disarmed and restarts disarmed. Arming is a file that a script creates for a set time or a physical switch holds. |
| No record of what happened | Every order, confirmation, action and result is a line in a hash-chained ledger; a removed, edited or reordered line fails verification. The action is logged first, and if the ledger cannot be written the action does not happen. The Screen Crab gives a second, independent record of one console. |
| Someone else using the API | A bearer token, the isolated staff LAN, and optional TLS with client certificates. Satellites check each request against their own copy of the policy. |
| The hardware itself | USB keyboards bypass most endpoint controls. Treat the unit as privileged equipment: label its cables, keep it physically secured, and never leave it attached to a computer you do not administer. |

## 8. Parts and budget

Prices marked *estimate* are typical street prices that I did not look up
individually; check them when ordering. The Raspberry Pi prices are current
list prices after this year's memory-driven increases.

| Phase | Item | Qty | Each | Total | Basis |
|---|---|---|---|---|---|
| 1 | Raspberry Pi 5 (4 GB) | 1 | $110.00 | $110.00 | list price Oct 2026 |
| 1 | Raspberry Pi 27 W USB-C power supply | 1 | $12.95 | $12.95 | PiShop Oct 2026 |
| 1 | Raspberry Pi 5 case with fan | 1 | $10.95 | $10.95 | PiShop Oct 2026 |
| 1 | microSD card 64 GB A2 | 2 | $12.00 | $24.00 | estimate |
| 1 | USB 3.0 HDMI capture dongle (UVC, MS2130 class) | 2 | $20.00 | $40.00 | estimate |
| 1 | CH9329 USB-to-USB HID cable | 4 | $12.00 | $48.00 | estimate |
| 1 | HDMI cable 0.5 to 1 m | 4 | $5.00 | $20.00 | estimate |
| 1 | USB speakerphone with echo cancellation and mute button | 1 | $45.00 | $45.00 | estimate |
| 1 | Powered USB 3.0 hub (7 port) | 1 | $30.00 | $30.00 | estimate |
| 2 | USB 2.0 HDMI capture dongle (UVC, MS2109 class) | 2 | $12.00 | $24.00 | estimate |
| 2 | USB 3.0 Gigabit Ethernet adapter | 1 | $15.00 | $15.00 | estimate |
| 2 | Cat6 patch cables (5 pack) | 1 | $12.00 | $12.00 | estimate |
| 2 | 60 W 6-port USB charger with short cables | 1 | $30.00 | $30.00 | estimate |
| 2 | Latching arm switch with LED and jumper wires | 1 | $10.00 | $10.00 | estimate |
| 2 | microSD card 64 GB (Screen Crab) | 1 | $10.00 | $10.00 | estimate |
| 2 | Mounting board, standoffs, velcro, port labels | 1 | $20.00 | $20.00 | estimate |
| 3 | Raspberry Pi Zero 2 W | 1 | $15.00 | $15.00 | list price Oct 2026 |
| 3 | microSD card 32 GB and micro-USB data cable (pod) | 1 | $13.00 | $13.00 | estimate |
| 3 | 5-inch touch display for the Pi 5 (fallback) | 1 | $40.00 | $40.00 | estimate |
| 3 | USB-C hub with Ethernet (Coral Dev Board Mini only) | 1 | $15.00 | $15.00 | estimate |

| | |
|---|---|
| Phase 1 | $340.90 |
| Phase 2 | $121.00 |
| Phase 3 (optional items) | $83.00 |
| Parts | $544.90 |
| With 8% for tax and shipping | $588.49 |
| Unallocated from $650 | $61.51 |

The same list is `bom.csv` in the archive; `make bom` totals it.

Ways to move the number:

- **Down by $32.50.** A 2 GB Pi 5 ($77.50) would carry this workload. The
  4 GB board leaves room for a small local language model later.
- **Up for a stronger chief.** A 16 GB Intel N150 mini PC is about $285 and
  replaces the Pi 5, its power supply, case and card. It makes sense only if
  you want open-ended planning by a local model on the box, which this
  design otherwise borrows from a lender.
- **Buy instead of build, per target.** For plain remote console access, an
  off-the-shelf KVM-over-IP unit per computer is simpler than a tether. This
  build's value is the layer above: orders, gating, placement and records.
  Such a unit could later be added as another kind of hands and eyes.

## 9. Build plan

Each phase ends with checks that either pass or tell you what to fix.

### Phase 0: today, no purchases

1. On any Linux machine with Go 1.22 to 1.24: `make preflight test demo`.
2. Write your real policy: the targets, the commands you want available by
   voice, and which of them need confirmation. Keep it in git; a change to
   what the system may do is then a reviewed pull request.
3. Look at the Coral for an Ethernet jack, and put a 64-bit Raspberry Pi OS
   Lite on the Pi 3 B+.

### Phase 1: the chief and two targets ($340.90)

1. Raspberry Pi OS Lite (64-bit) on the Pi 5. Install `tesseract-ocr`,
   `poppler-utils`, `ffmpeg`, `alsa-utils`. Install `cos` (README).
2. Attach one tether to a machine you can afford to mistype on. Set the
   udev rules so the cables get stable names.
3. Install the Cactus runner and the Whistle and Needle models, checking
   their SHA-256 sums against the release manifest. Generate the tool
   schema with `cos tools`.

Checks:

- Hands: after `stty -F /dev/ukvm/hands-server1 9600 raw`, writing the six
  bytes `57 AB 00 01 00 03` returns a 14-byte reply starting `57 AB 00 81`.
- Eyes: `ffmpeg -f v4l2 -input_format mjpeg -i /dev/ukvm/eyes-server1
  -frames:v 1 f.png` then `tesseract f.png stdout --psm 6` prints what is on
  the screen.
- Dry run: `cos order "target server1; run uptime"` reports what it would
  send. Then `scripts/arm.sh on 5m` and the same order types the command
  and reads the result back.
- Speech: record ten of your own orders with `arecord`. Time the Cactus call
  on each and compare the returned calls with what you said. The target is
  under about 1.5 seconds from the end of speech to the order appearing on
  the panel. No vendor figure exists for Whistle on a Pi, so this
  measurement decides whether the 4 GB Pi 5 is enough or the speech step
  should move to a lender.

### Phase 2: the staff LAN, satellites and records ($121.00)

1. Staff LAN on the switch; the Pi 5 uplinks through the USB adapter.
2. `cos serve` on the Pi 3 B+ with `deploy/satellite-pi3.json` and tether 3;
   the same on the Coral with tether 4.
3. Arm switch, Screen Crab on the most sensitive target, one power brick,
   everything on one board.

Checks:

- An order for server3 runs on the Pi 3 B+ and its result comes back.
- Unplug the Pi 3 B+: `status` shows it DOWN within 45 seconds and an order
  for server3 fails with "no reachable tether" instead of hanging.
- `cos ledger-verify /var/lib/cos/audit.jsonl` passes; edit one character
  of a copy and it fails.
- Pull the Screen Crab's card and match a few stills to ledger entries by
  time.

### Phase 3: refinements ($83.00, each item optional)

- The front panel on the Pico i.MX7D, or the fallback display on the Pi 5.
- A Pi Zero 2 W pod (`deploy/gadget-pod.sh`) for a target that needs an
  installer image presented as a USB drive.
- Mirage for screen state that has no text, such as icons and progress.
- A wake word on the Coral's Edge TPU in place of push to talk.
- TLS with client certificates between nodes (the flags exist).
- The state directory exported over 9P, so jobs, runbooks and the ledger
  can be mounted and read as files from any machine.

## 10. The prototype

The archive `ukvm-0.1.0-prototype.tar.gz` holds the source, tests, example
configurations, systemd units, udev rules, scripts and this document. It
also carries prebuilt static binaries for arm64, 32-bit ARM and x86-64 in
`dist/` with their own `SHA256SUMS`. A second `SHA256SUMS` delivered beside
the archive covers the archive itself.

```
cmd/cos/            serve, order, listen, tools, pdf, ledger-verify
internal/hid/       HID usage table, US layout, chords, report layouts
internal/ch9329/    CH9329 frame encoder and reply parser
internal/order/     order grammar, policy validation, gate, Needle tool schema
internal/broker/    device registry, liveness, placement
internal/ledger/    hash-chained audit log
internal/intake/    utterance segmenter, Cactus call, OCR, PDF, on-screen locate
internal/staff/     the chief: jobs, dispatch, runbooks, API, front panel
deploy/  scripts/   configs, units, udev rules, pod script, preflight, arm, demo
```

`go.mod` has no requirements: every import is the standard library. The
state it keeps is plain files (one JSON file per job, one text file per
runbook, one line per ledger record), and its stages are processes joined
by pipes: `arecord | cos listen`, a PDF on standard input over SSH and text
back on standard output.

### What the demonstration shows

`make demo` starts the daemon with three simulated targets. Each target's
display is an image file read by the real Tesseract, and each keyboard cable
is a file that receives the bytes the real cable would carry. Abridged
output from the run on the build machine:

```
$ cos order 'target server3; run uptime'            (not armed)
j0002 done
  run_command{"command":"uptime"} on cos
    DRY-RUN: would send 14 key reports to server3

$ cos order 'run uptime'                            (armed)
j0003 done
    ops@server3:~$ uptime
    05:58:01 up 41 days, 3:12, 1 user, load average: 0.08, 0.03, 0.01
   first two frames on the cable (key 'u' down, then all keys up):
      57 ab 00 02 08 00 00 18 00 00 00 00 00 24 57 ab
      00 02 08 00 00 00 00 00 00 00 00 0c

$ cos order 'target bmc; click "Restart"'
j0004 awaiting-confirm (click_text changes the target)
   after CONFIRM:  clicked at 672,594 on bmc

$ cos order 'target locked; run uptime'
j0005 failed (the screen shows a login or password prompt; not typing a command into it)

$ cos order 'run rm -rf /'
j0006 refused (command "rm -rf /" is not on the allowlist)

$ cos order 'read pdf scan.pdf'                     (a picture of a page)
    BMC Recovery Procedure
    1. Hold the ID button for ten seconds. ...

$ cos ledger-verify state/audit.jsonl
ok: 37 records, head 7a00aa35...
```

`docs/panel.png` is the front panel rendered at 800×480 with an order
awaiting confirmation.

### What building it taught

- **OCR reads words well and digits badly.** On console text Tesseract
  returned `nvmeOn1lp2` for `nvme0n1p2` and `@` for several zeros. Screen
  reading is good enough to answer "is there a login prompt", "did the
  command print something" and "where is the Restart button". It is not
  good enough to trust a number. That is the practical reason the chief
  prefers a target's own shell when one is lent.
- **Enlarging frames can hurt.** Doubling a 720p dialog before OCR made
  Tesseract stop seeing the labels inside button outlines. Frames are now
  enlarged only when they are low-resolution modes.
- **A label can appear twice.** "Restart" matched both the button and the
  question above it. The locator now ranks an exact match above one that
  differs in case or punctuation, and refuses to click when two matches
  rank the same.

## 11. Open risks and unverified items

| Item | Status | What settles it |
|---|---|---|
| CH9329 on real hardware | Frames match the vendor's examples; no cable has been driven. The reply layout for the status query follows the protocol document and should be checked. | Phase 1 hands check |
| Frame grab from a capture dongle | The `ffmpeg` command is the default and is configurable; untested | Phase 1 eyes check |
| Cactus command line | The argument shape and JSON fields follow the earlier field guide, which itself flagged parts of the CLI as thinly documented | Run the installed runner once by hand and adjust `internal/intake/voice.go` |
| Whistle speed on a Pi 5 | No published figure | Phase 1 speech check |
| Mirage | Not integrated. The field guide reports it weak at reading large or non-Latin text, so text stays with Tesseract. | Phase 3 |
| Pico i.MX7D under Linux | One forum report has the LCD and touch working; not tried here | Phase 3, with the $40 fallback |
| Which Coral board | Unknown | Look for an Ethernet jack |
| Absolute pointer in firmware screens | Some firmware accepts only relative mice. The CH9329 supports both; the prototype sends absolute. | Try it on each target's setup screen |
| Keyboard layout | US only. A target set to another layout will mistype punctuation. | Add a layout table in `internal/hid` |
| Estimated prices | Sixteen lines are estimates | Check when ordering; $61.51 is unallocated |
| The panel has no login | By design it binds to loopback for the attached display | Put a reverse proxy with authentication in front before exposing it further |

## 12. Sources

Prices and board support
- Raspberry Pi 5 list prices by memory size, power supply and case prices, and N150 mini PC prices (October 2026): https://terminalbytes.com/raspberry-pi-5-vs-n100-mini-pc-2026/
- Raspberry Pi price list including the Zero 2 W: https://raspberry.tips/en/raspberrypi-infos/raspberry-pi-prices-lifecycle
- Boards that support USB gadget mode, and the advice to use a powered hub: https://www.raspberrypi.com/news/usb-gadget-mode-in-raspberry-pi-os-ssh-over-usb/
- MS2130 USB 3.0 capture dongles: https://www.cnx-software.com/2022/11/07/ms2130-based-4k-hdmi-to-usb-3-0-video-capture-dongle-sells-for-19/

Parts on hand
- Screen Crab product page and configuration: https://shop.hak5.org/products/screen-crab and https://docs.hak5.org/screen-crab/getting-started/configuring-the-screen-crab
- Screen Crab ports and power: https://lab401.com/products/hak5-screen-crab
- Coral Dev Board Mini specification: https://www.cnx-software.com/2020/10/17/buy-coral-dev-board-mini-sbc/
- Coral module memory options: https://www.allaboutcircuits.com/news/google-unveils-new-and-updated-coral-hardware-for-2020/
- PICO-PI-IMX7 kit: https://developer.technexion.com/docs/system-on-modules/pico/pico-imx7/development-kits/pico-pi-imx7
- Installing Linux on the Android Things kit, with TechNexion image names: https://gist.github.com/liquidx/fd1002ec870a7c13f04a0b8a44744246
- Display and touch on mainline 5.6: https://forum.digikey.com/t/pico-pi-imx7-using-later-kernels-since-5-6-x/11317

CH9329
- Protocol summary with worked checksums: https://sjmf.github.io/kvm-serial/CH9329_PROTO/
- WCH datasheet and serial protocol document (mirrored): https://git.imuslab.com/TC/RemdesKVM/raw/master/datasheets/ch9329.pdf

Speech and vision models
- The Cactus field guide prepared earlier in this project ("Cactus Compute Whistle, Needle and Mirage for Voice, Vision and PDF Driven KVM Automation"), and through it: https://huggingface.co/Cactus-Compute/cactus , https://cactuscompute.com/blog/whistle , https://cactuscompute.com/blog/mirage , https://github.com/cactus-compute/cactus/issues/190
