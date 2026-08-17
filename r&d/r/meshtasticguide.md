# ESP32 LoRa V3 Meshtastic Dev Kit — Comprehensive Guide

## Meshnology N32 Two-Node Kit

---

# Table of Contents

1. Kit Overview and Hardware Specs
2. Assembling the Nodes
3. Bootstrap: Firmware Installation
4. Setup and Configuration — Physical Device
5. Setup and Configuration — Remote Ubuntu Server
6. Basic Usage Guide
7. Advanced Usage Guide
8. CLI Cheatsheet
9. Use Case 1: Off-Grid Emergency Comms
10. Use Case 2: Environmental Sensor Network
11. Use Case 3: MQTT-Bridged Remote Monitoring

---

# 1. Kit Overview and Hardware Specs

## What You Get (Two-Node Kit)

The Meshnology N32 two-node kit ships with the following per node (×2):

- 1× ESP32-S3 LoRa V3 development board (Heltec WiFi LoRa 32 V3 compatible)
- 1× 863–928 MHz LoRa antenna (SMA or IPEX)
- 1× 3000 mAh 3.7V Li-ion battery with protection board
- 1× 3D-printed protective enclosure
- 1× USB Type-C data/charging cable

## Core Specifications

| Parameter | Value |
|---|---|
| **MCU** | ESP32-S3FN8, dual-core Xtensa LX7, up to 240 MHz |
| **LoRa Transceiver** | Semtech SX1262 |
| **Frequency Range** | 863–928 MHz (US/EU/ANZ regions) |
| **TX Power** | Up to 21 ±1 dBm |
| **RX Sensitivity** | −134 dBm @ SF12, BW=125 kHz |
| **Flash / SRAM** | 8 MB / 512 KB |
| **Wi-Fi** | 802.11 b/g/n |
| **Bluetooth** | BLE 5.0 |
| **Display** | 0.96″ OLED (128×64, I2C) |
| **USB** | Type-C (CP2102 USB-to-UART bridge) |
| **I2C Pins** | SDA: GPIO 41, SCL: GPIO 42 |
| **Board Dimensions** | 50.2 × 25.5 × 10.2 mm |
| **Battery** | 3.7V 3000 mAh Li-ion, JST 1.25 connector |

## What You Need (Not Included)

- A computer with Chrome or Edge browser (for web flasher) or Python 3 (for CLI)
- A smartphone with the Meshtastic app (Android or iOS)
- Optionally: an Ubuntu server with USB pass-through or network connectivity to the nodes

---

# 2. Assembling the Nodes

## Safety First

**CRITICAL: Never power on the radio without an antenna attached.** Transmitting without an antenna will damage or destroy the SX1262 LoRa transceiver. This damage is permanent and not covered by warranty.

## Step-by-Step Assembly

### Step 1 — Unbox and Inventory

Lay out both node kits. Confirm you have all components listed above for each node. Inspect the boards for shipping damage — look for bent pins, cracked solder joints, or loose components.

### Step 2 — Attach the Antenna

Locate the IPEX (U.FL) antenna connector on the ESP32 board — it is the small gold circular connector near the edge of the board, next to the SX1262 chip.

If your kit includes an IPEX-to-SMA pigtail:

1. Align the IPEX snap connector directly over the board's IPEX socket.
2. Press down firmly and evenly until you hear/feel a small click. Do not use excessive force or push at an angle.
3. Thread the SMA antenna onto the pigtail's SMA connector. Hand-tighten only — do not use pliers.

If your kit includes a direct IPEX antenna (wire with IPEX connector):

1. Snap the IPEX connector onto the board socket as described above.
2. Route the antenna wire so it extends away from the board and is not coiled over the ESP32 chip or display.

### Step 3 — Connect the Battery

The board has a JST 1.25mm 2-pin battery connector. The 3000 mAh battery ships with a matching JST plug.

1. Identify the polarity — red wire is positive (+), black wire is negative (−). The board is reverse-polarity protected, but always verify.
2. Gently insert the JST connector. It clicks into place. Do not force it.
3. The OLED display may briefly flash on when the battery is connected — this is normal.

**Charging note:** The board charges the battery over USB-C. Use a USB-A to USB-C cable for reliable charging. USB-C to USB-C cables may not initiate charging on V3 boards.

### Step 4 — Seat the Board in the Enclosure

1. Orient the board so the USB-C port aligns with the enclosure's port cutout.
2. Route the antenna cable through the antenna slot or hole.
3. Place the battery in the enclosure's battery bay (behind/beneath the board).
4. Snap or slide the enclosure lid closed.

### Step 5 — Repeat for Node 2

Assemble the second node identically.

### Step 6 — Verify USB Connectivity

Before flashing firmware, connect each node to your computer via the USB-C cable:

On Linux:

```bash
lsusb
# Look for: "Silicon Labs CP210x UART Bridge" or "QinHeng Electronics"

ls /dev/ttyUSB* /dev/ttyACM*
# You should see /dev/ttyUSB0 or /dev/ttyACM0
```

On macOS:

```bash
ls /dev/cu.usbserial-* /dev/cu.SLAB_USBtoUART*
```

On Windows: Check Device Manager → Ports (COM & LPT) for a new COM port.

If the device is not recognized, install the CP210x USB driver from Silicon Labs: https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers

---

# 3. Bootstrap: Firmware Installation

You have two paths: the web flasher (recommended for beginners) or the CLI script (recommended for headless/server deployments).

## Method A — Web Flasher (Recommended)

### Prerequisites

- Google Chrome or Microsoft Edge (other browsers lack WebSerial support)
- USB data cable connected to the node (charging-only cables will not work)
- CP210x driver installed (see Section 2, Step 6)

### Procedure

