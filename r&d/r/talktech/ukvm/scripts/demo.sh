#!/usr/bin/env bash
# demo.sh: run the whole staff on one machine with simulated tethers.
#
# No hardware is needed. Each simulated target's "display" is a fixture
# image, read by the real Tesseract; its "keyboard cable" is a file that
# receives the exact CH9329 byte stream the real cable would carry.
set -euo pipefail

cd "$(dirname "$0")/.."
BIN="$PWD/dist/cos"
[[ -x $BIN ]] || { echo "build first: make build" >&2; exit 1; }
for t in tesseract pdftotext pdfinfo pdftoppm curl od; do
  command -v "$t" >/dev/null || { echo "demo needs $t" >&2; exit 1; }
done

W="$(mktemp -d)"
FIX="$PWD/internal/intake/testdata"
export COS_TOKEN="demo-token-demo-token-demo-token"
ADDR=127.0.0.1:18088
mkdir -p "$W/docs"
cp "$FIX/scan.pdf" "$FIX/text.pdf" "$W/docs/"

cat > "$W/cos.json" <<EOF
{
  "self": "cos",
  "doc_root": "$W/docs",
  "settle_ms": 10,
  "policy": {
    "targets": ["server3", "bmc", "locked"],
    "commands": {"uptime": "auto", "df -h /": "auto", "sudo reboot": "confirm"},
    "chords": {"ctrl+l": "auto", "ctrl+alt+t": "confirm"}
  },
  "nodes": [
    {"name": "cos", "tier": "self",
     "caps": ["ocr", "pdf", "hands:server3", "eyes:server3", "hands:bmc", "eyes:bmc", "hands:locked", "eyes:locked"]},
    {"name": "bigbox", "tier": "lender", "ssh": "ops@192.0.2.10", "caps": ["pdf", "ocr", "exec:server3"]}
  ],
  "tethers": {
    "server3": {"hands": "sim:$W/wire-server3.bin", "eyes": "file:$FIX/terminal.png"},
    "bmc":     {"hands": "sim:$W/wire-bmc.bin",     "eyes": "file:$FIX/dialog.png"},
    "locked":  {"hands": "sim:$W/wire-locked.bin",  "eyes": "file:$FIX/login.png"}
  }
}
EOF

"$BIN" serve -config "$W/cos.json" -listen "$ADDR" -panel 127.0.0.1:18089 \
       -state "$W/state" -armed-file "$W/armed" 2>"$W/daemon.log" &
PID=$!
trap 'kill $PID 2>/dev/null || true; rm -rf "$W"' EXIT
for _ in $(seq 50); do curl -fsS -o /dev/null -H "Authorization: Bearer $COS_TOKEN" "http://$ADDR/v1/status" 2>/dev/null && break; sleep 0.1; done

order() { echo; echo "\$ cos order '$*'"; "$BIN" order -addr "http://$ADDR" "$@" || true; }
api()   { curl -fsS -X "$1" -H "Authorization: Bearer $COS_TOKEN" "http://$ADDR$2" ${3:+--data-binary "$3"}; }
say()   { echo; echo "== $*"; }

say "1. A report. The lender 'bigbox' is configured but unreachable, so it shows DOWN."
order status

say "2. Not armed: the order is gated, logged and described, and nothing is sent."
order "target server3; run uptime"
echo "   bytes on server3's cable so far: $(stat -c %s "$W/wire-server3.bin" 2>/dev/null || echo 0)"

say "3. Armed. bigbox would be asked first (it lends a shell on server3) but it is down,"
echo "   so the keyboard cable is used and the answer is read back off the screen."
touch "$W/armed"
order "run uptime"
echo "   first two frames on the cable (key 'u' down, then all keys up):"
head -c 28 "$W/wire-server3.bin" | od -An -tx1 | sed 's/^/     /'

say "4. A click on a button, found by its label. Clicks always wait for confirmation."
order 'target bmc; click "Restart"'
JOB="$(api GET /v1/jobs | grep -o '"id":"j[0-9]*"' | head -1 | cut -d'"' -f4)"
echo "   confirming $JOB the way the panel's CONFIRM button does:"
api POST "/v1/jobs/$JOB/confirm" | grep -o '"output":"[^"]*"' | tail -1 | sed 's/^/     /'
echo "   pointer frames on the bmc cable (move, press, release):"
od -An -tx1 "$W/wire-bmc.bin" | sed 's/^/     /'

say "5. A login prompt is on screen: the staff will not type a command into it."
order "target locked; run uptime"

say "6. Not in the vocabulary: refused before anything else happens."
order "run rm -rf /"

say "7. A scanned PDF (a picture of a page) becomes text; a text PDF goes straight through."
order "read pdf scan.pdf"
order "read pdf text.pdf"

say "8. A runbook: saved, refused until approved, then runs unattended."
api PUT /v1/runbooks/morning-check "target server3; run uptime; run df -h /"; echo
order "runbook run morning-check"
order cancel
api POST /v1/runbooks/morning-check/approve; echo
order "runbook run morning-check"

say "9. Tracking: a fact filed and recalled, and the closing report."
order "note firmware.server3 = 2.19.1 checked today"
order "recall firmware.server3"
order status

say "10. The audit ledger verifies end to end."
"$BIN" ledger-verify "$W/state/audit.jsonl"
echo "    kinds of record: $(grep -o '"kind":"[a-z-]*"' "$W/state/audit.jsonl" | sort | uniq -c | tr -s ' ' | tr '\n' ';')"

if [[ -n "${DEMO_KEEP:-}" ]]; then
  cp -r "$W/state" "$DEMO_KEEP"
  curl -fsS "http://127.0.0.1:18089/" > "$DEMO_KEEP/panel.html"
fi
if [[ -n "${DEMO_HOLD:-}" ]]; then echo; echo "panel: http://127.0.0.1:18089/  (Ctrl-C to stop)"; wait $PID; fi
