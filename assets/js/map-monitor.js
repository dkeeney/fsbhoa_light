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
            // Perform Reverse Lookup to find the Zone
            let SchedStatus = 0;
            
            // Find the zone where mapping_ids array contains this mapping.id
            // Ensure type matching (string vs int)
            const ownerZone = allZones.find(z => 
                z.mapping_ids && z.mapping_ids.some(id => String(id) === String(mapping.id))
            );

            if (ownerZone) {
                // Fetch the numeric status (0=Inactive, 1=Active, 2=QR Enabled)
                schedStatus = liveStatus[`Sched${ownerZone.schedule_id}`] || 0;
            }
            
            // Check for active timer
            const qroffValue = ownerZone ? (liveStatus[`qroff_zone_${ownerZone.id}`] || 0) : 0;

            // 3. Determine Color Class
            let statusClass = '';
            let currentState = 'off';
            const isActuallyOn = monitoredOn > 0;
            const isPartial = isActuallyOn && monitoredOn < monitoredTotal;
            let titleText = mapping.description;
            if (isActuallyOn && qroffValue > 0) {
                titleText += ` (QR Timer ends ${formatPLCTime(qroffValue)})`;
            }
            pin.title = titleText;

            //if (isActuallyOn) {
            //    console.log(`Id: ${mapping.id}, Pin ${mapping.description}: qroffValue is ${qroffValue}, ownerZone is ${ownerZone ? ownerZone.id : 'NONE'}`);
            //}

            if (isPartial) {
                statusClass = 'status-partial'; // CSS handles the pulse animation
                currentState = 'on';
            }
            else if (isActuallyOn) {
                currentState = 'on';
                // Yellow for Logic-driven or QR ON, Red-Orange for Manual ON
                // Logic: It is "Auto" if the schedule is 1 OR if we have a non-zero timer
                if (schedStatus === 1 || qroffValue > 0) {
                    statusClass = 'status-auto-on'; // Yellow
                } else {
                    statusClass = 'status-manual-on'; // Red
                }
            }
            else {
                currentState = 'off';
                // Blue if Sched is 1 but physically OFF, otherwise Gray
                statusClass = (schedStatus === 1) ? 'status-manual-off' : 'status-auto-off';
            }

            // Apply Classes
            // Reset classes first to avoid accumulation
            pin.className = `map-pin-live map-pin-${pin.dataset.size || 'small'} ${statusClass}`;
            
            // Store state for toggle logic
            pin.dataset.state = currentState;
        });

        // Update Photocell Text (Consistent with monitor-manager)
        const photocellStatus = liveStatus['Photocell'] === true
            ? '<span style="color: #333; font-weight: bold;">NIGHT</span> (Sundown Active)'
            : '<span style="color: orange; font-weight: bold;">DAY</span> (Waiting for Sundown)';
        statusIndicator.innerHTML = `<strong>Status:</strong> ${photocellStatus}`;
    }

    async function updateStatus() {
        if (isUpdating) return;
        isUpdating = true;
        try {
            // Re-attempt config fetch if data is missing
            if (allMappings.length === 0 || allZones.length === 0) {
                const [mapRes, zoneRes] = await Promise.all([api.getMappings(), api.getZones()]);
                if (mapRes.ok && zoneRes.ok) {
                    allMappings = await mapRes.json();
                    allZones = await zoneRes.json();
                    console.log('Map Data Loaded:', allMappings.length, 'mappings found.');
                    renderPins();
                } else {
                    console.error('Map Monitor: API fetch failed', mapRes.status, zoneRes.status);
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
        console.warn('Map Monitor: map_image_url is missing from localization data.'); 
        app.innerHTML = '<p>No map image set. Please check Lighting Settings.</p>';
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


    function formatPLCTime(val) {
        if (!val || val === 0) return "";
        let hours = Math.floor(val / 100);
        let mins = val % 100;
        let ampm = hours >= 12 ? 'pm' : 'am';
        hours = hours % 12;
        hours = hours ? hours : 12; 
        let strMins = mins < 10 ? '0' + mins : mins;
        return hours + ':' + strMins + ampm;
    }
});