1. Navigate to https://flasher.meshtastic.org in Chrome or Edge.
2. Click **Connect** and select the serial port for your device from the browser prompt.
3. Select your device type: **Heltec WiFi LoRa 32 V3** (or `heltec-v3`).
4. Choose the firmware version — select the latest **Stable** release unless you need a specific feature from Alpha.
5. Click **Install** and wait for the erase + flash cycle to complete. This takes 1–3 minutes.
6. When the flasher reports success, the device will reboot and display the Meshtastic boot screen on its OLED.
7. Repeat for Node 2.

### Preserving Settings on Re-flash

If you are updating firmware on a node that is already configured:

1. Before flashing, export your config: `meshtastic --export-config > node-backup.yaml`
2. Flash the new firmware.
3. After reboot, restore the config: `meshtastic --configure node-backup.yaml`

## Method B — CLI Script (Linux / Ubuntu)

### Prerequisites

```bash
# Install Python 3 and pip
sudo apt-get update
sudo apt-get install -y python3 python3-pip

# Install esptool
pip3 install --upgrade esptool

# Verify device connectivity
esptool.py chip_id
# Expected output includes: Chip is ESP32-S3
```

### Download Firmware

```bash
# Create a working directory
mkdir -p ~/meshtastic && cd ~/meshtastic

# Download the latest stable release (check meshtastic.org/downloads for current version)
FIRMWARE_VERSION="2.6.3.xxxxxx"  # Replace with actual latest version
wget "https://github.com/meshtastic/firmware/releases/download/v${FIRMWARE_VERSION}/firmware-${FIRMWARE_VERSION}.zip"

# Extract
unzip "firmware-${FIRMWARE_VERSION}.zip" -d firmware
cd firmware
```

### Flash the Device

For a clean initial install (erases all settings):

```bash
chmod +x device-install.sh
./device-install.sh -f firmware-heltec-v3-*.bin
```

For a firmware update (preserves settings):

```bash
chmod +x device-update.sh
./device-update.sh -f firmware-heltec-v3-*-update.bin
```

**Important:** Use the correct firmware file for your board. The Heltec V3 firmware file contains `heltec-v3` in its name. Using the wrong firmware can brick the device.

### Verify the Flash

```bash
# Install the Meshtastic CLI
pip3 install --upgrade "meshtastic[cli]"

# Verify the node responds
meshtastic --info
```

You should see device info including the firmware version, node ID, and hardware model.

### Troubleshooting

**"Permission denied" on /dev/ttyUSB0:**

```bash
sudo usermod -a -G dialout $USER
# Log out and log back in for group membership to take effect
```

**esptool cannot detect the chip:**

1. Try a different USB cable — many cables are charge-only.
2. Hold the BOOT button on the board while plugging in USB, then release after 2 seconds.
3. Try a different USB port (USB 2.0 ports sometimes work better than USB 3.0).

**"externally-managed-environment" error with pip:**

```bash
# Use pipx instead
sudo apt install -y pipx
pipx install "meshtastic[cli]"
pipx ensurepath
# Open a new shell
```

---

# 4. Setup and Configuration — Physical Device

This section covers configuring your nodes directly, either through the on-device interface, the smartphone app, or a locally-connected CLI.

## Initial Configuration (Required)

After flashing, every node MUST have its region set before it will transmit. Without a region, the radio stays off.

### Using the Meshtastic App (Android / iOS)

1. Install the Meshtastic app from Google Play or the App Store.
2. Enable Bluetooth on your phone.
3. Open the app and tap **+** to add a device. Select your node from the BLE scan results (it appears as `Meshtastic_XXXX`).
4. Navigate to **Settings → LoRa** and set your **Region**:
   - `US` — United States (902–928 MHz)
   - `EU_868` — Europe (863–870 MHz)
   - `EU_433` — Europe (433 MHz)
   - `ANZ` — Australia / New Zealand (915–928 MHz)
   - `CN` — China (470–510 MHz)
   - See the full list in Section 8.
5. Tap **Save**. The node will reboot and begin advertising on the mesh.

### Using the CLI (USB Serial)

```bash
# Set region to US (change to your region)
meshtastic --set lora.region US

# Set a friendly name for the node
meshtastic --set-owner "Node-Alpha"
meshtastic --set-owner-short "NA"

# Verify configuration
meshtastic --info
```

### Using the Web Interface (ESP32 Only)

1. When unconfigured, the ESP32 creates a Wi-Fi access point named `meshtasticXXXX` with password `meshtastic`.
2. Connect your computer/phone to this Wi-Fi network.
3. Open a browser and navigate to http://meshtastic.local (or http://192.168.4.1 if mDNS does not work).
4. Configure the region and other settings through the web UI.
5. After setting Wi-Fi credentials, the node will connect to your local network instead of running as an AP.

## Device Configuration Essentials

### Setting Node Identity

```bash
# Node 1
meshtastic --set-owner "Base-Station" --set-owner-short "BS"

# Node 2 (connect second node)
meshtastic --set-owner "Field-Unit" --set-owner-short "FU"
```

### LoRa Radio Settings

```bash
# Set modem preset (controls range vs. speed trade-off)
meshtastic --set lora.modem_preset LONG_FAST    # Default, good balance

# Other presets (from fastest/shortest to slowest/longest range):
# SHORT_TURBO, SHORT_FAST, SHORT_SLOW, MEDIUM_FAST,
# MEDIUM_SLOW, LONG_FAST, LONG_MODERATE, LONG_SLOW, VERY_LONG_SLOW

# Set hop limit (how many times a message is relayed)
meshtastic --set lora.hop_limit 3    # Default, max is 7

# Set TX power (0 = max legal power for your region)
meshtastic --set lora.tx_power 0
```

### Channel Configuration

By default, all nodes share a primary channel (index 0) with a publicly known encryption key. This is fine for testing but insecure for any real use.

