# TODO

## Next
- [ ] **Unit tests for the lighting service** (`lighting-service/*_test.go`). Cover the pure functions that translate WordPress config into PLC registers:
  - `generateScheduleBlock`: span packing, 14-span cap, spans with no days skipped
  - `calculateLoopIndex` / `yOutputToModbusAddress`: Y101→0, Y103→1, Y201→8, Y301→16, invalid input
  - `triggerToPLCData`: trigger codes and HHMM parsing ("07:15:00" → 715)
  - `daysToBitmask`, `scheduleIDToModbusAddress`
  - `applySolarTranslations`: SUNDOWN/SUNRISE replaced with HHMM, other spans untouched
  - Keep them runnable without a PLC (`go test ./...`). The softPLC end-to-end check stays a manual step.

## Later
- [ ] Optional sunset/sunrise offset (e.g. "sunset + 15 min"). Not needed for now: 18:21 on 2026-10-15 looked right.
- [ ] Move the calendar logic out of the ladder into Go. The PLC would get a flat per-light on/off table pushed nightly for several days ahead, which removes the span/slot/bitmask logic and its CLICK no-nested-loops workarounds. Do this the next time the ladder has to change anyway.
- [ ] Single source of truth for the register map. `plc_client.go`, `debug_handlers.go` and the ladder `## Memory Map` are kept in sync by hand.
- [ ] Add the logrotate config (`/etc/logrotate.d/fsbhoa-lighting`) to `install.sh`.
- [ ] Check whether `/fsbhoa-lighting/v1/verify-swipe` (`includes/api-routes.php`) is still used. The Go service calls Access Control directly.

## Production housekeeping (manual)
- [ ] Mappings 61 and 77 have output `---` and are skipped at every push. Fix or delete them in the configuration UI.
- [ ] Drop the empty `wp_lighting_queue` table on access.fsbhoa.com (`fsbhoa_db`). The remote plugin created it there by mistake on 2026-10-15.
- [ ] Change the CLICK PLC login password. The old one was in this public repo's ladder files until 2026-10-15.
- [ ] `IgnoreSolarCheck` is ON in production, so QR scans turn lights on at any time of day. Confirm that's intended.
