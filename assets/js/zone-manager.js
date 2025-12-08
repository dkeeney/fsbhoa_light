// --- Helper Functions (Global) ---
const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';
const formatTime = (timeStr) => {
    if (!timeStr || !timeStr.includes(':')) return 'Invalid Time';
    let [hours, minutes] = timeStr.split(':');
    hours = parseInt(hours, 10);
    const ampm = hours >= 12 ? 'PM' : 'AM';
    hours = hours % 12;
    hours = hours ? hours : 12; // the hour '0' should be '12'
    return `${hours}:${minutes} ${ampm}`;
};

// --- API Definitions (Global) ---
// fsbhoa_lighting_data is localized from PHP
const apiBaseUrl = fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/';
const apiHeaders = { 'X-WP-Nonce': fsbhoa_lighting_data.nonce };
const apiPostHeaders = { 'Content-Type': 'application/json', 'X-WP-Nonce': fsbhoa_lighting_data.nonce };

const zoneApi = {
    get: () => fetch(apiBaseUrl + 'zones', { headers: apiHeaders }),
    save: (data) => fetch(apiBaseUrl + 'zones', { method: 'POST', headers: apiPostHeaders, body: JSON.stringify(data) }),
    delete: (zoneId) => fetch(apiBaseUrl + `zones/${zoneId}`, { method: 'DELETE', headers: apiHeaders })
};
const mappingApi = {
    get: () => fetch(apiBaseUrl + 'mappings', { headers: apiHeaders }),
    save: (data) => fetch(apiBaseUrl + 'mappings', { method: 'POST', headers: apiPostHeaders, body: JSON.stringify(data) }),
    delete: (mappingId) => fetch(apiBaseUrl + `mappings/${mappingId}`, { method: 'DELETE', headers: apiHeaders })
};
const scheduleApi = {
    get: () => fetch(apiBaseUrl + 'schedules', { headers: apiHeaders }),
    save: (data) => fetch(apiBaseUrl + 'schedules', { method: 'POST', headers: apiPostHeaders, body: JSON.stringify(data) }),
    delete: (id) => fetch(apiBaseUrl + `schedules/${id}`, { method: 'DELETE', headers: apiHeaders })
};
const assignmentApi = {
    saveOne: (data) => fetch(apiBaseUrl + 'zone-assignment', { // Use new singular endpoint
        method: 'POST',
        headers: apiPostHeaders,
        body: JSON.stringify(data)
    })
};
const testApi = {
    send: (id, state) => fetch(apiBaseUrl + 'test-mapping', { 
        method: 'POST', 
        headers: apiPostHeaders, 
        body: JSON.stringify({ mapping_id: id, state: state }) 
    })
};
const statusApi = {
    get: () => fetch(apiBaseUrl + 'status', { headers: apiHeaders })
};

// --- Global Data Store ---
let allZones = [];
let allMappings = [];
let allSchedules = [];

// =================================================================
// INITIAL LOAD FUNCTION (Global)
// =================================================================
const loadAllConfigData = async () => {
    const zoneApp = document.getElementById('fsbhoa-zone-manager-app');
    const scheduleApp = document.getElementById('fsbhoa-schedules-app');
    const mappingApp = document.getElementById('fsbhoa-mapping-manager-app');

    try {
        console.log("Loading all config + live status...");
        
        // 1. Fetch Zones, Mappings, Schedules, AND Status
        const [zonesRes, mappingsRes, schedulesRes, statusRes] = await Promise.all([
            zoneApi.get(), mappingApi.get(), scheduleApi.get(), statusApi.get()
        ]);

        if (!zonesRes.ok) throw new Error(`Failed loading zones`);
        if (!mappingsRes.ok) throw new Error(`Failed loading mappings`);
        if (!schedulesRes.ok) throw new Error(`Failed loading schedules`);
        
        // 2. Parse JSON
        allZones = await zonesRes.json();
        allMappings = await mappingsRes.json();
        allSchedules = await schedulesRes.json();
        // Handle status gracefully if service is offline
        const liveStatus = statusRes.ok ? await statusRes.json() : {};

        // 3. Render
        if (zoneApp) renderZonesTable(zoneApp.querySelector('#zones-list-container'), zoneApp.querySelector('#save-zone-assignments-btn'), allZones, allSchedules);
        if (scheduleApp) renderSchedulesTable(scheduleApp.querySelector('#schedules-list-container'), allSchedules);
        
        // FIX: Pass allZones and liveStatus to the mapping renderer
        if (mappingApp) renderMappingsTable(mappingApp.querySelector('#mappings-list-container'), allMappings, allZones, liveStatus);

        console.log("Data loaded.");
    } catch (error) {
        console.error('Error loading initial configuration data:', error);
        const errorMsg = '<p style="color: red;">Error loading configuration. Check console and ensure Go service is running.</p>';
        if (zoneApp) zoneApp.querySelector('#zones-list-container').innerHTML = errorMsg;
        if (scheduleApp) scheduleApp.querySelector('#schedules-list-container').innerHTML = errorMsg;
        if (mappingApp) mappingApp.querySelector('#mappings-list-container').innerHTML = errorMsg;
    }
};

