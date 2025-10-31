document.addEventListener('DOMContentLoaded', function () {
    // --- Helper Functions ---
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

    // --- API Definitions ---
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

    // --- Global Data Store ---
    let allZones = [];
    let allMappings = [];
    let allSchedules = [];

    // =================================================================
    // RENDER FUNCTIONS (Defined in global scope)
    // =================================================================

    const renderZonesTable = (container, saveBtn) => {
        const tableRows = allZones.map(zone => {
            const assignedScheduleId = zone.schedule_id || 0;
            return `
                <tr>
                    <td><strong>${escapeHTML(zone.zone_name)}</strong></td>
                    <td>${escapeHTML(zone.description)}</td>
                    <td>
                        <select class="zone-schedule-select" data-zone-id="${zone.id}" style="width: 100%;">
                            <option value="0">-- None --</option>
                            ${allSchedules.map(s => `<option value="${s.id}" ${assignedScheduleId == s.id ? 'selected' : ''}>${escapeHTML(s.schedule_name)}</option>`).join('')}
                        </select>
                    </td>
                    <td>
                        <a href="#" class="edit-zone-link" data-zone-id="${zone.id}">Edit Details</a> |
                        <a href="#" class="delete-zone-link" data-zone-id="${zone.id}" style="color: #b32d2e;">Delete</a>
                    </td>
                </tr>`;
        }).join('');
        container.innerHTML = `
            <table class="wp-list-table widefat striped fixed">
                <thead><tr><th style="width: 25%;">Zone Name</th><th>Description</th><th style="width: 25%;">Assigned Schedule</th><th style="width: 15%;">Actions</th></tr></thead>
                <tbody>${tableRows.length ? tableRows : '<tr><td colspan="4">No zones found. Click "Add New Zone" to get started.</td></tr>'}</tbody>
            </table>`;
        saveBtn.style.display = 'none'; // Hide the old save button
    };

    const renderZoneForm = async (formContainer, listContainer, addNewBtn, saveBtn, zone = {}) => {
        const isEditing = !!zone.id;
        const title = isEditing ? 'Edit Zone' : 'Add New Zone';
        
        // Re-fetch zones for up-to-date gray-out logic
        const zonesRes = await zoneApi.get();
        const currentZones = await zonesRes.json();
        
        const mappingsChecklistHTML = allMappings.map(map => {
            const isChecked = zone.mapping_ids && zone.mapping_ids.includes(map.id.toString());
            let isDisabled = false;
            let assignedToZoneName = '';
            for (const otherZone of currentZones) {
                if (otherZone.id === zone.id) continue;
                if (otherZone.mapping_ids && otherZone.mapping_ids.includes(map.id.toString())) {
                    isDisabled = true;
                    assignedToZoneName = otherZone.zone_name;
                    break;
                }
            }
            const disabledAttr = isDisabled ? 'disabled' : '';
            const labelStyle = isDisabled ? 'style="color: #999; cursor: not-allowed;"' : '';
            const tooltip = isDisabled ? `title="Already assigned to zone: '${escapeHTML(assignedToZoneName)}'"` : '';
            return `<label ${labelStyle} ${tooltip} style="display: block; margin-bottom: 5px;"><input type="checkbox" name="mapping_ids[]" value="${map.id}" ${isChecked ? 'checked' : ''} ${disabledAttr}> Controller ${map.plc_id}: ${escapeHTML(map.description)} (Outputs: ${JSON.parse(map.plc_outputs).join(', ')})</label>`;
        }).join('');

        formContainer.innerHTML = `
            <h2>${title}</h2>
            <form id="zone-form">
                <input type="hidden" name="zone_id" value="${zone.id || 0}">
                <table class="form-table"><tbody>
                    <tr class="form-field form-required">
                        <th scope="row"><label for="zone_name">Zone Name</label></th>
                        <td><input name="zone_name" type="text" id="zone_name" value="${escapeHTML(zone.zone_name || '')}" class="regular-text" required></td>
                    </tr>
                    <tr class="form-field">
                        <th scope="row"><label for="description">Description</label></th>
                        <td><input name="description" type="text" id="description" value="${escapeHTML(zone.description || '')}" class="large-text"></td>
                    </tr>
                    <tr class="form-field">
                        <th scope="row" style="vertical-align: top; padding-top: 12px;">Assign PLC Output Mappings</th>
                        <td style="padding-top: 10px;">${mappingsChecklistHTML || 'No mappings defined yet. Please add mappings below first.'}</td>
                    </tr>
                </tbody></table>
                <p class="submit">
                    <button type="submit" class="button button-primary">Save Zone</button>
                    <button type="button" class="button" id="cancel-edit-btn" style="margin-left: 10px;">Cancel</button>
                </p>
            </form>`;
        listContainer.style.display = 'none';
        addNewBtn.style.display = 'none';
        saveBtn.style.display = 'none';
        formContainer.style.display = 'block';
    };

    const renderSchedulesTable = (container) => {
        const rows = allSchedules.map(s => {
            const spansSummary = s.spans.map(span => {
                const days = (span.days_of_week && span.days_of_week.length > 0) ? span.days_of_week.join(', ') : 'No days selected';
                return `[${span.on_trigger === 'TIME' ? formatTime(span.on_time) : 'Sundown'} - ${span.off_trigger === 'TIME' ? formatTime(span.off_time) : 'Sunrise'}] on ${days}`;
            }).join('<br>');
            return `
                <tr>
                    <td><strong>${escapeHTML(s.schedule_name)}</strong></td>
                    <td>${spansSummary}</td>
                    <td>
                        <a href="#" class="edit-schedule-link" data-schedule-id="${s.id}">Edit</a> |
                        <a href="#" class="delete-schedule-link" data-schedule-id="${s.id}" style="color: #b32d2e;">Delete</a>
                    </td>
                </tr>`;
        }).join('');
        container.innerHTML = `
            <table class="wp-list-table widefat striped" style="margin-top:20px;">
                <thead><tr><th>Schedule Name</th><th>Time Spans</th><th>Actions</th></tr></thead>
                <tbody>${rows.length ? rows : '<tr><td colspan="3">No schedules found. Click "Add New Schedule" to get started.</td></tr>'}</tbody>
            </table>
        `;
    };

    const renderSpanRow = (span = {}) => {
        const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
        const dayCheckboxes = days.map(day => {
            const isChecked = span.days_of_week && span.days_of_week.includes(day);
            return `<label title="${day}" style="margin: 0 4px; font-size: 0.9em; white-space: nowrap;"><input type="checkbox" name="days_of_week" value="${day}" ${isChecked ? 'checked' : ''}>${day.substring(0,1)}</label>`;
        }).join('');
        const controlStyle = "padding: 2px 4px; font-size: 0.9em; height: auto; line-height: normal;";
        return `
            <div class="schedule-span-row" style="margin-bottom: 10px; padding: 10px; border: 1px solid #ddd; background: #f9f9f9; border-radius: 4px; display: flex; align-items: center; justify-content: space-between; gap: 8px;">
                <div class="days-of-week" style="border-right: 1px solid #eee; padding-right: 8px; white-space: nowrap;">
                   <span style="font-weight: bold; margin-right: 5px;">Days:</span> ${dayCheckboxes}
                </div>
                <div style="display: flex; align-items: center; gap: 4px;"> 
                    <span style="font-weight: bold;">On:</span>
                    <select name="on_trigger" style="flex-shrink: 0; width: 100px; ${controlStyle}">
                        <option value="SUNDOWN" ${span.on_trigger === 'SUNDOWN' ? 'selected' : ''}>Sundown</option>
                        <option value="TIME" ${span.on_trigger === 'TIME' ? 'selected' : ''}>Time</option>
                    </select>
                    <input type="time" name="on_time" value="${span.on_time ? span.on_time.substring(0, 5) : ''}" style="${span.on_trigger !== 'TIME' ? 'display:none;' : ''} ${controlStyle}">
                </div>
                <div style="display: flex; align-items: center; gap: 4px;"> 
                     <span style="font-weight: bold;">Off:</span>
                    <select name="off_trigger" style="flex-shrink: 0; width: 100px; ${controlStyle}">
                        <option value="SUNRISE" ${span.off_trigger === 'SUNRISE' ? 'selected' : ''}>Sunrise</option>
                        <option value="TIME" ${span.off_trigger === 'TIME' ? 'selected' : ''}>Time</option>
                    </select>
                    <input type="time" name="off_time" value="${span.off_time ? span.off_time.substring(0, 5) : ''}" style="${span.off_trigger !== 'TIME' ? 'display:none;' : ''} ${controlStyle}">
                 </div>
                <button type="button" class="button remove-span-btn" style="margin-left: auto; padding: 0 8px; line-height: 1.5;">&times;</button> 
            </div>
        `;
    };

    const renderScheduleForm = (formContainer, listContainer, addNewBtn, schedule = { spans: [] }) => {
        const title = schedule.id ? 'Edit Schedule' : 'Add New Schedule';
        const spanRowsHTML = schedule.spans.length > 0 ? schedule.spans.map(renderSpanRow).join('') : renderSpanRow();
        formContainer.innerHTML = `
            <h2>${title}</h2>
            <form id="schedule-form">
                <input type="hidden" name="schedule_id" value="${schedule.id || 0}">
                <table class="form-table">
                    <tr>
                        <th scope="row"><label for="schedule_name">Schedule Name</label></th>
                        <td><input type="text" name="schedule_name" value="${escapeHTML(schedule.schedule_name || '')}" class="regular-text" required></td>
                    </tr>
                    <tr>
                        <th scope="row">Time Spans</th>
                        <td id="schedule-spans-container">${spanRowsHTML}</td>
                    </tr>
                </table>
                <button type="button" id="add-span-btn" class="button">+ Add Time Span</button>
                <hr style="margin: 20px 0;">
                <button type="submit" class="button button-primary">Save Schedule</button>
                <button type="button" id="cancel-btn" class="button">Cancel</button>
            </form>
        `;
        listContainer.style.display = 'none';
        addNewBtn.style.display = 'none';
        formContainer.style.display = 'block';
    };
    
    const renderMappingsTable = (container) => {
        allMappings.sort((a, b) => {
            const getSortValue = (map) => {
                try { const outputs = JSON.parse(map.plc_outputs); if (outputs && outputs.length > 0) return parseInt(outputs[0].substring(1), 10); } catch (e) {}
                return 0;
            };
            if (a.plc_id !== b.plc_id) return a.plc_id - b.plc_id;
            return getSortValue(a) - getSortValue(b);
        });
        const tableRows = allMappings.map(map => `
            <tr>
                <td>Controller ${map.plc_id}</td>
                <td><strong>${escapeHTML(map.description)}</strong></td>
                <td>${JSON.parse(map.plc_outputs).join(', ')}</td>
                <td>${JSON.parse(map.relays).join(', ')}</td>
                <td>
                    <a href="#" class="edit-mapping-link" data-mapping-id="${map.id}">Edit</a> |
                    <a href="#" class="delete-mapping-link" data-mapping-id="${map.id}" style="color: #b32d2e;">Delete</a>
                </td>
            </tr>
        `).join('');
        container.innerHTML = `
            <table class="wp-list-table widefat striped">
                <thead><tr><th>Controller</th><th>Description</th><th>PLC Outputs</th><th>Relays</th><th>Actions</th></tr></thead>
                <tbody>${tableRows.length ? tableRows : '<tr><td colspan="5">No mappings found. Click "Add New Mapping" to get started.</td></tr>'}</tbody>
            </table>`;
    };

    const renderMappingForm = (formContainer, listContainer, addNewBtn, map = {}) => {
        const isEditing = !!map.id;
        const title = isEditing ? 'Edit Mapping' : 'Add New Mapping';
        formContainer.innerHTML = `
            <h2>${title}</h2>
            <form id="mapping-form">
                <input type="hidden" name="mapping_id" value="${map.id || 0}">
                <table class="form-table">
                    <tr>
                        <th scope="row"><label for="map-plc-id">Controller</label></th>
                        <td>
                            <select id="map-plc-id" name="plc_id">
                                <option value="1" ${map.plc_id == 1 ? 'selected' : ''}>Controller #1 (Lodge)</option>
                                <option value="2" ${map.plc_id == 2 ? 'selected' : ''}>Controller #2 (Pool House)</option>
                            </select>
                        </td>
                    </tr>
                    <tr>
                        <th scope="row"><label for="map-description">Description</label></th>
                        <td><input type="text" id="map-description" name="description" class="regular-text" value="${escapeHTML(map.description || '')}" required></td>
                    </tr>
                    <tr>
                        <th scope="row"><label for="map-plc-outputs">PLC Outputs (comma-separated)</label></th>
                        <td><input type="text" id="map-plc-outputs" name="plc_outputs" class="regular-text" value="${map.plc_outputs ? JSON.parse(map.plc_outputs).join(',') : ''}" required></td>
                    </tr>
                    <tr>
                        <th scope="row"><label for="map-relays">Relays (comma-separated)</label></th>
                        <td><input type="text" id="map-relays" name="relays" class="regular-text" value="${map.relays ? JSON.parse(map.relays).join(',') : ''}" required></td>
                    </tr>
                </table>
                <button type="submit" class="button button-primary">Save Mapping</button>
                <button type="button" class="button" id="cancel-mapping-edit-btn">Cancel</button>
            </form>
        `;
        listContainer.style.display = 'none';
        addNewBtn.style.display = 'none';
        formContainer.style.display = 'block';
    };


    // =================================================================
    // INITIAL LOAD FUNCTION
    // =================================================================
    const loadAllConfigData = async () => {
        const zoneApp = document.getElementById('fsbhoa-zone-manager-app');
        const scheduleApp = document.getElementById('fsbhoa-schedules-app');
        const mappingApp = document.getElementById('fsbhoa-mapping-manager-app');
        
        try {
            console.log("Loading all config data...");
            const [zonesRes, mappingsRes, schedulesRes] = await Promise.all([
                zoneApi.get(), mappingApi.get(), scheduleApi.get()
            ]);

            if (!zonesRes.ok) throw new Error(`Failed loading zones: ${zonesRes.statusText}`);
            if (!mappingsRes.ok) throw new Error(`Failed loading mappings: ${mappingsRes.statusText}`);
            if (!schedulesRes.ok) throw new Error(`Failed loading schedules: ${schedulesRes.statusText}`);

            allZones = await zonesRes.json();
            allMappings = await mappingsRes.json();
            allSchedules = await schedulesRes.json();
            
            if (zoneApp) renderZonesTable(zoneApp.querySelector('#zones-list-container'), zoneApp.querySelector('#save-zone-assignments-btn'));
            if (scheduleApp) renderSchedulesTable(scheduleApp.querySelector('#schedules-list-container'));
            if (mappingApp) renderMappingsTable(mappingApp.querySelector('#mappings-list-container'));
            
            console.log("All config data loaded and rendered.");
        } catch (error) {
            console.error('Error loading initial configuration data:', error);
            const errorMsg = '<p style="color: red;">Error loading configuration. Check console and ensure Go service is running.</p>';
            if (zoneApp) zoneApp.querySelector('#zones-list-container').innerHTML = errorMsg;
            if (scheduleApp) scheduleApp.querySelector('#schedules-list-container').innerHTML = errorMsg;
            if (mappingApp) mappingApp.querySelector('#mappings-list-container').innerHTML = errorMsg;
        }
    };

    // =================================================================
    // ATTACH EVENT LISTENERS
    // =================================================================

    // --- Zone Manager ---
    const zoneApp = document.getElementById('fsbhoa-zone-manager-app');
    if (zoneApp) {
        const listContainer = zoneApp.querySelector('#zones-list-container');
        const formContainer = zoneApp.querySelector('#zone-form-container');
        const addNewBtn = zoneApp.querySelector('#add-new-zone-btn');
        const saveAssignmentsBtn = zoneApp.querySelector('#save-zone-assignments-btn');

        addNewBtn.addEventListener('click', (e) => { e.preventDefault(); renderZoneForm(formContainer, listContainer, addNewBtn, saveAssignmentsBtn); });

        zoneApp.addEventListener('click', async (e) => {
            if (e.target.matches('.edit-zone-link, .delete-zone-link, #cancel-edit-btn, #save-zone-assignments-btn')) e.preventDefault();
            
            if (e.target.matches('.edit-zone-link')) {
                const zoneId = e.target.dataset.zoneId;
                const zoneToEdit = allZones.find(z => z.id == zoneId);
                renderZoneForm(formContainer, listContainer, addNewBtn, saveAssignmentsBtn, zoneToEdit);
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
        
        addScheduleBtn.addEventListener('click', e => { e.preventDefault(); renderScheduleForm(scheduleFormContainer, scheduleListContainer, addScheduleBtn); });

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
                document.querySelectorAll('.schedule-span-row').forEach(row => {
                    const daysOfWeek = Array.from(row.querySelectorAll('input[name="days_of_week"]:checked')).map(cb => cb.value);
                    data.spans.push({
                        days_of_week: daysOfWeek,
                        on_trigger: row.querySelector('[name="on_trigger"]').value,
                        on_time: row.querySelector('[name="on_time"]').value,
                        off_trigger: row.querySelector('[name="off_trigger"]').value,
                        off_time: row.querySelector('[name="off_time"]').value,
                    });
                });
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
        addNewMappingBtn.addEventListener('click', e => { 
            e.preventDefault(); 
            renderMappingForm(mappingFormContainer, mappingListContainer, addNewMappingBtn); 
        });

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
            }
        });

        mappingApp.addEventListener('submit', async e => {
            if (e.target.matches('#mapping-form')) {
                e.preventDefault();
                const formData = new FormData(e.target);
                const data = Object.fromEntries(formData.entries());
                await mappingApi.save(data);
                mappingFormContainer.style.display = 'none'; mappingListContainer.style.display = 'block'; addNewMappingBtn.style.display = 'inline-block';
                loadAllConfigData();
            }
        });
    }

    // --- Trigger Initial Load ---
    loadAllConfigData();
});
