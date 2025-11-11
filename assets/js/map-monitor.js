document.addEventListener('DOMContentLoaded', function () {
    const app = document.getElementById('fsbhoa-map-monitor-app');
    if (!app) return;

    // --- Config ---
    // fsbhoa_lighting_data is localized from PHP
    const mapImageUrl = fsbhoa_lighting_data.map_image_url;
    const apiBaseUrl = fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/';
    const apiHeaders = { 'X-WP-Nonce': fsbhoa_lighting_data.nonce };

    // --- Page Elements ---
    const imageEl = document.getElementById('map-monitor-image');
    const pinOverlay = document.getElementById('map-pin-overlay');
    const statusIndicator = document.getElementById('map-status-indicator');

    // --- State ---
    let allMappings = []; // Stores all mapping data (incl. pins)
    let isUpdating = false;

    // --- API Calls ---
    const api = {
        getStatus: () => fetch(apiBaseUrl + 'status', { headers: apiHeaders }),
        getMappings: () => fetch(apiBaseUrl + 'mappings', { headers: apiHeaders })
    };

    /**
     * Renders all pins based on the stored mapping data.
     * This is called once on load.
     */
    function renderPins() {
        let pinHTML = '';
        allMappings.forEach(mapping => {
            // Check if coordinates exist and are a valid array
            if (Array.isArray(mapping.map_coordinates)) {
                mapping.map_coordinates.forEach(pin => {
                    // pin = { x, y, size }
                    // We use the mapping ID as the key to link status
                    pinHTML += `
                        <div class="map-pin-live map-pin-${pin.size} is-off"
                             data-mapping-id="${mapping.id}"
                             title="${escapeHTML(mapping.description)}"
                             style="left: ${pin.x}%; top: ${pin.y}%; transform: translate(-50%, -50%);">
                        </div>
                    `;
                });
            }
        });
        pinOverlay.innerHTML = pinHTML;
    }

    /**
     * Updates the color of all rendered pins based on live status.
     * This is called every 5 seconds.
     * @param {object} liveStatus - The status object from /status
     */
    function updatePinStatus(liveStatus) {
        // Create a fast lookup for mapping status
        const statusMap = {};
        allMappings.forEach(mapping => {
            let mappingIsOn = false;
            
            // A mapping is ON if its *first* defined PLC output is ON
            try {
                const outputs = mapping.plc_outputs; // This is now an array
                const plcID = mapping.plc_id;       // Get the PLC ID
                
                if (outputs.length > 0) {
                    const firstOutput = outputs[0];
                    // Construct the same unique key the Go service is sending
                    const uniqueKey = `PLC${plcID}-${firstOutput}`; 
                    
                    // Check the status using the unique key
                    if (liveStatus[uniqueKey] === true) {
                        mappingIsOn = true;
                    }
                }
            } catch (e) { /* ignore parse error */ }
            statusMap[mapping.id] = mappingIsOn;
        });

        // Loop over all pins on the page and update their class
        const pins = pinOverlay.querySelectorAll('.map-pin-live');
        pins.forEach(pin => {
            const mappingId = pin.dataset.mappingId;
            if (statusMap[mappingId]) {
                pin.classList.add('is-on');
                pin.classList.remove('is-off');
            } else {
                pin.classList.remove('is-on');
                pin.classList.add('is-off');
            }
        });

        // Update photocell status
        const photocellStatus = liveStatus['Photocell'] === true
            ? '<span style="color: #333; font-weight: bold;">DARK</span> (Lights enabled)'
            : '<span style="color: orange; font-weight: bold;">LIGHT</span> (Lights disabled by daylight)';

        statusIndicator.innerHTML = `<strong>Photocell:</strong> ${photocellStatus} (Updating every 5 seconds)`;
    }

    /**
     * Main data fetching and update loop
     */
    async function updateStatus() {
        if (isUpdating) return; // Prevent overlap
        isUpdating = true;

        try {
            // Fetch mappings only on the first run
            if (allMappings.length === 0) {
                statusIndicator.innerHTML = '<p>Loading mappings...</p>';
                const mappingsRes = await api.getMappings();
                if (!mappingsRes.ok) throw new Error('Failed to load mappings.');
                allMappings = await mappingsRes.json();
                renderPins(); // Draw the pins for the first time
            }

            // Fetch live status
            statusIndicator.innerHTML = '<p>Fetching live status...</p>';
            const statusRes = await api.getStatus();
            if (!statusRes.ok) throw new Error('Failed to load status.');
            const liveStatus = await statusRes.json();

            // Update pin colors
            updatePinStatus(liveStatus);

        } catch (error) {
            console.error('Map update failed:', error);
            statusIndicator.innerHTML = `<p style="color: red;">Error: ${error.message}</p>`;
        } finally {
            isUpdating = false;
        }
    }

    const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';

    // --- Start ---
    if (!mapImageUrl) {
        app.innerHTML = '<h1>Error</h1><p>No map image has been set. Please set one on the <a href="/wp-admin/admin.php?page=fsbhoa-lighting-settings">FSBHOA Lighting settings page</a>.</p>';
        return;
    }
    
    imageEl.src = mapImageUrl;
    imageEl.alt = "HOA Map";

    updateStatus(); // Initial load
    setInterval(updateStatus, 5000); // Refresh every 5 seconds
});
