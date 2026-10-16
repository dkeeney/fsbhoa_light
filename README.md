# FSBHOA Lighting Control

Schedule-driven control of outdoor and amenity lighting for a homeowners' association, built on **CLICK PLCs**, a small **Go** service and a **WordPress** admin UI.

It runs the lights at the Lodge and Cabana of the **Four Seasons at Bakersfield Homeowners Association** ([FSBHOA](https://fsbhoa.com)) in Bakersfield, California: 40 lighting circuits in 17 zones across two sites, covering parking lots, walkways, the pool, courts and buildings. It has been in daily use since late 2025.

If your HOA (or school, park or church) wants lights that follow sunset automatically, can be overridden from a browser, and keep working when the server is down, this may be a useful starting point. It's open source (GPL-3.0), but it was built for one site, so expect to adapt it. See [Adapting it to your site](#adapting-it-to-your-site).

---

## Features

- **Zones and schedules from a browser.** Group relays into zones (e.g. "Parking Lot North") and give each zone a weekly schedule.
- **Flexible schedules.** Up to 12 schedules, each with up to 14 time spans. Each span picks its days of the week, and its start and end can each be a fixed time, **sunset/sunrise** (calculated daily from your latitude and longitude) or the **photocell**.
- **The PLCs run the lights by themselves.** Schedules are stored in the PLCs' retentive memory. If the server, WordPress or the network goes down, the lights keep following their schedules.
- **Manual override** of any zone from the status monitor, and per-output test buttons for commissioning.
- **Live status monitor**, as a list and as a map with zones drawn over a site plan.
- **Optional "lights on" QR codes** for courts and other amenities. A resident scans a code at the court and the lights come on for a set time (default 90 minutes), but only during allowed hours. Optionally the scan is approved only if that resident recently badged through the gate (needs the companion access control system).
- **Latching relays, sequenced pulses.** Each light circuit is a latching relay driven through an H-bridge. The PLC pulses one output pair at a time (250 ms on, 100 ms dead time) so a modest power supply can drive many relays.
- **PLC emulator for testing.** `softPLC` runs the actual ladder logic file on a PC, so you can test changes without touching the hardware.

---

## How it works

```
                 ┌──────────────────────────────┐
 Admin browser ─►│ WordPress (access server)     │
                 │  fsbhoa_lighting plugin       │  zones, mappings, schedules (MySQL)
                 │  config UI · monitors · REST  │
                 └──────────────┬───────────────┘
                     REST + API key (/full-config, /sync, /status …)
                 ┌──────────────▼───────────────┐
                 │ lighting-service (Go)         │  sunrise/sunset · schedule packing
                 │ systemd · port 8085           │  overrides · status · clock sync
                 └───────┬───────────────┬──────┘
                Modbus TCP               │ long-poll (optional QR feature)
          ┌──────────────▼─┐   ┌─────────▼────────────────────┐
          │ CLICK PLC #1    │   │ Public HOA website            │
          │ "Lodge" master  │   │  fsbhoa-remote-lighting plugin│◄── resident scans
          │ 24 light pairs  │   │  QR page + job queue          │    a QR code
          └───────┬────────┘   └───────────────────────────────┘
       Modbus RECV│ photocell state (every 1 s)
          ┌───────▼────────┐
          │ CLICK PLC #2    │  photocell on X001
          │ "Cabana" slave  │
          │ 16 light pairs  │
          └────────────────┘
```

1. **WordPress plugin** (`fsbhoa_lighting.php`, `includes/`, `assets/js/`): the admin UI and the source of truth. It adds a **Settings → Lighting Control** page and four shortcodes for front-end pages. It also writes the Go service's config file, `/var/lib/fsbhoa/lighting_service.json`.
2. **Go service** (`lighting-service/`): the bridge to the PLCs.
   - **On a save in the UI:** it fetches the full config from WordPress, converts sunset/sunrise to clock times, packs the schedules into PLC registers, and tells the PLCs to re-apply their schedules.
   - **Every night at 3 AM** (and at startup): it re-syncs the PLC clocks (daylight saving time) and refreshes today's sunrise and sunset times, without disturbing any manual overrides.
3. **PLC ladder logic** (`LadderLogicForAI.txt`): a hand-written text rendering of the CLICK program, which you enter in the free CLICK programming software.
   - **Main loop:** sequences the relay pulses.
   - **`SCHED_ENGINE`:** decides whether each schedule is ON, OFF or "QR-enabled" right now.
   - **`ACTION_ENGINE`:** turns schedule changes and QR timers into ON/OFF requests.
   - **Both PLCs:** run the same program. The Lodge acts as Modbus master and reads the photocell from the Cabana PLC.
4. **Remote plugin** (`remote-plugin/`, optional): installed on your **public** website. It hosts the QR-code landing page and a small job queue. The Go service long-polls it, so the lighting server never has to accept connections from the internet.
5. **softPLC** (`softPLC/`): a Python CLICK emulator (Modbus TCP server plus web dashboard) that parses and runs `LadderLogicForAI.txt`.

---

## Is it a fit for your site?

It assumes roughly this hardware:

| Item | What we use / what the code assumes |
|---|---|
| PLCs | 1–2 **AutomationDirect CLICK PLUS** CPUs with Ethernet/Wi-Fi (Modbus TCP server). The program uses `RECV` over the network between the two PLCs. |
| Outputs | 16-point DC output modules. Each light uses an **output pair**: odd Y = ON pulse, even Y = OFF pulse. Up to 3 modules = 24 lights per PLC. |
| Relays | **Latching** relays driven through H-bridge driver boards. One driver can switch several relays at once. |
| Photocell | Optional. A dry contact on `X001` of the second PLC. |
| Server | Linux (Debian, Ubuntu or Raspberry Pi OS) with Apache, PHP 8, MariaDB/MySQL, WordPress and Go 1.24+. A Raspberry Pi is plenty. |
| Network | The server and PLCs on the same LAN. Static IPs for the PLCs. |

**Limits that come from the PLC program:** 12 schedules, 14 spans per schedule, 24 lights on the master and 16 on the slave, and two PLCs. Times have one-minute resolution.

If you use plain (non-latching) contactors instead, the main loop's pulse logic would need to change to hold outputs on. Everything above it (zones, schedules, sunset handling, UI) would still apply.

---

## Repository layout

```
fsbhoa_lighting.php        WordPress plugin entry (tables, script loading)
includes/                  REST routes, settings page, shortcode views
assets/js/                 Config UI, map editor, status monitors
lighting-service/          Go service (Modbus client, HTTP API, QR poller)
LadderLogicForAI.txt       The PLC program as text (current)
OrigLadderLogicForAI.txt   Older snapshot, for reference
softPLC/                   CLICK PLC emulator + configs for two simulated PLCs
remote-plugin/             Optional QR-code plugin for your public website
install.sh                 Fresh-server setup script (Apache, MariaDB, PHP, Go, WordPress)
backup.sh                  Nightly database + uploads backup (cron)
TODO.md                    Known follow-ups
```

---

## Installation

### 1. Server

On a fresh Debian or Raspberry Pi OS machine, `install.sh` installs Apache, MariaDB, PHP, Go and WordPress, clones this repo, links it into `wp-content/plugins/` and creates the systemd unit. **Edit the variables at the top first** (`APP_USER`, `REPO_URL`), then run:

```bash
sudo ./install.sh
```

Then follow the steps it prints: secure MariaDB, create the database, and finish the WordPress installer in a browser.

Prefer to do it by hand? You need a working WordPress install, then:

```bash
git clone https://github.com/dkeeney/fsbhoa_light.git ~/fsbhoa_light
sudo ln -s ~/fsbhoa_light /var/www/html/wp-content/plugins/fsbhoa_lighting
```

The plugin writes the Go service's config to `/var/lib/fsbhoa/`, so make that folder writable by the web server:

```bash
sudo mkdir -p /var/lib/fsbhoa && sudo chown www-data:www-data /var/lib/fsbhoa
```

### 2. WordPress plugin

1. Activate **FSBHOA Lighting Control** under *Plugins*. Activation creates the tables and checks for the PHP extensions `mysqli`, `json`, `imagick`, `gd` and `intl`.
2. Go to **Settings → Lighting Control** and fill in:
   - **Go service port** (default `8085`) and log file path
   - **Latitude / Longitude**, used for sunset and sunrise
   - **PLC #1 / PLC #2 addresses**, e.g. `192.168.1.60:502`
   - **Lighting API key**: click *Generate New Key*. The Go service uses it to read the configuration.
   - **Map image**, used by the map monitor
   - *(optional)* the QR / remote site and Access Control settings (see [QR codes](#optional-qr-code-lights-on))
3. Click **Save Settings**. This writes `/var/lib/fsbhoa/lighting_service.json`.
4. Create pages and add one shortcode to each:

   | Shortcode | Page |
   |---|---|
   | `[lighting_configuration]` | Zones, PLC output mappings, map placement |
   | `[lighting_schedules]` | Schedules and spans |
   | `[lighting_status_monitor]` | Live list with override buttons (use the page slug `lighting-monitor`) |
   | `[lighting_map_monitor]` | Live site map |

   All four pages are **admin-only**. Keep the whole site behind a login. Our access control plugin forces login site-wide; a "force login" plugin does the same job.

### 3. Go service

```bash
cd ~/fsbhoa_light/lighting-service
./build.sh                # go mod tidy && go build
```

`install.sh` creates the service unit for you. If you're setting up by hand, create `/etc/systemd/system/fsbhoa-lighting.service`:

```ini
[Unit]
Description=FSBHOA Lighting Control Service
After=network.target mariadb.service

[Service]
Type=simple
User=pi
Group=pi
WorkingDirectory=/home/pi/fsbhoa_light/lighting-service
ExecStart=/home/pi/fsbhoa_light/lighting-service/lighting-service
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Change `User`, `Group` and the two paths to match your server. systemd doesn't allow `#` comments at the end of a line in unit files.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fsbhoa-lighting.service
sudo journalctl -u fsbhoa-lighting.service -n 50 --no-pager
```

The service reads `/var/lib/fsbhoa/lighting_service.json`, or a path given as its first argument.

**Restart button (optional).** The settings page can start, stop and restart the service. To allow that, let the web server user run those four commands only. Create the file with `sudo visudo -f /etc/sudoers.d/www-data-lighting` and add:

```
www-data ALL=(ALL) NOPASSWD: /bin/systemctl restart fsbhoa-lighting.service
www-data ALL=(ALL) NOPASSWD: /bin/systemctl status fsbhoa-lighting.service
www-data ALL=(ALL) NOPASSWD: /bin/systemctl start fsbhoa-lighting.service
www-data ALL=(ALL) NOPASSWD: /bin/systemctl stop fsbhoa-lighting.service
```

**Log rotation.** The service appends to its log file forever. Add `/etc/logrotate.d/fsbhoa-lighting`:

```
/home/pi/fsbhoa_light/lighting-service/lighting-service.log {
    su pi pi
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    copytruncate
}
```

**Firewall.** Port 8085 only needs to be reachable from the WordPress server itself (and, if you like, from monitor screens on your LAN). Don't expose it to the internet.

### 4. PLC program

1. Enter `LadderLogicForAI.txt` into the **CLICK Programming Software**: the main program plus the `SCHED_ENGINE`, `ACTION_ENGINE` and `CLEAR_MEM` subroutines. The `## Memory Map` section at the end documents every register.
2. **Master/slave.** Load the same program into both PLCs, with one difference on **Rung 3**: the master (with 24 lights) *sets* `C153`; the slave (with 16 lights and the photocell) *resets* it.
3. **Static IPs.** The slave's IP is hard-coded in the master's `RECV` instruction (Rung 8, `192.168.42.61` in our install). Change it to match your network.
4. In the CLICK software, mark **DS99–DS1112 as retentive**, so schedules survive a power cycle.
5. Set a strong PLC password. Never commit it.

### 5. Configure your lights

1. **Configuration page:** create a **mapping** for each output pair. Give it the PLC, the ON output (e.g. `Y101`), and the relays it drives (for your records). Enter `---` as the output for spare relays.
2. Create **zones** and attach mappings to them. Optionally place them on the map.
3. **Schedules page:** create schedules and assign each zone a schedule. Saving pushes everything to the PLCs.
4. **Status monitor:** check that each zone reports its state and responds to the ON/OFF override buttons.

---

## Optional: QR code "lights on"

This lets residents turn on court or amenity lights by scanning a QR code, without giving anyone access to the lighting server.

1. **On your public WordPress site** (the HOA's website, not the lighting server):
   - Build the plugin zip:
     ```bash
     remote-plugin/deploy.sh      # builds remote-plugin/fsbhoa-remote-lighting.zip
     ```
   - Upload it under *Plugins → Add New → Upload Plugin* and activate it.
   - Create a page with the slug `lights` containing the shortcode `[court_lights]`.
   - Add a key to that site's `wp-config.php`:
     ```php
     define( 'FSBHOA_LIGHTING_API_KEY', '<paste output of: openssl rand -base64 32>' );
     ```
     There's no fallback key. Without this line, the plugin refuses every request.
2. **On the lighting server**, in *Settings → Lighting Control*:
   - Set the **remote URL** (e.g. `https://your-hoa.org/wp-json/lights/v1/`) and the same **API key**.
   - Set the **QR base URL** (e.g. `https://your-hoa.org/lights`) and the **QR duration** (minutes).
3. Generate QR codes from the configuration page and post them at each court.

**How a scan is approved:**
- **Default:** the resident must be logged in to the public website. The Go service then asks the access control system whether that resident badged in recently. The scan is also checked against the zone's schedule: it's only accepted if the zone has a QR-enabled span (a span whose start trigger is QR) covering part of the requested time. Scans are accepted up to an hour before sunset.
- **Public QR mode** (a setting): no website login and no badge check. Anyone with the code can turn the lights on, still subject to the schedule window.
- **`IgnoreSolarCheck`** (a setting) skips the schedule window check, so scans work at any hour.

The badge check uses our companion access control system, [`fsbhoa_ac_core`](https://github.com/dkeeney/fsbhoa_ac_core), through `/wp-json/fsbhoa/v1/access/verify-email`. Without it, use public QR mode or adapt `verifyLocalSwipe` in `lighting-service/remote_client.go`.

---

## Testing without hardware: softPLC

`softPLC` emulates a CLICK PLC closely enough to run the real ladder file:

```bash
pip install "pymodbus>=3.6,<4" flask
cd softPLC
python3 softPLC.py plc_config_lodge.json     # Modbus on :5020, dashboard on :8020
python3 softPLC.py plc_config_cabana.json    # Modbus on :5021, dashboard on :8021
```

Point the PLC addresses in WordPress at `127.0.0.1:5020` and `127.0.0.1:5021`. The dashboard shows registers, lets you edit them, and can single-step the program rung by rung. The sample configs start it running (`"startup_running": true`); set that to `false` to start paused in step mode. Retentive memory is saved to `retentive_memory_*.json`.

Because softPLC executes `LadderLogicForAI.txt` directly, keep that file in the format it parses: `Rung N: [conditions] -> (action)` lines, `(parallel) ->` continuation lines, `## Subroutine: NAME` headers and `//` comments.

---

## Adapting it to your site

Things that are specific to our install and that you'll likely change:

- **PLC IP addresses.** One is in the ladder logic (`RECV`, Rung 8); the others are in the settings.
- **Number of lights per PLC.** Rungs 4–5 (`DS6` = 16 or 24) and `NumLights` in `plc_client.go`.
- **Labels and names.** "Lodge", "Pool" and "Cabana" in UI labels; page slugs `lighting-monitor` and `lights`; the default latitude/longitude (Bakersfield, CA). That default is only used if the setting is left empty.
- **The register map.** It's duplicated in three places that must agree: the ladder `## Memory Map`, the constants at the top of `lighting-service/plc_client.go`, and `lighting-service/debug_handlers.go`.

Other PLCs that speak Modbus TCP could be used. You would rewrite the ladder logic for them and adjust the address calculations in `plc_client.go`.

---

## Security notes

- Keep the lighting server on your LAN. Only the admin pages need to be reachable, and they should be behind a login.
- Use **different** random keys for the lighting API and the remote site. Store them only in WordPress settings and `wp-config.php`, never in this repo. The service masks them in its logs.
- The QR flow is pull-only: the lighting server polls the public site, so nothing on the internet can call into your lighting network.

---

## Status and contributing

This is a volunteer project for one HOA, shared in the hope it's useful. Issues and pull requests are welcome, but support is best-effort. See [TODO.md](TODO.md) for known gaps; the main ones are unit tests for the Go service and moving more of the calendar logic out of the PLC.

## License

[GNU GPL v3.0](LICENSE)