```bash
# Generate a random secure key for the primary channel
meshtastic --ch-set psk random --ch-index 0

# View the channel URL (contains the PSK) for sharing with other nodes
meshtastic --info
# Copy the "Complete URL" — it encodes all channel settings

# On the second node, apply the channel URL
meshtastic --seturl "https://meshtastic.org/e/#XXXXXX..."
```

### Wi-Fi Configuration (ESP32)

```bash
# Connect to a local Wi-Fi network
meshtastic --set network.wifi_ssid "YourSSID" \
           --set network.wifi_psk "YourPassword" \
           --set network.wifi_enabled true
```

### GPS / Position

The Heltec V3 does not have a built-in GPS. To set a fixed position:

```bash
# Set fixed coordinates (latitude, longitude, altitude in meters)
meshtastic --setlat 40.7128 --setlon -74.0060 --setalt 10
```

---

# 5. Setup and Configuration — Remote Ubuntu Server

This section covers managing Meshtastic nodes from a remote Ubuntu server — either via a USB-connected node forwarded over SSH, via the network (Wi-Fi/TCP), or via MQTT bridge.

## Method A — USB-Connected Node on the Server

If the node is physically plugged into the Ubuntu server via USB:

### Install the CLI

```bash
# On the Ubuntu server
sudo apt-get update
sudo apt-get install -y python3 python3-pip

# Option 1: pip install
pip3 install --upgrade "meshtastic[cli]"

# Option 2: pipx install (if pip complains about externally-managed env)
sudo apt install -y pipx
pipx install "meshtastic[cli]"
pipx ensurepath
source ~/.bashrc

# Option 3: Standalone binary (no Python needed)
wget https://github.com/meshtastic/python/releases/latest/download/meshtastic_ubuntu
chmod +x meshtastic_ubuntu
sudo mv meshtastic_ubuntu /usr/local/bin/meshtastic
```

### Configure Permissions

```bash
# Add your user to the dialout group
sudo usermod -a -G dialout $USER
newgrp dialout   # Apply immediately without logging out

# Verify the device is visible
ls -la /dev/ttyUSB0
meshtastic --info
```

### Manage the Node

```bash
# All standard CLI commands work over USB serial
meshtastic --set lora.region US
meshtastic --set-owner "Server-Node"
meshtastic --nodes           # List mesh members
meshtastic --info            # Full device info
meshtastic --export-config > server-node-config.yaml
```

## Method B — Network/TCP Connection (ESP32 Wi-Fi)

If the node is on the same network (or reachable via VPN/SSH tunnel):

### Enable the Network API on the Node

First, configure the node (via USB or app) to connect to Wi-Fi and enable API access:

```bash
# On the node (via USB)
meshtastic --set network.wifi_ssid "YourSSID" \
           --set network.wifi_psk "YourPassword" \
           --set network.wifi_enabled true
```

After the node connects to Wi-Fi, find its IP address from the OLED display or your router's DHCP table.

### Connect from the Remote Server

```bash
# Connect to the node over TCP
meshtastic --host 192.168.1.50 --info

# All CLI commands work with --host
meshtastic --host 192.168.1.50 --nodes
meshtastic --host 192.168.1.50 --sendtext "Hello from the server"
meshtastic --host 192.168.1.50 --set lora.modem_preset LONG_FAST
```

### SSH Tunnel for Remote Networks

If the node's Wi-Fi network is not directly reachable from your server:

```bash
# From your workstation or a jump host with access to the node's network
ssh -L 4403:192.168.1.50:4403 user@gateway-host

# Then from the server, connect to the forwarded port
meshtastic --host 127.0.0.1 --port 4403 --info
```

## Method C — Remote Administration Over the Mesh

Meshtastic (firmware 2.5+) supports administering remote nodes through the mesh itself, using public-key cryptography.

### Setup on the Admin Node (connected to server via USB)

```bash
# Get the admin node's public key
meshtastic --info
# Note the "Public Key" value (base64 string)
```

### Setup on the Remote Node (one-time, via USB)

```bash
# Paste the admin node's public key into the remote node's config
meshtastic --set security.admin_key "base64:YOUR_ADMIN_PUBLIC_KEY_HERE"

# You can add up to 3 admin keys
meshtastic --set security.admin_key "base64:KEY1" \
           --set security.admin_key "base64:KEY2"
```

### Issue Remote Commands from the Server

```bash
# Target the remote node by its ID
meshtastic --dest '!abcd1234' --get lora
meshtastic --dest '!abcd1234' --set device.role ROUTER
meshtastic --dest '!abcd1234' --set-owner "Remote-Relay"
```

**Warning:** Only `--set` and `--get` operations are supported remotely. Test all configuration changes on a directly-connected test node first — a bad config pushed over the mesh can disconnect the remote node permanently.

## Headless Server Automation

### Systemd Service for Monitoring

Create a service that logs all mesh traffic:

```bash
sudo tee /etc/systemd/system/meshtastic-monitor.service > /dev/null << 'EOF'
[Unit]
Description=Meshtastic Mesh Monitor
After=network.target

[Service]
Type=simple
User=meshuser
ExecStart=/usr/local/bin/meshtastic --port /dev/ttyUSB0 --listen
Restart=always
RestartSec=10
StandardOutput=append:/var/log/meshtastic/mesh.log
StandardError=append:/var/log/meshtastic/mesh-error.log

[Install]
WantedBy=multi-user.target
EOF

sudo mkdir -p /var/log/meshtastic
sudo chown meshuser:meshuser /var/log/meshtastic
sudo systemctl daemon-reload
sudo systemctl enable meshtastic-monitor
sudo systemctl start meshtastic-monitor
```

### Cron-Based Health Check Script

