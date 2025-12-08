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
        })
    };

    // Trigger Turbo Mode (Called after a button click)
    const triggerBurstMode = () => {
        console.log("🚀 Entering Turbo Poll Mode (250ms) for 10 seconds...");
        burstEndTime = Date.now() + BURST_DURATION;
        // If we aren't currently updating, we will naturally pick up the speed on the next loop.
    };

    const renderStatus = (status) => {
        if (zoneData.length === 0 || mappingData.length === 0) {
            statusContainer.innerHTML = '<p>Loading configuration...</p>';
            return;
        }

        const rows = zoneData.map(zone => {
            // 1. Find all mappings for this zone
            const zoneMappings = mappingData.filter(m => zone.mapping_ids.includes(m.id));
            
            let totalLights = 0;
            let lightsOn = 0;
            let plcID = 0; // Use the first found PLC ID for override logic later

            // 2. Count ON vs OFF lights
            zoneMappings.forEach(mapping => {
                plcID = mapping.plc_id; 
                if (mapping.plc_outputs && mapping.plc_outputs.length > 0) {
                    // FIX: Only check the first output (the Control/ON wire)
                    const output = mapping.plc_outputs[0];
                    totalLights++;
                    const uniqueKey = `PLC${mapping.plc_id}-${output}`;
                    if (status[uniqueKey] === true) lightsOn++;
                }
            });

            // 3. Determine Schedule State
            // The Go service now sends "Sched1": true, "Sched5": false, etc.
            const schedID = zone.schedule_id;
            const isSchedActive = status[`Sched${schedID}`] === true;

            // 4. Calculate Status Logic
            let statusClass = '';
            let statusLabel = '';
            let tooltip = '';
            
            // --- Determine Base Color (State) ---
            if (lightsOn > 0) {
                // Zone has Light (Full or Partial)
                statusLabel = 'ON';
                
                if (isSchedActive) {
                    // Schedule says ON + Light detected -> YELLOW (Auto)
                    statusClass = 'status-auto-on';
                    tooltip = 'Auto ON (Schedule Active)';
                } else {
                    // Schedule says OFF + Light detected -> ORANGE (Manual)
                    statusClass = 'status-manual-on';
                    tooltip = 'Manual ON (Override)';
                }
            } 
            else {
                // Zone is Dark
                statusLabel = 'OFF';
                
                if (isSchedActive) {
                    // Schedule says ON + No Light -> BLUE (Manual OFF)
                    statusClass = 'status-manual-off';
                    tooltip = 'Manual OFF (Override)';
                } else {
                    // Schedule says OFF + No Light -> BLACK (Auto)
                    statusClass = 'status-auto-off';
                    tooltip = 'Off';
                }
            }

            // --- Determine Animation (Consistency) ---
            if (lightsOn > 0 && lightsOn < totalLights) {
                // Mixed State -> Add Pulse
                statusClass += ' status-pulsing';
                tooltip += ` - PARTIAL (${lightsOn}/${totalLights})`;
            }


            // Render the Bulb Icon
            const statusText = `<span class="dashicons dashicons-lightbulb monitor-bulb ${statusClass}" title="${tooltip}"></span>`;

            // --- Override Links (Same as before) ---
            // We disable the link matching the current state
            const onLinkClasses = `override-link ${statusLabel === 'ON' ? 'is-disabled' : ''}`;
            const offLinkClasses = `override-link ${statusLabel === 'OFF' ? 'is-disabled' : ''}`;
            
            const overrideLinks = `
                <a href="#" class="${onLinkClasses}" data-zone-id="${zone.id}" data-state="on" title="Turn Zone ON">ON</a>
                <span style="margin: 0 5px; color: #ccc;">|</span>
                <a href="#" class="${offLinkClasses}" data-zone-id="${zone.id}" data-state="off" title="Turn Zone OFF">OFF</a>
            `;
            const schedBadge = isSchedActive 
                ? '<span style="color:#46b450; font-weight:bold; font-size:11px;">ACTIVE</span>' 
                : '<span style="color:#ccc; font-size:11px;">Inactive</span>';

            return `
                <tr>
                    <td style="font-weight:600; font-size:13px; color:#222;">
                        ${escapeHTML(zone.zone_name)}
                    </td>
                    <td>
                        ${schedBadge}
                    </td>
                    <td>
                        <div class="state-wrapper">
                            ${statusText} 
                            <span class="state-label">${statusLabel}</span>
                        </div>
                    </td>
                    <td style="text-align:right;">
                        <div style="display:flex; justify-content:flex-end; gap:4px; align-items:center;">
                            ${overrideLinks}
                        </div>
                    </td>
                </tr>
            `;
        }).join('');

        const photocellStatus = status['Photocell'] === true
            ? '<span style="color: #333; font-weight: bold;">DARK</span> (Lights enabled)'
            : '<span style="color: orange; font-weight: bold;">LIGHT</span> (Lights disabled by daylight)';

        // Show current polling speed
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
            const status = await statusRes.json();

            if (forceConfig || zoneData.length === 0) {
                const [zonesRes, mappingsRes] = await Promise.all([api.getZones(), api.getMappings()]);
                if (zonesRes.ok && mappingsRes.ok) {
                    zoneData = await zonesRes.json();
                    mappingData = await mappingsRes.json();
                }
            }
            renderStatus(status);
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
            setTimeout(runUpdateLoop, nextDelay);
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
    });

    // Start the loop
    runUpdateLoop(true);
});

