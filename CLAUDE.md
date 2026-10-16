# CLAUDE.md

FSBHOA lighting controller: schedules and drives latching lighting relays at the Lodge and Cabana through two CLICK PLCs.

## Architecture

Four parts, each running in a different place:

1. **WordPress plugin** (`fsbhoa_lighting.php`, `includes/`, `assets/js/`): the admin UI and the source of truth (MySQL tables `wp_fsbhoa_lighting_*`).
   - Shortcodes `[lighting_configuration]`, `[lighting_schedules]`, `[lighting_status_monitor]`, `[lighting_map_monitor]` render the views. All are admin-only (`manage_options`).
   - REST namespace `fsbhoa-lighting/v1`. `/full-config` is protected by the Go service API key (`X-API-KEY`), not by a WP login.
   - `includes/admin-settings.php` (Settings > Lighting Control) **writes the Go service config** to `/var/lib/fsbhoa/lighting_service.json` and can restart the systemd service.
2. **Go service** (`lighting-service/`, systemd unit `fsbhoa-lighting.service`, port 8085): the bridge between WordPress and the PLCs over Modbus TCP.
   - `POST /sync`: pulls `/full-config` from WP, computes sunrise/sunset (`applySolarTranslations`), packs schedules into PLC slots, writes the registers, then sets C151 (resync).
   - `/override/zone/:id/:state` and `/test/mapping/:id/:state` set the RequestON/RequestOFF coils. `/trigger-timer/zone/:id` writes the QROff time. `/status` reads the PLC state back. `/debug/registers` dumps raw registers.
   - A UI sync (`/sync`) is a **full push plus C151**, which forces every light to its schedule state and cancels manual overrides and running QR timers.
   - At startup and daily at 3 AM it re-syncs the PLC clocks (DST) and runs `refreshSolarSchedules`, which re-pushes the schedule registers with today's sunrise/sunset **without** C151, so overrides survive. Before this was added (Oct 2026), SUNDOWN times went stale between UI saves and lights came on late.
   - `remote_client.go` long-polls the public HOA site (Bluehost) for QR-code jobs. It checks the Access Control system for a recent gate swipe by that email and checks the schedule's QR window. If both pass, it writes QROff.
3. **PLC ladder logic** (`LadderLogicForAI.txt`): a hand-written text rendering of the CLICK program. The real PLCs are programmed by hand in CLICK software from this file. Lodge = master (24 lights, C153 set); Cabana = slave (16 lights) and has the photocell on X001, which the Lodge reads via Modbus RECV every second.
4. **Remote plugin** (`remote-plugin/`): a separate WP plugin installed on the public HOA site (**fsbhoa.com** on Bluehost, not access.fsbhoa.com). It handles the `[court_lights]` QR page and the `lighting_queue` table, and exposes `wait-for-job`, `update-job` and `debug-db`. All three require `X-API-Key` matching the `FSBHOA_LIGHTING_API_KEY` constant in Bluehost's `wp-config.php`. There is deliberately no fallback key.

`softPLC/softPLC.py` is a CLICK emulator (pymodbus + Flask). It **parses and executes `LadderLogicForAI.txt` directly**. The Lodge runs on Modbus 5020 and the Cabana on 5021, each with a dashboard at the Modbus port + 3000. Retentive DS99–DS1112 persist to `retentive_memory_*.json` (untracked). `OrigLadderLogicForAI.txt` is an older snapshot, kept for reference.

## Ladder logic rules (LadderLogicForAI.txt)

- **It must stay parseable by softPLC**: `## Subroutine: NAME` headers, `Rung N: [conds] -> (action)`, continuation lines starting `(parallel) ->`, `//` comments, and everything after `## Memory Map` is ignored. Edits to this file change simulator behavior.
- **Hardware limits** (real CLICK PLC): no nested loops or nested subroutine calls; rungs containing FOR/NEXT cannot have contacts; indirect addressing (`DS[DSn]`, `C[DSn]`) only inside COPY.
- Only one relay pulse at a time (power supply limit). The main loop sequences light pairs DS3 = 1..DS6, with a 250 ms pulse (T1) and a 100 ms dead time (T2).
- Schedule states: 0 = OFF, 1 = ON, 2 = QR-enabled. Triggers: 0 = time, 1 = photocell, 2 = QR+time, 3 = QR+photocell. Times are HHMM integers. Days are a bitmask with Sun = bit 0.
- When copying the logic to the Cabana PLC, Rung 3 changes (master sets C153, slave resets it).

## Memory map coupling (keep in sync!)

The ladder `## Memory Map`, the Go constants at the top of `lighting-service/plc_client.go`, and `debug_handlers.go` must agree. Modbus addresses are 0-based: DSn → n-1; Cn → 16384 + n-1; Y outputs per `yOutputToModbusAddress`.

| Range | Meaning |
|---|---|
| C101–C124 | Light current state |
| C151 / C152 | Sync request / latched copy |
| C153 / C154 | isMaster / photocell state |
| C201–C224, C251–C274 | RequestON / RequestOFF per light |
| DS99 | Total active spans |
| DS100–DS939 | 12 schedule slots × 14 spans × 5 regs (days, onTrig, onTime, offTrig, offTime) |
| DS941–DS964 / DS971–DS994 | Current / previous QROff time per light |
| DS1000–DS1023 | Light → schedule slot (0 = unused) |
| DS1031–DS1042 | Slot → WordPress schedule ID |
| DS1051–DS1062 / DS1101–DS1112 | Current / previous schedule state |

Light index = output pair: `loopIndex = (module-1)*8 + (output-1)/2`. On output = odd Y, Off = even Y. Only schedules assigned to a zone are uploaded (max 12).

## Commands

```bash
# Go service
cd lighting-service && ./build.sh && sudo systemctl restart fsbhoa-lighting.service
sudo journalctl -u fsbhoa-lighting.service -n 50 --no-pager
go vet ./...            # keep clean; no unit tests yet (see TODO.md)

# PHP syntax check
php -l includes/<file>.php

# Simulators (testbed)
cd softPLC && python3 softPLC.py plc_config_lodge.json   # and plc_config_cabana.json
```

## Deployment notes

- This checkout is **live**: `/var/www/html/wp-content/plugins/` symlinks to `/home/pi/fsbhoa_light` and to `remote-plugin/`. PHP/JS edits take effect immediately. Bump the version in `wp_enqueue_script` calls when changing JS, to bust caches.
- Go changes need a rebuild and restart. The binary `lighting-service/lighting-service` is gitignored.
- `/var/lib/fsbhoa/` also holds config for sibling FSBHOA systems (access control, etc.). Don't touch those files.
- This Pi is the **testbed** (plugin inactive, PLCs are softPLC on 127.0.0.1). **Production** is `fsbhoa@access.fsbhoa.com` with its checkout at `~/fsbhoa_light`. To deploy: push, then on production run `git pull --ff-only && cd lighting-service && ./build.sh && sudo systemctl restart fsbhoa-lighting.service`. Production logs are in `journalctl -u fsbhoa-lighting.service`.
- Remote plugin: run `remote-plugin/deploy.sh`, then upload the zip to fsbhoa.com and choose "Replace current with uploaded". The zip must contain a `fsbhoa-remote-lighting/` folder, or WordPress installs it as a second plugin. `*.zip` is gitignored.
- Never log or commit secrets. The repo is public on GitHub. API keys live in WP settings, `/var/lib/fsbhoa/lighting_service.json` and Bluehost's `wp-config.php`. The startup config log masks them.
- No automated tests. Verify against the softPLC simulators where possible, not the real PLCs.