```bash
#!/usr/bin/env bash
# /usr/local/bin/meshtastic-health-check.sh
set -euo pipefail

LOG="/var/log/meshtastic/health.log"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

NODE_INFO=$(meshtastic --info 2>&1) || {
    echo "${TIMESTAMP} ERROR: Cannot reach node" >> "$LOG"
    exit 1
}

NODE_COUNT=$(meshtastic --nodes 2>&1 | grep -c "^|" || echo "0")
BATTERY=$(echo "$NODE_INFO" | grep -oP 'battery_level:\s*\K\d+' || echo "unknown")

echo "${TIMESTAMP} OK nodes_visible=${NODE_COUNT} battery=${BATTERY}%" >> "$LOG"
```

```bash
# Install the cron job (runs every 15 minutes)
chmod +x /usr/local/bin/meshtastic-health-check.sh
echo "*/15 * * * * meshuser /usr/local/bin/meshtastic-health-check.sh" | sudo tee /etc/cron.d/meshtastic-health
```

---

# 6. Basic Usage Guide

## Sending and Receiving Messages

### Via the Smartphone App

1. Open the Meshtastic app.
2. Connect to your node via Bluetooth.
3. Tap the **Messages** tab.
4. Select a channel (default: Primary) or a specific node for a direct message.
5. Type and send. Messages are relayed through all nodes in the mesh.

### Via the CLI

```bash
# Send a broadcast message on the primary channel
meshtastic --sendtext "Hello mesh!"

# Send a direct message to a specific node
meshtastic --sendtext "Private msg" --dest '!abcd1234'

# Send on a specific channel
meshtastic --sendtext "Team update" --ch-index 1
```

### Via the Web Interface

Navigate to your node's IP or http://meshtastic.local in a browser. The chat interface provides real-time messaging.

## Viewing the Mesh Network

```bash
# List all visible nodes with signal info
meshtastic --nodes

# Get detailed device information
meshtastic --info

# Export full configuration
meshtastic --export-config
```

## Understanding the OLED Display

The Heltec V3's 0.96″ OLED cycles through several screens:

- **Screen 1 — Device Info:** Node name, firmware version, battery level, channel utilization, and airtime.
- **Screen 2 — Node List:** Shows recently heard nodes with signal strength (SNR/RSSI).
- **Screen 3 — Messages:** Last received messages.
- **Screen 4 — GPS/Position:** Coordinates (if GPS is connected) or fixed position.

Press the **PROG** button (labeled "PRG" on some boards) to cycle through screens. Long-press to toggle the display on/off.

## Device Roles

Device roles control how a node behaves in the mesh:

```bash
meshtastic --set device.role CLIENT           # Default — normal user node
meshtastic --set device.role CLIENT_MUTE      # Receives only, does not relay
meshtastic --set device.role ROUTER           # Relays for others, reduced power use
meshtastic --set device.role ROUTER_CLIENT    # Relays AND acts as a user node
meshtastic --set device.role REPEATER         # Bare relay, no display/BLE/Wi-Fi
meshtastic --set device.role TRACKER          # Optimized for position reporting
meshtastic --set device.role SENSOR           # Optimized for telemetry reporting
meshtastic --set device.role CLIENT_HIDDEN    # Does not appear in node list
```

---

# 7. Advanced Usage Guide

## Multi-Channel Setup

Meshtastic supports 8 simultaneous channels (0–7). Use separate channels for different purposes:

```bash
# Primary channel (index 0) — general mesh comms
meshtastic --ch-set psk random --ch-index 0 --ch-set name "General"

# Secondary channel — team comms with a different key
meshtastic --ch-set psk random --ch-index 1 --ch-set name "TeamAlpha"

# Tertiary channel — sensor data
meshtastic --ch-set psk random --ch-index 2 --ch-set name "Sensors"

# Share the full config URL (encodes all channels + keys)
meshtastic --info
# Apply to other nodes:
meshtastic --seturl "https://meshtastic.org/e/#XXXXXX..."
```

**Rule:** Active channels must be consecutive. You cannot enable channel 3 without channels 1 and 2 being active.

## MQTT Bridge to the Internet

Bridge your local mesh to the internet via MQTT, enabling cross-internet mesh communication and integration with home automation systems.

### Prerequisites

- One node on Wi-Fi (the gateway node)
- An MQTT broker (use the public `mqtt.meshtastic.org` or self-host with Mosquitto)

### Configuration

```bash
# On the gateway node — enable MQTT
meshtastic --set mqtt.enabled true \
           --set mqtt.json_enabled true \
           --set mqtt.encryption_enabled true

# For the default Meshtastic MQTT server (no auth required):
# No additional server config needed

# For a custom MQTT broker:
meshtastic --set mqtt.address "mqtt.yourserver.com" \
           --set mqtt.username "meshuser" \
           --set mqtt.password "meshpass" \
           --set mqtt.tls_enabled true \
           --set mqtt.root "mymesh"

# Enable uplink/downlink on the channels you want to bridge
meshtastic --ch-set uplink_enabled true --ch-index 0
meshtastic --ch-set downlink_enabled true --ch-index 0
```

### Map Reporting

```bash
# Enable public map reporting (shows your node on meshmap.net)
meshtastic --set mqtt.map_reporting_enabled true \
           --set mqtt.map_publish_interval_secs 3600 \
           --set mqtt.map_position_precision 13
```

## Position Precision and Privacy

Control how much location detail you share:

```bash
# Full precision (exact location)
meshtastic --ch-set position_precision 32 --ch-index 0

# City-level precision (~3 km)
meshtastic --ch-set position_precision 13 --ch-index 0

# Disable position sharing entirely
meshtastic --ch-set position_precision 0 --ch-index 0
```

## Power Optimization

```bash
# Extend battery life — reduce Bluetooth advertising window
meshtastic --set power.wait_bluetooth_secs 60

# Enable power-saving mode (reduces wake frequency)
meshtastic --set power.is_power_saving true

# Set lite-sleep interval (seconds between mesh duty cycles)
meshtastic --set power.ls_secs 300

# For a solar-powered relay, use ROUTER role with power savings
meshtastic --set device.role ROUTER \
           --set power.is_power_saving true \
           --set power.ls_secs 600
```

