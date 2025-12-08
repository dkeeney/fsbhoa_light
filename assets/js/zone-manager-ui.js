// --- RENDER FUNCTIONS (Zones & Schedules) ---

function renderZonesTable(container, saveBtn, allZones, allSchedules) {
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

async function renderZoneForm(formContainer, listContainer, addNewBtn, saveBtn, allMappings, zone = {}) {
    const isEditing = !!zone.id;
    const title = isEditing ? 'Edit Zone' : 'Add New Zone';

    // Re-fetch zones for up-to-date gray-out logic
    const zonesRes = await zoneApi.get(); // zoneApi is global
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
        let labelStyle = 'display: block; margin-bottom: 5px;';
        if (isDisabled) {
            labelStyle += ' color: #999; cursor: not-allowed;'; // 2. Add disabled style
        }
        const disabledAttr = isDisabled ? 'disabled' : '';
        const tooltip = isDisabled ? `title="Already assigned to zone: '${escapeHTML(assignedToZoneName)}'"` : '';
        const outputsText = Array.isArray(map.plc_outputs) ? map.plc_outputs.join(', ') : '';

        return `<label style="${labelStyle}" ${tooltip}><input type="checkbox" name="mapping_ids[]" value="${map.id}" ${isChecked ? 'checked' : ''} ${disabledAttr}> Controller ${map.plc_id}: ${escapeHTML(map.description)} (Outputs: ${outputsText})</label>`;
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

function renderSchedulesTable(container, allSchedules) {
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

function renderSpanRow(span = {}) {
    const days = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
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

function renderScheduleForm(formContainer, listContainer, addNewBtn, schedule = { spans: [] }) {
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
                    <th scope="row" style="vertical-align: top; padding-top: 15px;">
                        Time Spans
                        <div style="font-weight: normal; font-style: italic; color: #666; margin-top: 8px; font-size: 0.85em; line-height: 1.4; border-left: 3px solid #ffb900; padding-left: 8px;">
                            <strong>Note:</strong> Spans cannot cross midnight.<br>
                            To cover overnight (e.g. 8PM to 6AM), please create two spans:<br>
                            1. Start Time to 11:59PM<br>
                            2. 12:00AM to End Time
                        </div>
                    </th>
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

function renderMappingsTable(container, allMappings, allZones, liveStatus) {
    // --- Sort Logic ---
    allMappings.sort((a, b) => {
        const getSortValue = (map) => {
            try {
                const outputs = map.plc_outputs;
                if (outputs && outputs.length > 0) {
                    const firstOutput = outputs[0];
                    const num = parseInt(firstOutput.substring(1), 10);
                    if (isNaN(num)) return 99999;
                    return num;
                }
            } catch (e) {}
            return 99999;
        };
        if (a.plc_id !== b.plc_id) return a.plc_id - b.plc_id;
        return getSortValue(a) - getSortValue(b);
    });

    const tableRows = allMappings.map(map => {
        // --- 1. Schedule Logic ---
        let isSchedActive = false;
        if (map.linked_zone_ids && map.linked_zone_ids.length > 0) {
            const zoneId = map.linked_zone_ids[0];
            const zone = allZones.find(z => z.id == zoneId);
            if (zone && liveStatus[`Sched${zone.schedule_id}`] === true) {
                isSchedActive = true;
            }
        }

        // --- 2. Output Badges (Clean Gray Version) ---
        let monitoredOn = 0;
        let outputBadges = '';
        if (Array.isArray(map.plc_outputs)) {
            outputBadges = map.plc_outputs.map(out => {
                const key = `PLC${map.plc_id}-${out}`;
                if (liveStatus[key] === true) monitoredOn++;
                
                // Compact gray badge
                return `<span style="display:inline-block; margin-right:3px; background:#f0f0f1; border:1px solid #ccc; padding:1px 4px; border-radius:2px; font-family:monospace; font-size:11px; color:#333;">${out}</span>`;
            }).join(' ');
        }

        // --- 3. Bulb Color Logic ---
        let mainClass = '';
        let mainTooltip = '';

        if (monitoredOn > 0) {
            mainClass = isSchedActive ? 'status-auto-on' : 'status-manual-on';
            mainTooltip = 'ON';
        } else {
            mainClass = isSchedActive ? 'status-manual-off' : 'status-auto-off';
            mainTooltip = 'OFF';
        }
        const mainBulb = `<span class="dashicons dashicons-lightbulb monitor-bulb ${mainClass}" title="${mainTooltip}" style="margin-right:8px; font-size:18px; width:18px; height:18px;"></span>`;

        return `
        <tr>
            <td style="white-space:nowrap;">Controller ${map.plc_id}</td>
            <td>
                <div style="display:flex; align-items:center;">
                    ${mainBulb}
                    <strong>${escapeHTML(map.description)}</strong>
                </div>
            </td>
            <td>${outputBadges}</td>
            <td>${Array.isArray(map.relays) ? map.relays.join(', ') : ''}</td>
            <td style="white-space:nowrap;">
                <div style="display: flex; align-items: center; gap: 3px;">
                    <button type="button" class="button micro-btn" data-id="${map.id}" data-state="on" style="color: green; border-color: #46b450;">ON</button>
                    <button type="button" class="button micro-btn" data-id="${map.id}" data-state="off" style="color: red; border-color: #dc3232;">OFF</button>
                    
                    <span style="color:#ddd; margin:0 3px;">|</span>
                    
                    <a href="#" class="edit-mapping-link" data-mapping-id="${map.id}" title="Edit">Edit</a>
                    <span style="color:#ddd; margin:0 3px;">|</span>
                    <a href="#" class="delete-mapping-link" data-mapping-id="${map.id}" title="Delete" style="color: #b32d2e; font-weight:bold;">&times;</a>
                </div>
            </td>
        </tr>
    `;
    }).join('');

    // Inject CSS for compression and Micro Buttons directly here
    container.innerHTML = `
        <style>
            /* Compact Table Styles */
            .fsbhoa-compact-table td, .fsbhoa-compact-table th {
                padding: 4px 6px !important; /* Tight padding */
                vertical-align: middle !important;
                font-size: 12px;
            }
            /* Micro Button Styles */
            .micro-btn {
                padding: 0 5px !important;
                font-size: 10px !important;
                height: 22px !important;
                line-height: 20px !important;
                min-height: 0 !important;
                background: #fff;
            }
            .micro-btn:hover { background: #f6f7f7; }
            .micro-btn:active { transform: translateY(1px); }
        </style>

        <table class="wp-list-table widefat striped fsbhoa-compact-table">
            <thead>
                <tr>
                    <th style="width:80px;">PLC</th>
                    <th>Description</th>
                    <th>Outputs</th>
                    <th>Relays</th>
                    <th style="width:130px;">Actions</th>
                </tr>
            </thead>
            <tbody>${tableRows.length ? tableRows : '<tr><td colspan="5">No mappings found.</td></tr>'}</tbody>
        </table>`;
    
    // Re-attach listeners
    container.querySelectorAll('.test-btn, .micro-btn').forEach(btn => {
        btn.addEventListener('click', async (e) => {
            e.preventDefault();
            const event = new CustomEvent('fsbhoa-test-mapping', { detail: { id: btn.dataset.id, state: btn.dataset.state, btn: btn } });
            document.dispatchEvent(event);
        });
    });
};
