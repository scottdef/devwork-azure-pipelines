#!/usr/bin/env bash
# gadget-pod.sh: turn a Raspberry Pi Zero 2 W into a USB keyboard, absolute
# pointer and (optionally) a read-only USB drive for one target computer.
#
# This is the optional "pod" tether. Use it where a CH9329 cable is not
# enough: the pod can also present an installer image as a USB drive, which
# a serial HID bridge cannot. The pod runs `cos serve` as a satellite with
#   "hands": "hidg:/dev/hidg0,/dev/hidg1"
#
# Prerequisites on the Zero 2 W (/boot/firmware/config.txt and cmdline.txt):
#   dtoverlay=dwc2          and          modules-load=dwc2
# Plug the Zero's inner micro-USB port (marked USB, not PWR) into the target.
#
# NOT exercised on hardware in this prototype. The report descriptors are the
# kernel's documented boot keyboard and a common 3-button 15-bit absolute
# pointer; check both on each target, firmware setup screens especially.
set -euo pipefail

G=/sys/kernel/config/usb_gadget/ukvm
IMAGE="${1:-}"   # optional path to an .iso or .img to expose read-only

[[ $EUID -eq 0 ]] || { echo "run as root" >&2; exit 1; }
modprobe libcomposite
[[ -d /sys/kernel/config/usb_gadget ]] || { echo "configfs gadget support is missing" >&2; exit 1; }
UDC="$(ls /sys/class/udc | head -n1)"
[[ -n "$UDC" ]] || { echo "no USB device controller: is dtoverlay=dwc2 set?" >&2; exit 1; }

if [[ -d $G ]]; then            # idempotent: tear down a previous gadget first
  echo "" > "$G/UDC" 2>/dev/null || true
  find "$G/configs/c.1" -maxdepth 1 -type l -delete
  rmdir "$G"/configs/c.1/strings/0x409 "$G"/configs/c.1 "$G"/functions/* "$G"/strings/0x409 "$G" 2>/dev/null || true
fi

mkdir -p "$G" && cd "$G"
echo 0x1d6b > idVendor          # Linux Foundation
echo 0x0104 > idProduct         # Multifunction Composite Gadget
echo 0x0200 > bcdUSB
mkdir -p strings/0x409 configs/c.1/strings/0x409
echo "ukvm-$(cut -c1-8 /etc/machine-id)" > strings/0x409/serialnumber
echo "ukvm"                              > strings/0x409/manufacturer
echo "ukvm keyboard and pointer"         > strings/0x409/product
echo "kbd+abs"                           > configs/c.1/strings/0x409/configuration
echo 250 > configs/c.1/MaxPower

# Function 1: boot-protocol keyboard, 8-byte reports. Kept first so firmware
# that only looks at the first interface finds a keyboard.
mkdir -p functions/hid.kbd
echo 1 > functions/hid.kbd/protocol
echo 1 > functions/hid.kbd/subclass
echo 8 > functions/hid.kbd/report_length
printf '\x05\x01\x09\x06\xa1\x01\x05\x07\x19\xe0\x29\xe7\x15\x00\x25\x01\x75\x01\x95\x08\x81\x02\x95\x01\x75\x08\x81\x03\x95\x05\x75\x01\x05\x08\x19\x01\x29\x05\x91\x02\x95\x01\x75\x03\x91\x03\x95\x06\x75\x08\x15\x00\x25\x65\x05\x07\x19\x00\x29\x65\x81\x00\xc0' \
  > functions/hid.kbd/report_desc

# Function 2: absolute pointer, 5-byte reports [buttons, x16le, y16le], 0..32767.
mkdir -p functions/hid.abs
echo 0 > functions/hid.abs/protocol
echo 0 > functions/hid.abs/subclass
echo 5 > functions/hid.abs/report_length
printf '\x05\x01\x09\x02\xa1\x01\x09\x01\xa1\x00\x05\x09\x19\x01\x29\x03\x15\x00\x25\x01\x95\x03\x75\x01\x81\x02\x95\x01\x75\x05\x81\x03\x05\x01\x09\x30\x09\x31\x15\x00\x26\xff\x7f\x75\x10\x95\x02\x81\x02\xc0\xc0' \
  > functions/hid.abs/report_desc

ln -s "$G/functions/hid.kbd" configs/c.1/
ln -s "$G/functions/hid.abs" configs/c.1/

# Function 3 (optional): a read-only drive holding an installer or tools image.
if [[ -n "$IMAGE" ]]; then
  [[ -r "$IMAGE" ]] || { echo "cannot read $IMAGE" >&2; exit 1; }
  mkdir -p functions/mass_storage.usb0
  echo 1 > functions/mass_storage.usb0/stall
  echo 1 > functions/mass_storage.usb0/lun.0/ro
  echo 1 > functions/mass_storage.usb0/lun.0/removable
  [[ "$IMAGE" == *.iso ]] && echo 1 > functions/mass_storage.usb0/lun.0/cdrom
  echo "$IMAGE" > functions/mass_storage.usb0/lun.0/file
  ln -s "$G/functions/mass_storage.usb0" configs/c.1/
  sha256sum "$IMAGE"
fi

echo "$UDC" > UDC
chgrp cos /dev/hidg0 /dev/hidg1 2>/dev/null && chmod 0660 /dev/hidg0 /dev/hidg1 || true
echo "gadget up on $UDC: /dev/hidg0 (keyboard) /dev/hidg1 (pointer)${IMAGE:+ + drive $IMAGE}"