## Remote Hardware (GPIO) Control

Control pins on a remote node over the mesh:

```bash
# Enable remote hardware on both nodes
meshtastic --set remote_hardware.enabled true

# Create a shared GPIO channel
meshtastic --ch-set name "gpio" --ch-index 1 --ch-set psk random

# Share the URL to both nodes
meshtastic --info   # copy the URL
meshtastic --seturl "https://meshtastic.org/e/#..."   # apply on remote node

# From the local node, toggle GPIO 4 HIGH on the remote node
meshtastic --gpio-wrb 4 1 --dest '!remoteid'

# Read GPIO 4 on the remote node
meshtastic --gpio-rd 0x10 --dest '!remoteid'

# Watch for GPIO 4 changes on the remote node
meshtastic --gpio-watch 0x10 --dest '!remoteid'
```

**Warning:** GPIO operations can physically damage hardware. Know your board's schematic before toggling pins. Never drive a pin that is already connected to an on-board peripheral.

## Serial Module

Pass serial data (UART) through the mesh:

```bash
# Enable serial module in text message mode
meshtastic --set serial.enabled true \
           --set serial.mode TEXTMSG \
           --set serial.baud 38400 \
           --set serial.rxd 35 \
           --set serial.txd 15
```

---

# 8. CLI Cheatsheet

## Connection Flags

```
--port /dev/ttyUSB0       Serial port (auto-detected if only one device)
--host 192.168.1.50       TCP/IP connection (ESP32 on Wi-Fi)
--ble Meshtastic_XXXX     Bluetooth LE connection
--ble-scan                Discover BLE devices
--dest '!abcd1234'        Target a specific remote node
--debug                   Enable verbose logging
--noproto                 Raw serial terminal (debugging)
```

## Information

```
meshtastic --info                    Full device info
meshtastic --nodes                   Node list with signal data
meshtastic --get all                 Dump all settings
meshtastic --export-config           Export config as YAML
meshtastic --export-config > f.yaml  Save config to file
```

## Configuration

```
meshtastic --set lora.region US                        Set region
meshtastic --set lora.modem_preset LONG_FAST           Set modem preset
meshtastic --set lora.hop_limit 3                      Set hop limit (1-7)
meshtastic --set lora.tx_power 0                       TX power (0=max legal)
meshtastic --set device.role CLIENT                    Set role
meshtastic --set-owner "NodeName"                      Set long name
meshtastic --set-owner-short "NN"                      Set short name (4 chars)
meshtastic --setlat 40.71 --setlon -74.00 --setalt 10  Fixed position
```

## Channels

```
meshtastic --ch-set psk random --ch-index 0           Random secure key
meshtastic --ch-set psk none --ch-index 0             No encryption
meshtastic --ch-set psk default --ch-index 0          Default (insecure) key
meshtastic --ch-set name "MyChan" --ch-index 1        Name a channel
meshtastic --ch-set uplink_enabled true --ch-index 0  Enable MQTT uplink
meshtastic --seturl "https://meshtastic.org/e/#..."   Import channel URL
```

## Wi-Fi (ESP32)

```
meshtastic --set network.wifi_ssid "SSID"             Set SSID
meshtastic --set network.wifi_psk "password"           Set password
meshtastic --set network.wifi_enabled true             Enable Wi-Fi
```

## MQTT

```
meshtastic --set mqtt.enabled true                     Enable MQTT
meshtastic --set mqtt.json_enabled true                Enable JSON output
meshtastic --set mqtt.encryption_enabled true          Encrypt MQTT traffic
meshtastic --set mqtt.address "broker.example.com"     Custom broker
meshtastic --set mqtt.username "user"                  Broker username
meshtastic --set mqtt.password "pass"                  Broker password
```

## Messaging

```
meshtastic --sendtext "Hello!"                         Broadcast message
meshtastic --sendtext "Hi" --dest '!abcd1234'          Direct message
meshtastic --sendtext "Team" --ch-index 1              Message on channel 1
meshtastic --listen                                    Monitor incoming msgs
```

## GPIO (Remote Hardware)

```
meshtastic --gpio-wrb 4 1 --dest '!id'               Set GPIO 4 HIGH
meshtastic --gpio-wrb 4 0 --dest '!id'               Set GPIO 4 LOW
meshtastic --gpio-rd 0x10 --dest '!id'                Read GPIO 4
meshtastic --gpio-watch 0x10 --dest '!id'             Monitor GPIO 4
```

## Backup & Restore

```
meshtastic --export-config > backup.yaml               Export config
meshtastic --configure backup.yaml                     Restore config
meshtastic --factory-reset                             Full factory reset
meshtastic --reboot                                    Reboot the node
```

## Device Roles Reference

```
CLIENT          Default user node
CLIENT_MUTE     Receive-only, no relay
CLIENT_HIDDEN   Hidden from node list
ROUTER          Relay-only, minimal power
ROUTER_CLIENT   Relay + user functionality
REPEATER        Bare relay, no BLE/Wi-Fi/display
TRACKER         Position-optimized
SENSOR          Telemetry-optimized
TAK             ATAK/CoT integration
TAK_TRACKER     ATAK tracker
LOST_AND_FOUND  Asset tracking
```

## LoRa Region Codes

```
US        902-928 MHz    United States
EU_868    863-870 MHz    Europe (868 MHz)
EU_433    433-434 MHz    Europe (433 MHz)
CN        470-510 MHz    China
JP        920-925 MHz    Japan
ANZ       915-928 MHz    Australia/New Zealand
KR        920-923 MHz    South Korea
TW        920-925 MHz    Taiwan
RU        868-870 MHz    Russia
IN        865-867 MHz    India
BR_902    902-907 MHz    Brazil
TH        920-925 MHz    Thailand
MY_919    919-924 MHz    Malaysia
SG_923    920-925 MHz    Singapore
PH_915    915-918 MHz    Philippines
LORA_24   2400 MHz       Worldwide (2.4 GHz)
```

