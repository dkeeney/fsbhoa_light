document.addEventListener('DOMContentLoaded', function () {
    const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';

    // =================================================================
    // ZONE MANAGER
    // =================================================================
    const zoneApp = document.getElementById('fsbhoa-zone-manager-app');
    if (zoneApp) {
        const listContainer = zoneApp.querySelector('#zones-list-container');
        const formContainer = zoneApp.querySelector('#zone-form-container');
        const addNewBtn = zoneApp.querySelector('#add-new-zone-btn');

        const zoneApi = {
            get: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/zones', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
            save: (data) => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/zones', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'X-WP-Nonce': fsbhoa_lighting_data.nonce },
                body: JSON.stringify(data)
            }),
            delete: (zoneId) => fetch(fsbhoa_lighting_data.rest_url + `fsbhoa-lighting/v1/zones/${zoneId}`, {
                method: 'DELETE',
                headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce }
            })
        };
        
        const mappingApi = {
            get: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/mappings', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } })
        };

        const renderZonesTable = (zones) => {
            const tableRows = zones.map(zone => `
                <tr>
                    <td><strong>${escapeHTML(zone.zone_name)}</strong></td>
                    <td>${escapeHTML(zone.description)}</td>
                    <td>
                        <a href="#" class="edit-zone-link" data-zone-id="${zone.id}">Edit</a> |
                        <a href="#" class="delete-zone-link" data-zone-id="${zone.id}" style="color: #b32d2e;">Delete</a>
                    </td>
                </tr>
            `).join('');

            listContainer.innerHTML = `
                <table class="wp-list-table widefat striped">
                    <thead><tr><th style="width: 30%;">Zone Name</th><th>Description</th><th style="width: 15%;">Actions</th></tr></thead>
                    <tbody>${tableRows}</tbody>
                </table>
            `;
        };

