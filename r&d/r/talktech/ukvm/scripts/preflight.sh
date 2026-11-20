#!/usr/bin/env bash
# preflight.sh: check that this machine can build cos, and report which
# runtime tools a node of the given role would be missing.
#
#   scripts/preflight.sh build        # a build machine or CI runner
#   scripts/preflight.sh chief        # the chief of staff (Pi 5)
#   scripts/preflight.sh satellite    # a tether-holding board
set -euo pipefail

role="${1:-build}"
fail=0
ok()   { printf '  ok    %s\n' "$*"; }
miss() { printf '  MISS  %s\n' "$*"; fail=1; }
warn() { printf '  warn  %s\n' "$*"; }
have() { command -v "$1" >/dev/null 2>&1; }

echo "preflight: $role"

if [[ $role == build ]]; then
  if have go; then
    v="$(go env GOVERSION)"; minor="${v#go1.}"; minor="${minor%%.*}"
    if (( minor >= 22 && minor <= 24 )); then ok "$v (project range: 1.22 to 1.24)"
    else miss "$v is outside the project range 1.22 to 1.24"; fi
  else miss "go toolchain"; fi
  for t in make sha256sum tar; do have "$t" && ok "$t" || miss "$t"; done
  for t in tesseract pdftotext pdfinfo pdftoppm; do
    have "$t" && ok "$t" || warn "$t absent: the real-converter tests will be skipped"
  done
else
  for t in tesseract pdftotext pdfinfo pdftoppm ffmpeg stty; do have "$t" && ok "$t" || miss "$t"; done
  [[ -r /etc/cos/cos.json ]] && ok "/etc/cos/cos.json" || miss "/etc/cos/cos.json"
  if [[ -r /etc/cos/env ]] && grep -Eq '^COS_TOKEN=.{24,}$' /etc/cos/env; then ok "COS_TOKEN (24+ chars)"
  else miss "/etc/cos/env with COS_TOKEN of at least 24 characters"; fi
  id cos >/dev/null 2>&1 && ok "user cos" || miss "user cos (useradd -r -G dialout,video,audio cos)"
  shopt -s nullglob
  hands=(/dev/ukvm/hands-*); eyes=(/dev/ukvm/eyes-*)
  (( ${#hands[@]} )) && ok "hands: ${hands[*]}" || warn "no /dev/ukvm/hands-* yet (udev rules + CH9329 cables)"
  (( ${#eyes[@]} ))  && ok "eyes:  ${eyes[*]}"  || warn "no /dev/ukvm/eyes-* yet (udev rules + capture dongles)"
  if [[ $role == chief ]]; then
    have ssh && ok ssh || miss "ssh (agentless lenders)"
    have arecord && ok arecord || miss "arecord (alsa-utils) for the microphone"
    [[ -x /opt/cactus/linux-arm64/cactus || -x /opt/cactus/linux-x86_64/cactus ]] \
      && ok "cactus runner" || warn "cactus runner not installed: written orders work, spoken ones return 501"
    [[ -r /etc/cos/tools.json ]] && ok "/etc/cos/tools.json" || warn "generate it: cos tools -config /etc/cos/cos.json > /etc/cos/tools.json"
  fi
fi

(( fail == 0 )) && echo "preflight passed" || { echo "preflight FAILED" >&2; exit 1; }
