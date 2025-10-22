document.addEventListener('DOMContentLoaded', function () {
    const app = document.getElementById('fsbhoa-monitor-app');
    if (!app) return;

    const statusContainer = app.querySelector('#status-container');
    let zoneData = []; // Store zone info locally
    let mappingData = []; // Store mapping info locally
    let isUpdating = false; // Flag to prevent rapid clicks during update

    const api = {
        getStatus: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/status', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
        getZones: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/zones', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
        getMappings: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/mappings', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
        // --- NEW: API call for sending override command ---
        sendOverride: (zoneId, state) => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/override', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-WP-Nonce': fsbhoa_lighting_data.nonce
            },
            body: JSON.stringify({ zone_id: zoneId, state: state })
        })
    };

    const renderStatus = (status) => {
        if (zoneData.length === 0 || mappingData.length === 0) {
            statusContainer.innerHTML = '<p>Loading configuration...</p>';
            return;
        }

        const rows = zoneData.map(zone => {
            let isZoneOn = false;
            let firstOnOutput = null;

            // Determine zone state (existing logic)
            if (zone.mapping_ids && zone.mapping_ids.length > 0) {
                const firstMappingId = zone.mapping_ids[0];
                const mapping = mappingData.find(m => m.id == firstMappingId);
                if (mapping && mapping.plc_outputs) {
                    try {
                        const outputs = JSON.parse(mapping.plc_outputs);
                        if (outputs.length > 0) firstOnOutput = outputs[0];
                    } catch (e) { console.error("Error parsing mapping outputs:", mapping.plc_outputs); }
                }
            }
            if (firstOnOutput && status[firstOnOutput] === true) isZoneOn = true;

            const statusText = isZoneOn ? '<span style="color: green; font-weight: bold;">ON</span>' : '<span style="color: #666;">OFF</span>';
    
            // --- Apply CSS classes for icon state and disabled status ---
            const onLinkClasses = `override-link dashicons dashicons-lightbulb ${isZoneOn ? 'is-disabled' : ''}`;
            const offLinkClasses = `override-link dashicons dashicons-lightbulb is-off ${!isZoneOn ? 'is-disabled' : ''}`; // Add 'is-off' class
    
            return `
                <tr>
                    <td><strong>${escapeHTML(zone.zone_name)}</strong></td>
                    <td>${statusText}</td>
                    <td>
                        <a href="#" class="${onLinkClasses}" data-zone-id="${zone.id}" data-state="on" title="Turn ON"></a>
                        <a href="#" class="${offLinkClasses}" data-zone-id="${zone.id}" data-state="off" title="Turn OFF"></a>
                    </td>
                </tr>
            `;
        }).join('');

        const photocellStatus = status['Photocell'] === true
            ? '<span style="color: #333; font-weight: bold;">DARK</span> (Lights enabled)'
            : '<span style="color: orange; font-weight: bold;">LIGHT</span> (Lights disabled by daylight)';
    
        statusContainer.innerHTML = `
            <p>Status updates automatically every 5 seconds.</p>
            <table class="wp-list-table widefat striped fixed" style="margin-top:10px;">
                <thead><tr><th style="width: 40%;">Zone</th><th style="width: 20%;">Current State</th><th>Manual Override</th></tr></thead>
                <tbody>${rows}</tbody>
            </table>
            <div style="margin-top: 15px; font-size: 1.1em;">
                <strong>Photocell Status:</strong> ${photocellStatus}
            </div>
            <div id="override-status" style="margin-top: 10px; font-style: italic;"></div>
        `;
    };


    const updateStatus = async (forceConfigLoad = false) => {
        if (isUpdating) return; // Prevent overlapping updates
        isUpdating = true;

        try {
            const statusRes = await api.getStatus();
            if (!statusRes.ok) throw new Error(`Status API Error: ${statusRes.statusText}`);
            const status = await statusRes.json();

            // Fetch config only if needed (first load or forced)
            if (forceConfigLoad || zoneData.length === 0 || mappingData.length === 0) {
                const [zonesRes, mappingsRes] = await Promise.all([api.getZones(), api.getMappings()]);
                if (!zonesRes.ok || !mappingsRes.ok) throw new Error('Config API Error');
                zoneData = await zonesRes.json();
                mappingData = await mappingsRes.json();
            }

            renderStatus(status);

        } catch (error) {
            console.error('Failed to update status:', error);
            statusContainer.innerHTML = '<p style="color: red;">Error loading status. Check console and Go service.</p>';
        } finally {
            isUpdating = false;
        }
    };

    const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';

    // ---  Event listener for override buttons ---
    app.addEventListener('click', async (e) => {
        if (e.target.matches('.override-btn')) {
            e.preventDefault();
            const button = e.target;
            const zoneId = button.dataset.zoneId;
            const state = button.dataset.state;
            const statusDiv = document.getElementById('override-status');

            // Disable buttons during request
            button.disabled = true;
            statusDiv.textContent = `Sending ${state.toUpperCase()} command for Zone ${zoneId}...`;

            try {
                const response = await api.sendOverride(zoneId, state);
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.message || `Failed to send override (HTTP ${response.status})`);
                }
                statusDiv.textContent = `Override command sent. Waiting for next status update...`;
                // Optionally, trigger an immediate status update after a short delay
                setTimeout(() => updateStatus(), 1000); // Wait 1 sec for PLC to react

            } catch (error) {
                console.error('Override failed:', error);
                statusDiv.textContent = `Error: ${error.message}`;
                // Re-enable button on failure
                button.disabled = false;
            }
        }
    });

    // Initial load and then update every 5 seconds
    updateStatus(true); // Force config load on first run
    setInterval(updateStatus, 5000); // 5 seconds
});
