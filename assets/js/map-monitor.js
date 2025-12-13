document.addEventListener('DOMContentLoaded', function () {
    const app = document.getElementById('fsbhoa-map-monitor-app');
    if (!app) return;

    // --- Config ---
    const mapImageUrl = fsbhoa_lighting_data.map_image_url;
    const apiBaseUrl = fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/';
    const apiHeaders = { 'X-WP-Nonce': fsbhoa_lighting_data.nonce };
    const apiPostHeaders = { 'Content-Type': 'application/json', 'X-WP-Nonce': fsbhoa_lighting_data.nonce };

    const imageEl = document.getElementById('map-monitor-image');
    const pinOverlay = document.getElementById('map-pin-overlay');
    const statusIndicator = document.getElementById('map-status-indicator');

    // --- State ---
    let allMappings = [];
    let allZones = []; // NEW: Need zones to look up schedules
    let isUpdating = false;

    // --- API ---
    const api = {
        getStatus: () => fetch(apiBaseUrl + 'status', { headers: apiHeaders }),
        getMappings: () => fetch(apiBaseUrl + 'mappings', { headers: apiHeaders }),
        getZones: () => fetch(apiBaseUrl + 'zones', { headers: apiHeaders }), // NEW
        test: (id, state) => fetch(apiBaseUrl + 'test-mapping', { 
            method: 'POST', headers: apiPostHeaders, body: JSON.stringify({ mapping_id: id, state: state }) 
        })
    };

    // --- Logic ---

    async function handlePinClick(e) {
        e.preventDefault();
        e.stopPropagation();
        const pin = e.currentTarget;
        const mappingId = pin.dataset.mappingId;
        const currentState = pin.dataset.state; // 'on' or 'off'
        
        // Toggle state
        const targetState = (currentState === 'on') ? 'off' : 'on';
        
        pin.style.opacity = '0.5'; // Visual feedback
        try {
            await api.test(mappingId, targetState);
        } catch (err) {
            alert('Failed to toggle light.');
        } finally {
            pin.style.opacity = '1';
        }
    }

    function renderPins() {
        let pinHTML = '';
        allMappings.forEach(mapping => {
            if (Array.isArray(mapping.map_coordinates)) {
                mapping.map_coordinates.forEach(pin => {
                    pinHTML += `
                        <div class="map-pin-live map-pin-${pin.size} status-auto-off"
                             data-mapping-id="${mapping.id}"
                             data-state="off"
                             title="${escapeHTML(mapping.description)}"
                             style="left: ${pin.x}%; top: ${pin.y}%; transform: translate(-50%, -50%);">
                        </div>
                    `;
                });
            }
        });
        pinOverlay.innerHTML = pinHTML;
        
        // Attach Listeners
        pinOverlay.querySelectorAll('.map-pin-live').forEach(pin => {
            pin.addEventListener('click', handlePinClick);
        });
    }

    function updatePinStatus(liveStatus) {
        const pins = pinOverlay.querySelectorAll('.map-pin-live');
        
        pins.forEach(pin => {
            const mappingId = pin.dataset.mappingId;
            // Note: Mapping IDs are numbers, dataset attributes are strings.
            const mapping = allMappings.find(m => m.id == mappingId);
            if (!mapping) return;

            // 1. Determine Hardware State (ON/OFF/PARTIAL)
            let monitoredTotal = 0;
            let monitoredOn = 0;
            
            if (mapping.plc_outputs) {
                mapping.plc_outputs.forEach(out => {
                    const key = `PLC${mapping.plc_id}-${out}`;
                    if (liveStatus.hasOwnProperty(key)) {
                        monitoredTotal++;
                        if (liveStatus[key] === true) monitoredOn++;
                    }
                });
            }

            // 2. Determine Logical State (Schedule)
            // FIX: Perform Reverse Lookup to find the Zone
            let isSchedActive = false;
            
            // Find the zone where mapping_ids array contains this mapping.id
            // Ensure type matching (string vs int)
            const ownerZone = allZones.find(z => 
                z.mapping_ids && z.mapping_ids.some(id => id == mapping.id)
            );

            if (ownerZone) {
                // Check if the zone's schedule is currently active
                if (liveStatus[`Sched${ownerZone.schedule_id}`] === true) {
                    isSchedActive = true;
                }
            }

            // 3. Determine Color Class
            let statusClass = '';
            let currentState = 'off';

            if (monitoredTotal > 0 && monitoredOn > 0 && monitoredOn < monitoredTotal) {
                statusClass = 'status-partial'; // Pulsing
                currentState = 'on'; 
            } 
            else if (monitoredOn > 0) {
                currentState = 'on';
                statusClass = isSchedActive ? 'status-auto-on' : 'status-manual-on'; // Yellow vs Orange
            } 
            else {
                currentState = 'off';
                statusClass = isSchedActive ? 'status-manual-off' : 'status-auto-off'; // Blue vs Black
            }

            // Apply Classes
            // Reset classes first to avoid accumulation
            pin.className = `map-pin-live map-pin-${pin.dataset.size || 'small'} ${statusClass}`;
            
            // Store state for toggle logic
            pin.dataset.state = currentState;
        });

        // Update Photocell Text
        const photocellStatus = liveStatus['Photocell'] === true
            ? '<span style="color: #333; font-weight: bold;">DARK</span>'
            : '<span style="color: orange; font-weight: bold;">LIGHT</span>';
        statusIndicator.innerHTML = `<strong>Photocell:</strong> ${photocellStatus}`;
    }

    async function updateStatus() {
        if (isUpdating) return;
        isUpdating = true;
        try {
            // Fetch Config ONCE
            if (allMappings.length === 0) {
                const [mapRes, zoneRes] = await Promise.all([api.getMappings(), api.getZones()]);
                if (mapRes.ok && zoneRes.ok) {
                    allMappings = await mapRes.json();
                    allZones = await zoneRes.json();
                    renderPins();
                }
            }

            // Fetch Status
            const statusRes = await api.getStatus();
            if (statusRes.ok) {
                const liveStatus = await statusRes.json();
                updatePinStatus(liveStatus);
            }
        } catch (error) {
            console.error(error);
        } finally {
            isUpdating = false;
        }
    }

    const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';

    // --- Init ---
    if (!mapImageUrl) {
        app.innerHTML = '<p>No map image set.</p>';
        return;
    }
    imageEl.src = mapImageUrl;
    
    // 1. Run immediately
    updateStatus();

    // 2. Set interval for subsequent runs
    // (Assign to variable so we can clear it if needed, though less critical here)
    const mapInterval = setInterval(updateStatus, 2000);

    // 3. Add Tab Wake-Up
    document.addEventListener("visibilitychange", () => {
        if (document.visibilityState === 'visible') {
            updateStatus(); // Fetch immediately when user looks at the screen
        }
    });
});