## Modem Presets (Speed vs Range)

```
SHORT_TURBO       Fastest, ~2 km     21.88 kbps
SHORT_FAST        Fast, ~3 km        10.94 kbps
SHORT_SLOW        Medium, ~5 km      6.25 kbps
MEDIUM_FAST       Medium, ~8 km      3.52 kbps
MEDIUM_SLOW       Slower, ~12 km     1.95 kbps
LONG_FAST         Default, ~15 km    1.07 kbps  ← Default
LONG_MODERATE     Slow, ~20 km       0.34 kbps
LONG_SLOW         Very slow, ~30 km  0.18 kbps
VERY_LONG_SLOW    Slowest, ~40+ km   0.09 kbps
```

Range estimates assume clear line-of-sight with stock antennas.

---

# 9. Use Case 1: Off-Grid Emergency Communications

## Scenario

Two hikers carrying one node each on a backcountry trail. No cell service. They need to communicate position and text messages over 5–15 km while conserving battery for a 3-day trip.

## Hardware Config

- Both nodes use the kit's 3000 mAh batteries (fully charged before departure)
- Stock 915 MHz antennas
- Nodes carried in outer backpack pockets (better antenna orientation)

## Configuration — Node 1 ("Hiker Alpha")

```bash
# Set region
meshtastic --set lora.region US

# Set identity
meshtastic --set-owner "Hiker-Alpha" --set-owner-short "HA"

# Use LONG_FAST for good range with reasonable battery life
meshtastic --set lora.modem_preset LONG_FAST
meshtastic --set lora.hop_limit 3

# Set role — normal client
meshtastic --set device.role CLIENT

# Generate a secure channel key and share with the other hiker
meshtastic --ch-set psk random --ch-index 0 --ch-set name "Trail"
meshtastic --info   # Copy the channel URL

# Enable power savings
meshtastic --set power.is_power_saving true
meshtastic --set power.wait_bluetooth_secs 900
meshtastic --set power.ls_secs 300

# Disable Wi-Fi (not needed, saves power)
meshtastic --set network.wifi_enabled false

# Disable position sharing on the public channel for privacy
meshtastic --ch-set position_precision 14 --ch-index 0
```

## Configuration — Node 2 ("Hiker Bravo")

```bash
meshtastic --set lora.region US
meshtastic --set-owner "Hiker-Bravo" --set-owner-short "HB"
meshtastic --set lora.modem_preset LONG_FAST
meshtastic --set device.role CLIENT

# Import the shared channel from Hiker Alpha
meshtastic --seturl "https://meshtastic.org/e/#CHANNEL_URL_HERE"

# Same power settings
meshtastic --set power.is_power_saving true
meshtastic --set power.wait_bluetooth_secs 900
meshtastic --set power.ls_secs 300
meshtastic --set network.wifi_enabled false
```

## Usage on the Trail

Both hikers pair their phones via Bluetooth and use the Meshtastic app to send text messages and share GPS positions. With `LONG_FAST` and 3000 mAh batteries, expect 2–4 days of battery life depending on message frequency.

## Emergency SOS Pattern

If one hiker is in distress, send a repeating message using the CLI or app:

```bash
# From a phone-connected node via the app, or if SSH'd into a node:
meshtastic --sendtext "SOS - Hiker Alpha - GPS: 44.2735, -71.3035 - Injured ankle - Need help"
```

---

# 10. Use Case 2: Environmental Sensor Network

## Scenario

A remote weather station node transmits temperature, humidity, and barometric pressure readings over the mesh to a base station node connected to a server. The sensor node is solar-powered at a remote location; the base station is plugged in at home.

## Additional Hardware (Per Sensor Node)

- 1× BME280 sensor module (temperature/humidity/pressure, I2C, 3.3V or 5V)
- 4× female-to-female jumper wires
- Soldering iron + headers (if the Heltec V3 does not have pre-soldered headers)
- Optional: small solar panel (5V, 1W+) connected to the USB-C port for outdoor deployment

## Wiring the BME280 to the Heltec V3

```
BME280 Pin    →    Heltec V3 Pin
──────────         ──────────────
VCC (3.3V)    →    3.3V
GND           →    GND
SDA           →    GPIO 41 (I2C SDA)
SCL           →    GPIO 42 (I2C SCL)
```

**Important:** If using the 5V BME280 module variant, connect VCC to the 5V pin. However, the 5V variant will NOT work on battery power alone — it requires USB power. Use the 3.3V variant for battery/solar deployments.

## Configuration — Sensor Node

```bash
# Basic setup
meshtastic --set lora.region US
meshtastic --set-owner "Weather-Station" --set-owner-short "WX"

# Set role to SENSOR (optimized for telemetry)
meshtastic --set device.role SENSOR

# Enable environmental telemetry
meshtastic --set telemetry.environment_measurement_enabled true
meshtastic --set telemetry.environment_update_interval 300    # 5 minutes
meshtastic --set telemetry.environment_screen_enabled true

# Use LONG_FAST for reliable delivery over distance
meshtastic --set lora.modem_preset LONG_FAST

# Secure channel
meshtastic --ch-set psk random --ch-index 0 --ch-set name "WxData"
meshtastic --info   # Copy URL for base station

# Power settings for solar deployment
meshtastic --set power.is_power_saving true
meshtastic --set power.ls_secs 300

# Set fixed position (the sensor station doesn't move)
meshtastic --setlat 40.7484 --setlon -73.9856 --setalt 30
```

## Configuration — Base Station Node