// =================================================================
// ATTACH EVENT LISTENERS (This is the only code that runs on load)
// =================================================================
document.addEventListener('DOMContentLoaded', function () {
    
    // --- Print Button Handler ---
    const printButton = document.getElementById('fsbhoa-print-config-btn');
    if (printButton) {
        printButton.addEventListener('click', function(e) {
            e.preventDefault();
            window.print();
        });
    }

    // --- Debug Download Button Handler ---
    const debugBtn = document.getElementById('fsbhoa-debug-download-btn');
    if (debugBtn) {
        debugBtn.addEventListener('click', function(e) {
            e.preventDefault();
            
            // Call the new debug endpoint
            fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/debug-config', {
                headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce }
            })
            .then(response => response.json())
            .then(data => {
                // Create a downloadable file from the JSON
                const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(data, null, 2));
                const downloadAnchorNode = document.createElement('a');
                downloadAnchorNode.setAttribute("href", dataStr);
                downloadAnchorNode.setAttribute("download", "fsbhoa_lighting_debug.json");
                document.body.appendChild(downloadAnchorNode); // required for firefox
                downloadAnchorNode.click();
                downloadAnchorNode.remove();
            })
            .catch(err => alert('Error downloading config: ' + err));
        });
    }

    // --- Zone Manager ---
    const zoneApp = document.getElementById('fsbhoa-zone-manager-app');
    if (zoneApp) {
        const listContainer = zoneApp.querySelector('#zones-list-container');
        const formContainer = zoneApp.querySelector('#zone-form-container');
        const addNewBtn = zoneApp.querySelector('#add-new-zone-btn');
        const saveAssignmentsBtn = zoneApp.querySelector('#save-zone-assignments-btn');

        if(addNewBtn) {
            addNewBtn.addEventListener('click', (e) => { e.preventDefault(); renderZoneForm(formContainer, listContainer, addNewBtn, saveAssignmentsBtn, allMappings); });
        }

        zoneApp.addEventListener('click', async (e) => {
            if (e.target.matches('.edit-zone-link, .delete-zone-link, #cancel-edit-btn, #save-zone-assignments-btn')) e.preventDefault();

            if (e.target.matches('.edit-zone-link')) {
                const zoneId = e.target.dataset.zoneId;
                const zoneToEdit = allZones.find(z => z.id == zoneId);
                renderZoneForm(formContainer, listContainer, addNewBtn, saveAssignmentsBtn, allMappings, zoneToEdit);
            } else if (e.target.matches('.delete-zone-link')) {
                const zoneId = e.target.dataset.zoneId;
                if (confirm('Are you sure you want to delete this zone?')) {
                    await zoneApi.delete(zoneId);
                    loadAllConfigData();
                }
            } else if (e.target.matches('#cancel-edit-btn')) {
                formContainer.style.display = 'none'; listContainer.style.display = 'block'; addNewBtn.style.display = 'inline-block'; saveAssignmentsBtn.style.display = 'inline-block';
            }
        });

        zoneApp.addEventListener('submit', async (e) => {
            if (e.target.matches('#zone-form')) {
                e.preventDefault();
                const formData = new FormData(e.target);
                const data = Object.fromEntries(formData.entries());
                data.mapping_ids = formData.getAll('mapping_ids[]');
                await zoneApi.save(data);
                formContainer.style.display = 'none'; listContainer.style.display = 'block'; addNewBtn.style.display = 'inline-block';
                loadAllConfigData();
            }
        });

        //  Automatically save when a schedule dropdown is changed
        zoneApp.addEventListener('change', async (e) => {
            if (e.target.matches('.zone-schedule-select')) {
                const select = e.target;
                const zoneId = select.dataset.zoneId;
                const scheduleId = select.value;

                // Give visual feedback that something is happening
                select.style.transition = 'outline 0.2s ease';
                select.style.outline = '2px solid orange'; // "Working"

                try {
                    const response = await assignmentApi.saveOne({
                        zone_id: zoneId,
                        schedule_id: scheduleId
                    });
                    if (!response.ok) throw new Error('Failed to save');

                    // Success: flash green
                    select.style.outline = '2px solid green';
                } catch (error) {
                    // Failure: flash red
                    console.error('Error saving assignment:', error);
                    select.style.outline = '2px solid red';
                    alert('Error saving schedule assignment. Please check the console.');
                }

                // Remove feedback after a moment
                setTimeout(() => {
                    select.style.outline = 'none';
                }, 1000);
            }
        });

        
        
    }

    // --- Schedule Manager ---
    const scheduleApp = document.getElementById('fsbhoa-schedules-app');
    if (scheduleApp) {
        const scheduleListContainer = scheduleApp.querySelector('#schedules-list-container');
        const scheduleFormContainer = scheduleApp.querySelector('#schedule-form-container');
        const addScheduleBtn = scheduleApp.querySelector('#add-new-schedule-btn');

        if(addScheduleBtn) {
            addScheduleBtn.addEventListener('click', e => { e.preventDefault(); renderScheduleForm(scheduleFormContainer, scheduleListContainer, addScheduleBtn); });
        }

        scheduleApp.addEventListener('change', e => {
            if (e.target.matches('select[name="on_trigger"], select[name="off_trigger"]')) {
                e.target.nextElementSibling.style.display = e.target.value === 'TIME' ? 'inline-block' : 'none';
            }
        });

        scheduleApp.addEventListener('click', async e => {
            if (e.target.matches('.edit-schedule-link, .delete-schedule-link, .remove-span-btn, #add-span-btn, #cancel-btn')) e.preventDefault();

            if (e.target.matches('#cancel-btn')) {
                scheduleFormContainer.style.display = 'none'; scheduleListContainer.style.display = 'block'; addScheduleBtn.style.display = 'inline-block';
            } else if (e.target.matches('.edit-schedule-link')) {
                const id = e.target.dataset.scheduleId;
                const scheduleToEdit = allSchedules.find(s => s.id == id);
                renderScheduleForm(scheduleFormContainer, scheduleListContainer, addScheduleBtn, scheduleToEdit);
            } else if (e.target.matches('.delete-schedule-link')) {
                const id = e.target.dataset.scheduleId;
                if (confirm('Are you sure?')) { await scheduleApi.delete(id); loadAllConfigData(); }
            } else if (e.target.matches('#add-span-btn')) {
                document.getElementById('schedule-spans-container').insertAdjacentHTML('beforeend', renderSpanRow());
            } else if (e.target.matches('.remove-span-btn')) {
                if (scheduleApp.querySelectorAll('.schedule-span-row').length > 1) {
                    e.target.closest('.schedule-span-row').remove();
                } else { alert('A schedule must have at least one time span.'); }
            }
        });

        scheduleApp.addEventListener('submit', async e => {
            if (e.target.matches('#schedule-form')) {
                e.preventDefault();
                const data = {
                    schedule_id: e.target.querySelector('[name="schedule_id"]').value,
                    schedule_name: e.target.querySelector('[name="schedule_name"]').value,
                    spans: []
                };
                let validationError = null;

        	document.querySelectorAll('.schedule-span-row').forEach(row => {
                    if (validationError) return; // Stop processing if we found an error

                    const daysOfWeek = Array.from(row.querySelectorAll('input[name="days_of_week"]:checked')).map(cb => cb.value);
            
                    const onTrigger = row.querySelector('[name="on_trigger"]').value;
                    const offTrigger = row.querySelector('[name="off_trigger"]').value;
                    const onTimeInput = row.querySelector('[name="on_time"]').value;
                    let offTimeInput = row.querySelector('[name="off_time"]').value;

                    // --- LOGIC ENFORCEMENT ---
            
                    // 1. Auto-Correct "Midnight" (00:00) to "End of Day" (23:59)
                    // This applies to ALL TIME triggers (Start or End)
                    if (offTrigger === 'TIME' && (offTimeInput === '00:00' || offTimeInput === '')) {
                        offTimeInput = '23:59';
                    }

                    // 2. Validation Rules
                    if (onTrigger === 'TIME' && offTrigger === 'TIME') {
                        // Rule: Start must be before End
                        if (onTimeInput >= offTimeInput) {
                            validationError = `Invalid Time: ${formatTime(onTimeInput)} to ${formatTime(offTimeInput)}.\n\nSchedules cannot cross midnight. Please split this into two spans (e.g., Evening to 23:59, then 00:00 to Morning).`;
                            return;
                        }
                    } 
                    else if (onTrigger === 'SUNDOWN' && offTrigger === 'TIME') {
                        // Rule: If starting at Sundown, End Time must be PM (or 23:59)
                        // If they pick an AM time (e.g. 02:00), the light won't turn on until midnight.
                        // We use '12:00' as a safe heuristic for "Noon".
                        if (offTimeInput < '12:00' && offTimeInput !== '23:59') {
                             validationError = `Invalid Logic: "Sundown to ${formatTime(offTimeInput)}".\n\nBecause ${formatTime(offTimeInput)} is the next morning, this schedule crosses midnight.\n\nPlease split this into two spans:\n1. Sundown to 23:59\n2. 00:00 to ${formatTime(offTimeInput)}`;
                             return;
                        }
                    }
                    else if (onTrigger === 'TIME' && offTrigger === 'SUNRISE') {
                        // Rule: If ending at Sunrise, Start Time must be AM (e.g. 04:00)
                        // If they pick a PM time (e.g. 22:00), it crosses midnight.
                        if (onTimeInput > '12:00') {
                             validationError = `Invalid Logic: "${formatTime(onTimeInput)} to Sunrise".\n\nBecause ${formatTime(onTimeInput)} is the previous night, this schedule crosses midnight.\n\nPlease split this into two spans:\n1. ${formatTime(onTimeInput)} to 23:59\n2. 00:00 to Sunrise`;
                             return;
                        }
                    }

                    data.spans.push({
                        days_of_week: daysOfWeek,
                        on_trigger: onTrigger,
                        on_time: onTimeInput,
                        off_trigger: offTrigger,
                        off_time: offTimeInput, // Send the potentially corrected '23:59'
                    });
                });

                if (validationError) {
                    alert(validationError);
                    return; // Stop the save
                }

                await scheduleApi.save(data);
                scheduleFormContainer.style.display = 'none'; scheduleListContainer.style.display = 'block'; addScheduleBtn.style.display = 'inline-block';
                loadAllConfigData();
            }
        });
    }

    // --- PLC Output Mapping Manager ---
    const mappingApp = document.getElementById('fsbhoa-mapping-manager-app');
    if (mappingApp) {
        const mappingListContainer = mappingApp.querySelector('#mappings-list-container');
        const mappingFormContainer = mappingApp.querySelector('#mapping-form-container');
        const addNewMappingBtn = mappingApp.querySelector('#add-new-mapping-btn');

        // Pass the required container arguments to the render function
        if(addNewMappingBtn) {
            addNewMappingBtn.addEventListener('click', e => {
                e.preventDefault();
                renderMappingForm(mappingFormContainer, mappingListContainer, addNewMappingBtn);
            });
        }

        mappingApp.addEventListener('click', async e => {
            if(e.target.matches('.edit-mapping-link, .delete-mapping-link, #cancel-mapping-edit-btn')) e.preventDefault();

            if (e.target.matches('.edit-mapping-link')) {
                const mappingId = e.target.dataset.mappingId;
                const mapToEdit = allMappings.find(m => m.id == mappingId);

                // Pass the required container arguments to the render function
                renderMappingForm(mappingFormContainer, mappingListContainer, addNewMappingBtn, mapToEdit);

            } else if (e.target.matches('.delete-mapping-link')) {
                const mappingId = e.target.dataset.mappingId;
                if (confirm('Are you sure?')) { await mappingApi.delete(mappingId); loadAllConfigData(); }
            } else if (e.target.matches('#cancel-mapping-edit-btn')) {
                mappingFormContainer.style.display = 'none'; mappingListContainer.style.display = 'block'; addNewMappingBtn.style.display = 'inline-block';
            } else if (e.target.matches('.test-btn')) {
                e.preventDefault();
                const btn = e.target;
                const id = btn.dataset.id;
                const state = btn.dataset.state;
                const originalText = btn.textContent;

                // Visual feedback
                btn.disabled = true;
                btn.textContent = '...';

                try {
                    const res = await testApi.send(id, state);
                    const json = await res.json();
                    
                    if (res.ok) {
                        btn.textContent = 'OK';
                        console.log('Test successful:', json);
                    } else {
                        throw new Error(json.message || 'Failed');
                    }
                } catch (err) {
                    console.error(err);
                    btn.textContent = 'ERR';
                    alert('Test failed: ' + err.message);
                }

                // Reset button after 1 second
                setTimeout(() => { 
                    btn.disabled = false; 
                    btn.textContent = originalText; 
                }, 1000);
            }
        });

        mappingApp.addEventListener('submit', async e => {
            if (e.target.matches('#mapping-form')) {
                e.preventDefault();
                const formData = new FormData(e.target);
                const data = Object.fromEntries(formData.entries());
                
                // --- THIS IS THE FIX ---
                // We manually grab the raw value from the hidden input.
                // FormData would have given us an escaped and unusable string.
                const coordInput = document.getElementById('map_coordinates_data');
                data.map_coordinates = coordInput ? coordInput.value : '[]';
                
                // We don't want the DUMMY field from FormData
                delete data.map_coordinates_DUMMY; 
                
                await mappingApi.save(data);
                mappingFormContainer.style.display = 'none'; 
                mappingListContainer.style.display = 'block'; 
                addNewMappingBtn.style.display = 'inline-block';
                loadAllConfigData();
            }
        });
    }

    // --- Trigger Initial Load ---
    loadAllConfigData();
});
