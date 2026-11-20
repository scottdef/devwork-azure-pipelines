# cos: chief of staff for a voice-and-vision universal KVM

`cos` takes written or spoken orders, decides whether they may run, hands each
step to the device that should do it, and keeps an audit ledger. It drives
computers the way a person at the console would: a hardware keyboard and mouse
on a USB port, and a capture device on the display output. Nothing is
installed on the computers it operates.

One static Go binary, standard library only, Go 1.22 to 1.24.

Use it only on computers you own or administer.

## Try it without hardware

```sh
make preflight test     # unit and end-to-end tests
make demo               # the whole staff with simulated tethers
```

The demo needs `tesseract`, `poppler-utils` and `curl`. Each simulated
target's display is a fixture image read by the real Tesseract, and its
keyboard cable is a file that receives the exact CH9329 byte stream the real
cable would carry.

## Words used here

| Word | Meaning |
|---|---|
| principal | The person giving orders. |
| chief of staff | The node running `cos serve` that takes orders (a Raspberry Pi 5 in the reference build). |
| target | A computer being operated. |
| tether | The pair of cables on one target: *hands* (a CH9329 USB keyboard/mouse cable, or a USB gadget) and *eyes* (an HDMI capture device). |
| satellite | Another board running `cos serve` that holds tethers for targets the chief cannot reach by cable. |
| lender | Any reachable computer that offers resources over SSH (a shell, PDF conversion, OCR). No agent is installed on it. |
| armed | Actions are really performed. Not armed (the default), every action is gated, logged and described, and nothing is sent. |

## Orders

Written orders use a small grammar. Statements are separated by `;`, a
newline, or the word `then`.

```
target server3                 select a computer (also: switch to server3)
run uptime                     a command from the allowlist
press ctrl+alt+t               a chord from the allowlist
type "hello world"             dictated text (always asks first)
click 512 300                  position on a 0..1000 scale (always asks first)
click "Restart"                find a label on screen, then click it
read screen                    text currently on the target's display
read pdf manuals/bmc.pdf       PDF to text (scans go through OCR)
note bios.server3 = 2.19       file a fact;  recall bios.server3
runbook run nightly-check      a saved list of orders
status                         devices, jobs, mode
confirm / cancel               answer the order that is waiting
```

Spoken orders produce the same calls: one Cactus invocation loads Whistle
(speech to text) and Needle (text to tool calls) and returns both the
transcript and the calls. `cos tools` prints the tool schema for Needle from
the same policy the validator uses.

## What may run

Every order passes one gate (`internal/order/gate.go`):

| Order | Result |
|---|---|
| Not in the policy's vocabulary, or heard with confidence below 0.50 | refused |
| Changes a target (`type`, `click`, any command or chord marked `confirm`), or heard below 0.85 | waits for CONFIRM |
| Read-only or marked `auto`, heard clearly or written | runs |

An order left unconfirmed for five minutes expires. A spoken "confirm" is
refused unless `voice_confirm` is set. Text the system reads from screens and
documents is stored and displayed, and is never parsed as an order.

## Where work runs

Each job states what it needs; the broker (`internal/broker`) picks a
provider in this order:

1. a lender (a reachable computer offering the resource),
2. a staff board (a satellite),
3. the chief of staff's own OS, only when nobody else can do it.

`run` uses the target's own shell over SSH when it lends one and falls back
to typing on the tether and reading the screen when it does not.

## Layout

```
cmd/cos/            the binary: serve, order, listen, tools, pdf, ledger-verify
internal/hid/       HID usage table, US layout, chords, report layouts
internal/ch9329/    CH9329 serial frame encoder and reply parser
internal/order/     order grammar, policy validation, gate, Needle tool schema
internal/broker/    device registry, liveness, placement
internal/ledger/    hash-chained append-only audit log
internal/intake/    speech segmenting, Cactus call, OCR, PDF, on-screen locate
internal/staff/     the chief: jobs, dispatch, runbooks, API, front panel
deploy/             example configs, systemd units, udev rules, gadget pod script
scripts/            preflight, arm, demo, fixture generator
bom.csv             the parts list (make bom)
```

## Install on a node

```sh
make checksums                          # on the build machine: dist/cos-linux-*
scp -r dist/cos-linux-arm64 deploy scripts Makefile README.md pi5:ukvm/
                                        # then on the node, in ukvm/, as root:
useradd -r -G dialout,video,audio cos
make install BIN=./cos-linux-arm64      # binary, units, udev rules, example config
install -m 0600 /dev/null /etc/cos/env && echo "COS_TOKEN=$(openssl rand -hex 24)" > /etc/cos/env
$EDITOR /etc/cos/cos.json /etc/udev/rules.d/90-ukvm-tethers.rules
cos tools -config /etc/cos/cos.json > /etc/cos/tools.json
scripts/preflight.sh chief && systemctl enable --now cos cos-ears
scripts/arm.sh on 15m                   # until then everything is a dry run
```

Runtime tools on a node: `tesseract-ocr`, `poppler-utils`, `ffmpeg`,
`alsa-utils`, `openssh-client`, and the Cactus runner for speech.

## What has and has not been exercised

Exercised here (in tests and `make demo`): the order grammar, policy and
gate; placement and fallback; the confirmation, expiry and runbook-approval
flows; the ledger and its tamper detection; CH9329 frames against the
vendor's worked examples; OCR, scanned-PDF and locate-and-click paths with
the real Tesseract and poppler; the API, satellite hand-off and front panel.

Not exercised, because it needs the hardware or the engine: a real CH9329
cable, a real capture device (`ffmpeg` frame grab), the USB gadget pod, the
`arecord` microphone pipe, and the Cactus CLI (its argument shape follows the
project's field guide and should be checked against the runner you install).