```bash
meshtastic --set lora.region US
meshtastic --set-owner "Base-Station" --set-owner-short "HQ"
meshtastic --set device.role CLIENT

# Import the sensor channel
meshtastic --seturl "https://meshtastic.org/e/#CHANNEL_URL_HERE"

# Connect to Wi-Fi for MQTT bridge
meshtastic --set network.wifi_ssid "HomeWiFi" \
           --set network.wifi_psk "password123" \
           --set network.wifi_enabled true

# Enable MQTT to forward telemetry to a broker
meshtastic --set mqtt.enabled true \
           --set mqtt.json_enabled true \
           --set mqtt.encryption_enabled true \
           --set mqtt.address "mqtt.yourserver.com" \
           --set mqtt.username "meshuser" \
           --set mqtt.password "meshpass"

# Enable uplink on the sensor channel
meshtastic --ch-set uplink_enabled true --ch-index 0
```

## Python Script — Log Telemetry to CSV on the Server

```python
#!/usr/bin/env python3
"""meshtastic_wx_logger.py — Log environmental telemetry to CSV."""

import csv
import json
import sys
from datetime import datetime, timezone

import paho.mqtt.client as mqtt

BROKER = "mqtt.yourserver.com"
PORT = 1883
USERNAME = "meshuser"
PASSWORD = "meshpass"
TOPIC = "mymesh/2/json/LongFast/!+/telemetry"
CSV_FILE = "/var/log/meshtastic/weather_data.csv"


def on_connect(client, userdata, flags, rc):
    print(f"Connected to MQTT broker (rc={rc})")
    client.subscribe(TOPIC)


def on_message(client, userdata, msg):
    try:
        data = json.loads(msg.payload.decode())
        env = data.get("payload", {}).get("environment", {})
        if not env:
            return

        row = {
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "from": data.get("from", "unknown"),
            "temperature_c": env.get("temperature", ""),
            "humidity_pct": env.get("relative_humidity", ""),
            "pressure_hpa": env.get("barometric_pressure", ""),
        }

        file_exists = False
        try:
            with open(CSV_FILE, "r") as f:
                file_exists = True
        except FileNotFoundError:
            pass

        with open(CSV_FILE, "a", newline="") as f:
            writer = csv.DictWriter(f, fieldnames=row.keys())
            if not file_exists:
                writer.writeheader()
            writer.writerow(row)

        print(f"[{row['timestamp']}] T={row['temperature_c']}°C "
              f"H={row['humidity_pct']}% P={row['pressure_hpa']}hPa")

    except (json.JSONDecodeError, KeyError) as e:
        print(f"Parse error: {e}", file=sys.stderr)


def main():
    client = mqtt.Client()
    client.username_pw_set(USERNAME, PASSWORD)
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(BROKER, PORT, 60)
    print(f"Subscribing to {TOPIC} — logging to {CSV_FILE}")
    client.loop_forever()


if __name__ == "__main__":
    main()
```

### Run the Logger

```bash
pip3 install paho-mqtt --break-system-packages
python3 meshtastic_wx_logger.py
```

---

# 11. Use Case 3: MQTT-Bridged Remote Monitoring and Alerting

## Scenario

A two-node deployment where Node 1 is at a remote property (barn, cabin, gate) and Node 2 is at the main house connected to the internet. Node 1 monitors a GPIO-connected sensor (door switch, motion detector, water level float) and relays alerts through the mesh to Node 2, which bridges the alerts to an MQTT broker. A Python script on the server consumes the MQTT feed and sends notifications.

## Hardware Setup

- **Node 1 (Remote/Sensor):** ESP32 LoRa V3 + battery + magnetic reed switch connected between GPIO 4 and GND (with internal pull-up). Deployed at a remote barn door.
- **Node 2 (Gateway):** ESP32 LoRa V3 + USB power at the main house, connected to Wi-Fi.

## Wiring the Reed Switch (Node 1)

```
Reed Switch    →    Heltec V3
──────────          ──────────
Terminal 1     →    GPIO 4
Terminal 2     →    GND
```

No external resistor is needed — the ESP32's internal pull-up is enabled by the remote hardware module. When the door is closed, the switch is closed (GPIO reads LOW). When the door opens, the switch opens (GPIO reads HIGH).

## Configuration — Node 1 (Remote Sensor)

```bash
meshtastic --set lora.region US
meshtastic --set-owner "Barn-Monitor" --set-owner-short "BN"
meshtastic --set device.role SENSOR

# Enable remote hardware
meshtastic --set remote_hardware.enabled true

# Secure channel for GPIO operations
meshtastic --ch-set psk random --ch-index 0 --ch-set name "Property"

# Create a secondary channel for GPIO
meshtastic --ch-set name "gpio" --ch-index 1 --ch-set psk random

meshtastic --info   # Copy the URL

# Power savings (battery powered)
meshtastic --set power.is_power_saving true
meshtastic --set power.ls_secs 300

# Fixed position
meshtastic --setlat 42.3601 --setlon -71.0589 --setalt 5
```

## Configuration — Node 2 (Gateway)

```bash
meshtastic --set lora.region US
meshtastic --set-owner "House-Gateway" --set-owner-short "HG"
meshtastic --set device.role CLIENT

# Import channels from Node 1
meshtastic --seturl "https://meshtastic.org/e/#CHANNEL_URL_HERE"

# Enable remote hardware (to issue GPIO commands to Node 1)
meshtastic --set remote_hardware.enabled true

# Wi-Fi for internet bridge
meshtastic --set network.wifi_ssid "HomeWiFi" \
           --set network.wifi_psk "password123" \
           --set network.wifi_enabled true

# MQTT bridge
meshtastic --set mqtt.enabled true \
           --set mqtt.json_enabled true \
           --set mqtt.encryption_enabled true \
           --set mqtt.address "mqtt.yourserver.com" \
           --set mqtt.username "meshuser" \
           --set mqtt.password "meshpass" \
           --set mqtt.root "property"

# Enable uplink on both channels
meshtastic --ch-set uplink_enabled true --ch-index 0
meshtastic --ch-set uplink_enabled true --ch-index 1
```

