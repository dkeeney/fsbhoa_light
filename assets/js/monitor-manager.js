document.addEventListener('DOMContentLoaded', function () {
    const app = document.getElementById('fsbhoa-monitor-app');
    if (!app) return;

    const statusContainer = app.querySelector('#status-container');
    let zoneData = []; 
    let mappingData = [];
    
    // --- TIMING CONFIG ---
    const POLL_INTERVAL_NORMAL = 2000; // 2 seconds (Normal)
    const POLL_INTERVAL_BURST = 250;   // 250ms (Turbo Mode)
    const BURST_DURATION = 10000;      // 10 seconds of Turbo
    let burstEndTime = 0;              // Timestamp when Turbo ends
    let isUpdating = false;
    let loopTimerId = null;
    let allSchedules = [];

    const api = {
        getStatus: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/status', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
        getZones: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/zones', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
        getMappings: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/mappings', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
        sendOverride: (zoneId, state) => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/override', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', 'X-WP-Nonce': fsbhoa_lighting_data.nonce },
            body: JSON.stringify({ zone_id: zoneId, state: state })
        }),
        sync: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/sync', {
            method: 'POST',
            headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce }
        }),

        triggerTimer: (zoneId) => {
            console.log(`📡 Sending trigger-timer request for Zone: ${zoneId}`);
            return fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/trigger-timer', {
                method: 'POST',
                headers: { 
                    'Content-Type': 'application/json', 
                    'X-WP-Nonce': fsbhoa_lighting_data.nonce 
                },
                body: JSON.stringify({ zone_id: zoneId })
            })
        }
    };

    // Trigger Turbo Mode (Called after a button click)
    const triggerBurstMode = () => {
        console.log("🚀 Entering Turbo Poll Mode (250ms) for 10 seconds...");
        burstEndTime = Date.now() + BURST_DURATION;
        // If we aren't currently updating, we will naturally pick up the speed on the next loop.
    };

    const formatQROffTime = (rawTime) => {
        if (!rawTime || rawTime <= 0) return '';
        const hours = Math.floor(rawTime / 100);
        const minutes = rawTime % 100;
    
        // Format to 12-hour time (e.g., 6:30 PM)
        const period = hours >= 12 ? 'PM' : 'AM';
        const displayHours = hours % 12 || 12;
        const displayMinutes = minutes < 10 ? '0' + minutes : minutes;
    
        return `${displayHours}:${displayMinutes} ${period}`;
    };

    // Here is a sample of what the Go Service returns from the status poll:
    //  $ curl http://localhost:8085/status
    // {"PLC1-Y101":false,"PLC1-Y103":false,"PLC1-Y105":false,"PLC1-Y107":false,"PLC1-Y109":false,"PLC1-Y111":false,"PLC1-Y113":false,"PLC1-Y115":false,"PLC1-Y201":false,"PLC1-Y203":false,"PLC1-Y205":false,"PLC1-Y207":false,"PLC1-Y209":false,"PLC1-Y211":false,"PLC1-Y213":false,"PLC1-Y215":false,"PLC1-Y301":false,"PLC1-Y303":false,"PLC1-Y305":false,"PLC1-Y307":false,"PLC1-Y309":false,"PLC1-Y311":false,"PLC1-Y313":false,"PLC1-Y315":false,"PLC2-Y101":false,"PLC2-Y103":false,"PLC2-Y105":false,"PLC2-Y107":false,"PLC2-Y109":false,"PLC2-Y111":false,"PLC2-Y113":false,"PLC2-Y115":false,"PLC2-Y201":false,"PLC2-Y203":false,"PLC2-Y205":false,"PLC2-Y207":false,"PLC2-Y209":false,"PLC2-Y211":false,"PLC2-Y213":false,"PLC2-Y215":false,"Photocell":false,"Sched10":1,"Sched11":1,"Sched12":1,"Sched4":1,"Sched5":0,"Sched6":1,"Sched7":1,"Sched8":0,"Sched9":1,"schedule_map_1":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"schedule_map_2":[0,5,0,0,0,5,0,5,0,11,0,5,0,11,0,0,0,11,0,5,0,5,0,4]}

    const renderStatus = (status) => {
        if (zoneData.length === 0 || mappingData.length === 0) {
            statusContainer.innerHTML = '<p>Loading configuration...</p>';
            return;
        }

        const rows = zoneData.map(zone => {
            // 1. Fetch Schedule & QR Status from Go Service
            const schedStatus = status[`Sched${zone.schedule_id}`] || 0;
            const qroffValue = status[`qroff_zone_${zone.id}`] || 0;

            // hasTimedCapability means the Go Service 
            // is currently processing a QR-based span for this zone.
            // SchedStatus 2 = QR span is active but light is physically off.
            // SchedStatus 1 with qroffValue > 0 = QR span is active and light is on.
            const hasTimedCapability = (schedStatus === 2) || (schedStatus === 1 && qroffValue > 0);

            // 2. Identify Mappings (Physical Lights)
            const zoneMappings = mappingData.filter(m => 
                zone.mapping_ids && zone.mapping_ids.map(String).includes(String(m.id))
            );

            let totalLights = 0;
            let lightsOn = 0;

            zoneMappings.forEach(mapping => {
                // Each mapping is ONE light: [On-Trigger, Off-Trigger]
                if (Array.isArray(mapping.plc_outputs) && mapping.plc_outputs.length > 0) {
                    totalLights++;
                    const onTrigger = mapping.plc_outputs[0];
                    const uniqueKey = `PLC${mapping.plc_id}-${onTrigger}`;
                    const val = status[uniqueKey];

                    if (val === true || val === 1) {
                        lightsOn++;
                    }
                }
            });
            

            // 3. Determine Color Class & Tooltip
            //  Blue is now reserved strictly for when schedStatus === 1 
            //  (The Go service is actively trying to hold the light ON) but the light is OFF.
            //
            //  Gray now covers both the "Default Off" and the "QR Armed" state, which feels 
            //  much more natural for the user.
            //
            //  The Clock Icon will stay visible in that "QR Armed" and Gray state, providing 
            //  the visual hint that it can be turned on.

            let statusClass = 'status-auto-off'; // Default Gray
            let tooltip = 'Off';
            const isActuallyOn = lightsOn > 0;
            const isPartial = isActuallyOn && lightsOn < totalLights;

            if (isPartial) {
                statusClass = 'status-partial status-pulsing';
                tooltip = `PARTIAL ERROR: ${lightsOn}/${totalLights} ON`;
            } 
            else if (isActuallyOn) {
                if (schedStatus === 1 || qroffValue > 0) {
                    statusClass = 'status-auto-on'; // Yellow
                    tooltip = qroffValue > 0 ? 'QR Timer Active' : 'Schedule Active';
                } else {
                    statusClass = 'status-manual-on'; // Red-Orange
                    tooltip = 'Manual Override ON';
                }
            } 
            else {
                // Light is PHYSICALLY OFF
                if (schedStatus === 2) {
                    // If Go says it's in QR mode (2), 
                    // it's not a "Manual Off" error, it's just waiting.
                    statusClass = 'status-auto-off'; // Gray
                    tooltip = qroffValue > 0 ? 'QR Timer Running (Waiting for Sundown/Range)' : 'QR Armed (Ready to Scan)';
                } else if (schedStatus === 1) {
                    // Only show Blue if it's a standard "Should be ON" span
                    statusClass = 'status-manual-off'; // Blue
                    tooltip = 'Manual OFF (Override)';
                } else {
                    statusClass = 'status-auto-off'; // Gray
                    tooltip = 'Off';
                }
            }

            // 4. State Column: Bulb + Expiration/Clock
            let timerHtml = '';
            if (qroffValue > 0) {
                const formattedTime = formatQROffTime(qroffValue);
                timerHtml = `
                   <a href="#" class="trigger-timer-link" data-zone-id="${zone.id}" title="Timer Expiration" style="text-decoration:none;">
                       <strong style="font-size:10px; color:#2271b1; font-family:monospace; margin-left:6px; border: 1px solid #d1ecf1; padding: 1px 3px; border-radius: 3px; background: #f8fdff;" title="Timer Expiration">Exp: ${formattedTime}</strong>
                   </a>`;
            } else if (hasTimedCapability) {
                timerHtml = `
                   <a href="#" class="trigger-timer-link" data-zone-id="${zone.id}" title="Start Timer" style="text-decoration:none;">
                        <span class="dashicons dashicons-clock" style="color:#ccc; font-size:17px; margin-left:6px; vertical-align:middle;" title="QR Trigger Available"></span>
                   </a>`;
            }


            // Use flexbox to keep them on the same row and centered
            const statusDisplay = `
                <div style="display: flex; align-items: center; justify-content: center; min-height: 24px;">
                    <span class="dashicons dashicons-lightbulb monitor-bulb ${statusClass}" title="${tooltip}" style="font-size:18px; width:18px; height:18px;"></span>
                    ${timerHtml}
                </div>`;

            // 5. Manual Control Links
            const currentLabel = isActuallyOn ? 'ON' : 'OFF';
            const onLinkClasses = `override-link ${currentLabel === 'ON' ? 'is-disabled' : ''}`;
            const offLinkClasses = `override-link ${currentLabel === 'OFF' ? 'is-disabled' : ''}`;

            const overrideLinks = `
                <a href="#" class="${onLinkClasses}" data-zone-id="${zone.id}" data-state="on">ON</a>
                <span style="margin: 0 5px; color: #ccc;">|</span>
                <a href="#" class="${offLinkClasses}" data-zone-id="${zone.id}" data-state="off">OFF</a>
            `;

            // 6. Schedule Badge Column (Using schedStatus instead of isSchedActive)
            let schedBadge = '<span style="color:#ccc; font-size:11px;">Inactive</span>';
            if (schedStatus === 1) {
                schedBadge = '<span style="color:#46b450; font-weight:bold; font-size:11px;">ACTIVE</span>';
            } else if (schedStatus === 2) {
                schedBadge = '<span style="color:#2271b1; font-weight:bold; font-size:11px;">QR ENABLED</span>';
            }

            return `
                <tr>
                    <td style="font-weight:600; font-size:13px; color:#222;">${escapeHTML(zone.zone_name)}</td>
                    <td>${schedBadge}</td>
                    <td>${statusDisplay}</td>
                    <td style="text-align:right;">
                        <div style="display:flex; justify-content:flex-end; gap:4px; align-items:center;">
                            ${overrideLinks}
                        </div>
                    </td>
                </tr>`;
        }).join('');

        // --- Photocell and Header Logic ---
        const photocellStatus = status['Photocell'] === true
            ? '<span style="color: #333; font-weight: bold;">NIGHT</span> (Sundown Active)'
            : '<span style="color: orange; font-weight: bold;">DAY</span> (Waiting for Sundown)';

        const isBursting = typeof burstEndTime !== 'undefined' && Date.now() < burstEndTime;
        const refreshRate = isBursting ? "Turbo (0.2s)" : "2s";

        statusContainer.innerHTML = `
            <div style="display:flex; justify-content:space-between; color:#666; font-style:italic; margin-bottom:5px; font-size:11px; border-bottom:1px solid #eee; padding-bottom:4px;">
                <span>Update: ${refreshRate}</span>
                <span>${photocellStatus}</span>
            </div>
            <table class="wp-list-table widefat striped fixed compact-monitor-table">
                <thead>
                    <tr>
                        <th style="width: 35%;">Zone</th>
                        <th style="width: 15%;">Schedule</th>
                        <th style="width: 25%;">State</th>
                        <th style="width: 25%; text-align:right;">Manual Control</th>
                    </tr>
                </thead>
                <tbody>${rows}</tbody>
            </table>
            <div id="override-status" style="margin-top: 5px; font-style: italic; font-size: 11px; min-height:15px;"></div>
        `;
    };

    // --- Sync Button Handler ---
    const syncBtn = document.getElementById('fsbhoa-manual-sync-btn');
    if (syncBtn) {
        syncBtn.addEventListener('click', async (e) => {
            e.preventDefault();
            if (syncBtn.classList.contains('disabled')) return;

            const originalText = syncBtn.textContent;
            syncBtn.textContent = 'Syncing...';
            syncBtn.classList.add('disabled'); // Visual feedback
            syncBtn.style.opacity = '0.6';

            try {
                await api.sync();
                // Trigger an immediate status update
                setTimeout(() => runUpdateLoop(true), 1000); 
                syncBtn.textContent = 'Done!';
            } catch (err) {
                console.error(err);
                syncBtn.textContent = 'Error';
            }

            // Reset button after 2 seconds
            setTimeout(() => {
                syncBtn.textContent = originalText;
                syncBtn.classList.remove('disabled');
                syncBtn.style.opacity = '1';
            }, 2000);
        });
    }

    // --- The Dynamic Polling Loop ---
    const runUpdateLoop = async (forceConfig = false) => {
        if (isUpdating) return;
        isUpdating = true;
        
        // Clear any existing scheduled run to prevent double-firing
        if (loopTimerId) clearTimeout(loopTimerId);
        let nextDelay = POLL_INTERVAL_NORMAL;

        try {
            const statusRes = await api.getStatus();

            // --- AUTO-HEAL: Handle Expired Nonce (403) ---
            if (statusRes.status === 403) {
                console.warn("Nonce expired. Attempting to refresh...");
                
                // 1. Request new nonce using Cookie Auth (no nonce header required)
                const refreshRes = await fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/refresh-nonce', {
                    method: 'POST' 
                });

                if (refreshRes.ok) {
                    const data = await refreshRes.json();
                    // 2. Update the global variable
                    fsbhoa_lighting_data.nonce = data.nonce;
                    console.log("Nonce refreshed successfully. Retrying...");
                    
                    // 3. Retry immediately (fast track)
                    isUpdating = false;
                    runUpdateLoop(forceConfig); 
                    return; 
                } else {
                    throw new Error("Session expired and Nonce Refresh failed. Please reload.");
                }
            }
            // ---------------------------------------------

            if (!statusRes.ok) throw new Error(`Status API Error`);

            //console.log("RAW GO SERVICE STATUS:", status);

            const status = await statusRes.json();

            if (forceConfig || zoneData.length === 0) {
                // UPDATE: Added api.getSchedules() to the Promise.all
                const [zonesRes, mappingsRes, schedulesRes] = await Promise.all([
                    api.getZones(), 
                    api.getMappings(),
                    fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/schedules', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } })
                ]);

                if (zonesRes.ok && mappingsRes.ok && schedulesRes.ok) {
                    zoneData = await zonesRes.json();
                    mappingData = await mappingsRes.json();
                    allSchedules = await schedulesRes.json(); 
                }
            }
            if (statusRes.ok) renderStatus(status);
        } catch (error) {
            console.error(error);
            // If it's a hard auth failure, show message
            if (error.message.includes("Session expired")) {
                 statusContainer.innerHTML = `<div class="notice notice-error inline"><p><strong>Logged Out:</strong> Please refresh the page to log back in.</p></div>`;
                 return; // Stop the loop
            }
        } finally {
            isUpdating = false;
            // Check burst mode for next delay
            if (typeof burstEndTime !== 'undefined' && Date.now() < burstEndTime) {
                nextDelay = POLL_INTERVAL_BURST;
            }
            loopTimerId = setTimeout(runUpdateLoop, nextDelay);
        }
    };

    const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';

    // --- Click Handler ---
    app.addEventListener('click', async (e) => {
        if (e.target.matches('a.override-link')) {
            e.preventDefault();
            const link = e.target;
            if (link.classList.contains('is-disabled')) return;

            const zoneId = link.dataset.zoneId;
            const state = link.dataset.state;
            const statusDiv = document.getElementById('override-status');

            app.querySelectorAll(`.override-link[data-zone-id="${zoneId}"]`).forEach(btn => btn.style.opacity = '0.5');
            statusDiv.textContent = `Sending ${state}...`;

            try {
                // 1. Send the command
                const response = await api.sendOverride(zoneId, state);
                if (!response.ok) throw new Error('Failed');
                
                statusDiv.textContent = `Sent. Watching for change...`;
                
                // 2. Trigger Turbo Mode to catch the result fast
                triggerBurstMode();

            } catch (error) {
                console.error(error);
                statusDiv.textContent = `Error: ${error.message}`;
                app.querySelectorAll(`.override-link[data-zone-id="${zoneId}"]`).forEach(btn => btn.style.opacity = '1');
            }
        }
        // Handle the Clock Icon or Expiration Text click
        const timerLink = e.target.closest('.trigger-timer-link');
    
        if (timerLink) {
            e.preventDefault();
            const zoneId = timerLink.dataset.zoneId;
            const statusDiv = document.getElementById('override-status');

            timerLink.style.opacity = '0.5';
            statusDiv.textContent = `Triggering 90-minute timer for Zone ${zoneId}...`;

            try {
                const response = await api.triggerTimer(zoneId);
                if (!response.ok) throw new Error('Failed to trigger timer');

                statusDiv.textContent = 'Timer active. Refreshing status...';
                // Speed up polling to show the yellow light immediately
                triggerBurstMode(); 
            } catch (error) {
                statusDiv.textContent = `Error: ${error.message}`;
                console.error(error);
            } finally {
                timerLink.style.opacity = '1';
            }
        }
    });

    // --- 1. Start the loop
    runUpdateLoop(true);

    // --- 2. WAKE UP: Run Immediately when Tab is opened/focused ---
    document.addEventListener("visibilitychange", () => {
        if (document.visibilityState === 'visible') {
            console.log("Tab active: Triggering immediate update.");
            
            // Cancel the waiting timer and run NOW
            if (loopTimerId) clearTimeout(loopTimerId);
            runUpdateLoop();
        }
    });
});