const renderZoneForm = async (zone = {}) => {
            const isEditing = !!zone.id;
            const title = isEditing ? 'Edit Zone' : 'Add New Zone';
            
            // Fetch all zones and all mappings at the same time
            const [zonesResponse, mappingsResponse] = await Promise.all([ zoneApi.get(), mappingApi.get() ]);
            const allZones = await zonesResponse.json();
            const allMappings = await mappingsResponse.json();
            
            const mappingsChecklistHTML = allMappings.map(map => {
                const isChecked = zone.mapping_ids && zone.mapping_ids.includes(map.id.toString());
                
                // Check if this mapping is used by any OTHER zone
                let isDisabled = false;
                let assignedToZoneName = '';
                for (const otherZone of allZones) {
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

                return `<label ${labelStyle} ${tooltip}><input type="checkbox" name="mapping_ids[]" value="${map.id}" ${isChecked ? 'checked' : ''} ${disabledAttr}> Controller ${map.plc_id}: ${escapeHTML(map.description)} (Outputs: ${JSON.parse(map.plc_outputs).join(', ')})</label><br>`;
            }).join('');

            formContainer.innerHTML = `
                <h2>${title}</h2>
                <form id="zone-form">
                    <input type="hidden" name="zone_id" value="${zone.id || 0}">
                    <table class="form-table">
                        <tr valign="top">
                            <th scope="row"><label for="zone_name">Zone Name</label></th>
                            <td><input type="text" id="zone_name" name="zone_name" class="regular-text" value="${escapeHTML(zone.zone_name || '')}" required></td>
                        </tr>
                        <tr valign="top">
                            <th scope="row"><label for="description">Description</label></th>
                            <td><textarea id="description" name="description" class="large-text" rows="4">${escapeHTML(zone.description || '')}</textarea></td>
                        </tr>
                        <tr valign="top">
                            <th scope="row">Assign PLC Output Mappings</th>
                            <td>${mappingsChecklistHTML || 'No mappings defined yet. Please add mappings below first.'}</td>
                        </tr>
                    </table>
                    <button type="submit" class="button button-primary">Save Zone</button>
                    <button type="button" class="button" id="cancel-edit-btn">Cancel</button>
                </form>
            `;
            listContainer.style.display = 'none';
            addNewBtn.style.display = 'none';
            formContainer.style.display = 'block';
        };

        const loadZones = async () => {
            try {
                const response = await zoneApi.get();
                const zones = await response.json();
                renderZonesTable(zones);
            } catch (error) { console.error('Error loading zones:', error); }
        };

        addNewBtn.addEventListener('click', (e) => { e.preventDefault(); renderZoneForm(); });

        zoneApp.addEventListener('click', async (e) => {
            if (e.target.matches('.edit-zone-link, .delete-zone-link, #cancel-edit-btn')) {
                e.preventDefault();
            }
            if (e.target.matches('.edit-zone-link')) {
                const zoneId = e.target.dataset.zoneId;
                const response = await zoneApi.get();
                const zones = await response.json();
                const zoneToEdit = zones.find(z => z.id == zoneId);
                renderZoneForm(zoneToEdit);
            } else if (e.target.matches('.delete-zone-link')) {
                const zoneId = e.target.dataset.zoneId;
                if (confirm('Are you sure?')) {
                    await zoneApi.delete(zoneId);
                    loadZones();
                }
            } else if (e.target.matches('#cancel-edit-btn')) {
                formContainer.style.display = 'none';
                listContainer.style.display = 'block';
                addNewBtn.style.display = 'inline-block';
            }
        });

        zoneApp.addEventListener('submit', async (e) => {
            if (e.target.matches('#zone-form')) {
                e.preventDefault();
                const formData = new FormData(e.target);
                const data = Object.fromEntries(formData.entries());
                data.mapping_ids = formData.getAll('mapping_ids[]');
                await zoneApi.save(data);
                formContainer.style.display = 'none';
                listContainer.style.display = 'block';
                addNewBtn.style.display = 'inline-block';
                loadZones();
            }
        });

        loadZones();
    }

    // =================================================================
    // PLC OUTPUT MAPPING MANAGER
    // =================================================================
    const mappingApp = document.getElementById('fsbhoa-mapping-manager-app');
    if (mappingApp) {
        const mappingListContainer = mappingApp.querySelector('#mappings-list-container');
        const mappingFormContainer = mappingApp.querySelector('#mapping-form-container');
        const addNewMappingBtn = mappingApp.querySelector('#add-new-mapping-btn');

        const mappingApi = {
            get: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/mappings', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
            save: (data) => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/mappings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'X-WP-Nonce': fsbhoa_lighting_data.nonce },
                body: JSON.stringify(data)
            }),
            delete: (mappingId) => fetch(fsbhoa_lighting_data.rest_url + `fsbhoa-lighting/v1/mappings/${mappingId}`, {
                method: 'DELETE',
                headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce }
            })
        };

        const renderMappingsTable = (mappings) => {
            const tableRows = mappings.map(map => `
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
            mappingListContainer.innerHTML = `
                <table class="wp-list-table widefat striped">
                    <thead><tr><th>Controller</th><th>Description</th><th>PLC Outputs</th><th>Relays</th><th>Actions</th></tr></thead>
                    <tbody>${tableRows}</tbody>
                </table>`;
        };
        
        const renderMappingForm = (map = {}) => {
            const isEditing = !!map.id;
            const title = isEditing ? 'Edit Mapping' : 'Add New Mapping';
            mappingFormContainer.innerHTML = `
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
            mappingListContainer.style.display = 'none';
            addNewMappingBtn.style.display = 'none';
            mappingFormContainer.style.display = 'block';
        };

        const loadMappings = async () => {
            try {
                const response = await mappingApi.get();
                const mappings = await response.json();
                renderMappingsTable(mappings);
            } catch(e) { console.error('Error loading mappings:', e); }
        };

        addNewMappingBtn.addEventListener('click', e => { e.preventDefault(); renderMappingForm(); });

        mappingApp.addEventListener('click', async e => {
            if(e.target.matches('.edit-mapping-link, .delete-mapping-link, #cancel-mapping-edit-btn')) {
                e.preventDefault();
            }
            if (e.target.matches('.edit-mapping-link')) {
                const mappingId = e.target.dataset.mappingId;
                const response = await mappingApi.get();
                const mappings = await response.json();
                const mapToEdit = mappings.find(m => m.id == mappingId);
                renderMappingForm(mapToEdit);
            } else if (e.target.matches('.delete-mapping-link')) {
                const mappingId = e.target.dataset.mappingId;
                if (confirm('Are you sure?')) {
                    await mappingApi.delete(mappingId);
                    loadMappings();
                }
            } else if (e.target.matches('#cancel-mapping-edit-btn')) {
                mappingFormContainer.style.display = 'none';
                mappingListContainer.style.display = 'block';
                addNewMappingBtn.style.display = 'inline-block';
            }
        });

        mappingApp.addEventListener('submit', async e => {
            if (e.target.matches('#mapping-form')) {
                e.preventDefault();
                const formData = new FormData(e.target);
                const data = Object.fromEntries(formData.entries());
                await mappingApi.save(data);
                mappingFormContainer.style.display = 'none';
                mappingListContainer.style.display = 'block';
                addNewMappingBtn.style.display = 'inline-block';
                loadMappings();
            }
        });

        loadMappings();
    }
});