## Monitoring Script — GPIO Watchdog with Alerts

```python
#!/usr/bin/env python3
"""barn_monitor.py — Watch a remote GPIO pin and alert on state change."""

import json
import smtplib
import sys
from datetime import datetime, timezone
from email.mime.text import MIMEText

import paho.mqtt.client as mqtt

# --- Configuration ---
MQTT_BROKER = "mqtt.yourserver.com"
MQTT_PORT = 1883
MQTT_USER = "meshuser"
MQTT_PASS = "meshpass"
MQTT_TOPIC = "property/2/json/#"

ALERT_EMAIL_TO = "you@example.com"
ALERT_EMAIL_FROM = "alerts@example.com"
SMTP_HOST = "smtp.example.com"
SMTP_PORT = 587
SMTP_USER = "alerts@example.com"
SMTP_PASS = "emailpassword"

# Barn node ID
BARN_NODE_ID = "!abcd1234"


def send_email_alert(subject, body):
    """Send an email alert."""
    msg = MIMEText(body)
    msg["Subject"] = subject
    msg["From"] = ALERT_EMAIL_FROM
    msg["To"] = ALERT_EMAIL_TO

    try:
        with smtplib.SMTP(SMTP_HOST, SMTP_PORT) as server:
            server.starttls()
            server.login(SMTP_USER, SMTP_PASS)
            server.send_message(msg)
        print(f"Alert email sent: {subject}")
    except Exception as e:
        print(f"Email send failed: {e}", file=sys.stderr)


def on_connect(client, userdata, flags, rc):
    print(f"Connected to MQTT (rc={rc})")
    client.subscribe(MQTT_TOPIC)


def on_message(client, userdata, msg):
    try:
        data = json.loads(msg.payload.decode())
        sender = data.get("from", "")
        payload = data.get("payload", {})

        # Check for text messages from the barn node
        text = payload.get("text", "")
        if text and str(sender) == BARN_NODE_ID.replace("!", ""):
            timestamp = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")
            print(f"[{timestamp}] Barn message: {text}")

            if "DOOR" in text.upper() or "ALERT" in text.upper():
                send_email_alert(
                    f"Barn Alert — {timestamp}",
                    f"Message from barn monitor:\n\n{text}\n\nTimestamp: {timestamp}"
                )

    except (json.JSONDecodeError, KeyError) as e:
        print(f"Parse error: {e}", file=sys.stderr)


def main():
    client = mqtt.Client()
    client.username_pw_set(MQTT_USER, MQTT_PASS)
    client.on_connect = on_connect
    client.on_message = on_message
    client.connect(MQTT_BROKER, MQTT_PORT, 60)
    print("Barn monitor active — watching for alerts...")
    client.loop_forever()


if __name__ == "__main__":
    main()
```

## Polling the GPIO from the Gateway Node

In addition to the MQTT-based monitoring, you can manually poll the barn door sensor:

```bash
# Check if the barn door is open (read GPIO 4 on the barn node)
meshtastic --host 192.168.1.50 --gpio-rd 0x10 --dest '!abcd1234'

# Set up continuous GPIO watch
meshtastic --host 192.168.1.50 --gpio-watch 0x10 --dest '!abcd1234'
```

## Systemd Service for the Monitor

```bash
sudo tee /etc/systemd/system/barn-monitor.service > /dev/null << 'EOF'
[Unit]
Description=Barn Door MQTT Monitor
After=network.target

[Service]
Type=simple
User=meshuser
ExecStart=/usr/bin/python3 /opt/meshtastic/barn_monitor.py
Restart=always
RestartSec=30
StandardOutput=append:/var/log/meshtastic/barn-monitor.log
StandardError=append:/var/log/meshtastic/barn-monitor-error.log

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable barn-monitor
sudo systemctl start barn-monitor
```

---

# Appendix A: Troubleshooting Quick Reference

| Problem | Likely Cause | Fix |
|---|---|---|
| Node not detected on USB | Charge-only cable | Use a data cable |
| Node not detected on USB | Missing CP210x driver | Install Silicon Labs CP210x driver |
| Permission denied on /dev/ttyUSB0 | User not in dialout group | `sudo usermod -a -G dialout $USER` then re-login |
| No messages between nodes | Different regions set | Set same region on both nodes |
| No messages between nodes | Different PSK keys | Use `--seturl` to sync channels |
| No messages between nodes | Antenna not attached | Attach antenna (TX is disabled without it on some firmware) |
| Very short range | Wrong modem preset | Try `LONG_FAST` or `LONG_SLOW` |
| Battery not charging | USB-C to USB-C cable | Use USB-A to USB-C cable |
| Web flasher not working | Wrong browser | Use Chrome or Edge |
| esptool can't find device | Device not in bootloader | Hold BOOT button while plugging in USB |
| "externally-managed-environment" | Modern Ubuntu pip restriction | Use `pipx install "meshtastic[cli]"` |
| MQTT not forwarding | Uplink not enabled on channel | `--ch-set uplink_enabled true --ch-index 0` |
| Remote admin commands fail | Admin key not set on remote | Set `security.admin_key` on remote node via USB |

# Appendix B: Useful Links

- Meshtastic Documentation: https://meshtastic.org/docs/
- Meshtastic Firmware Downloads: https://meshtastic.org/downloads/
- Meshtastic Web Flasher: https://flasher.meshtastic.org/
- Meshtastic Python CLI: https://meshtastic.org/docs/software/python/cli/
- Heltec V3 Hardware Docs: https://meshtastic.org/docs/hardware/devices/heltec-automation/lora32/
- Meshnology Store: https://meshnology.com/
- Silicon Labs CP210x Drivers: https://www.silabs.com/developers/usb-to-uart-bridge-vcp-drivers
- Meshtastic Community Map: https://meshmap.net/
