#!/usr/bin/env python3
"""Regenerate the image and PDF fixtures used by the intake tests.

    uv run --with pillow scripts/mkfixtures.py internal/intake/testdata

The fixtures are committed, so this only needs running when they change.
"""
import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

out = Path(sys.argv[1] if len(sys.argv) > 1 else "internal/intake/testdata")
out.mkdir(parents=True, exist_ok=True)
MONO = "/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf"
SANS = "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"

# 1. A console the way a capture dongle sees it: light text on black, 720p.
term = Image.new("RGB", (1280, 720), (0, 0, 0))
d = ImageDraw.Draw(term)
f = ImageFont.truetype(MONO, 22)
lines = [
    "ops@server3:~$ uptime",
    " 05:58:01 up 41 days,  3:12,  1 user,  load average: 0.08, 0.03, 0.01",
    "ops@server3:~$ df -h /",
    "Filesystem      Size  Used Avail Use% Mounted on",
    "/dev/nvme0n1p2  234G   87G  136G  40% /",
    "ops@server3:~$",
]
for i, line in enumerate(lines):
    d.text((12, 12 + i * 30), line, font=f, fill=(210, 210, 210))
term.save(out / "terminal.png")

# 2. A login prompt: the staff must refuse to type commands into this.
login = Image.new("RGB", (1280, 720), (0, 0, 0))
d = ImageDraw.Draw(login)
for i, line in enumerate(["Ubuntu 24.04.3 LTS server3 tty1", "", "server3 login:"]):
    d.text((12, 12 + i * 30), line, font=f, fill=(210, 210, 210))
login.save(out / "login.png")

# 3. A dialog with buttons, for locate-and-click.
dlg = Image.new("RGB", (1280, 720), (236, 236, 236))
d = ImageDraw.Draw(dlg)
big = ImageFont.truetype(SANS, 28)
d.text((400, 220), "Apply firmware settings and restart?", font=big, fill=(20, 20, 20))
for label, x in (("Cancel", 420), ("Restart", 760)):
    d.rectangle((x, 400, x + 200, 460), outline=(60, 60, 60), width=2, fill=(250, 250, 250))
    tw = d.textlength(label, font=big)
    d.text((x + 100 - tw / 2, 412), label, font=big, fill=(20, 20, 20))
dlg.save(out / "dialog.png")

# 4. A scanned page: a PDF holding only a picture of text (no text layer).
page = Image.new("RGB", (1240, 1754), (255, 255, 255))
d = ImageDraw.Draw(page)
body = ImageFont.truetype(SANS, 36)
for i, line in enumerate([
    "BMC Recovery Procedure",
    "",
    "1. Hold the ID button for ten seconds.",
    "2. Wait for the amber light to blink twice.",
    "3. Connect to port 623 and reset the password.",
]):
    d.text((120, 160 + i * 70), line, font=body, fill=(0, 0, 0))
page.save(out / "scan.pdf", "PDF", resolution=150.0)

# 5. A two-page PDF with a real text layer, written by hand so it needs no
#    PDF library: the smallest file pdftotext will read.
def text_pdf(pages):
    objs = ["<< /Type /Catalog /Pages 2 0 R >>", None,
            "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"]
    kids = []
    for lines in pages:
        stream = "BT /F1 14 Tf 72 720 Td 18 TL " + " ".join(
            "(%s) Tj T*" % l.replace("(", r"\(").replace(")", r"\)") for l in lines) + " ET"
        objs.append("<< /Length %d >>\nstream\n%s\nendstream" % (len(stream), stream))
        content = len(objs)
        objs.append("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] "
                    "/Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>" % content)
        kids.append(len(objs))
    objs[1] = "<< /Type /Pages /Kids [%s] /Count %d >>" % (
        " ".join("%d 0 R" % k for k in kids), len(kids))
    buf, offsets = b"%PDF-1.4\n", []
    for i, o in enumerate(objs, 1):
        offsets.append(len(buf))
        buf += ("%d 0 obj\n%s\nendobj\n" % (i, o)).encode()
    xref = len(buf)
    buf += ("xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1)).encode()
    for off in offsets:
        buf += ("%010d 00000 n \n" % off).encode()
    buf += ("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, xref)).encode()
    return buf

(out / "text.pdf").write_bytes(text_pdf([
    ["Rack B Power Sequence", "Step one: confirm PDU channel 4 is live.",
     "Step two: power on the storage shelf before the head unit."],
    ["Escalation", "Call the on-duty operator if the shelf does not report in five minutes."],
]))
print("fixtures written to", out)
