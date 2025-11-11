// --- RENDER FUNCTIONS (Map Editor) ---

function activateMapEditor(existingCoordsJSON) {
    const editor = document.getElementById('map-pin-editor');
    const container = document.getElementById('map-pin-container');
    const palette = document.getElementById('map-pin-palette');
    const dataInput = document.getElementById('map_coordinates_data');
    
    // Check if elements exist
    if (!editor || !container || !palette || !dataInput) {
        console.error('Map editor elements not found! Bailing out.');
        return;
    }
    
    let draggedPin = null;
    let newPinData = null; // Will store { size: "small" }
    let pinStore = [];
    let offset = { x: 0, y: 0 };

    try {
        // This now receives the raw, un-escaped JSON string from the input
        pinStore = JSON.parse(existingCoordsJSON || '[]');
    } catch(e) { console.error("Could not parse pin coords", e, "Raw data:", existingCoordsJSON); pinStore = []; }

    // Function to create a pin element on the map
    const createPinElement = (pinData, index) => {
        const pin = document.createElement('div');
        // pinData is { x: float, y: float, size: "string" }
        pin.className = `map-pin-draggable map-pin-${pinData.size}`;
        pin.style.left = `${pinData.x}%`;
        pin.style.top = `${pinData.y}%`;
        pin.style.transform = 'translate(-50%, -50%)';
        pin.dataset.type = 'existing-pin';
        pin.dataset.index = index; // Store its index in the pinStore
        pin.dataset.size = pinData.size; // Store size for moves
        container.appendChild(pin);
        return pin;
    };
    
    // Function to render all existing pins
    const renderPins = () => {
        container.innerHTML = ''; // Clear old pins
        pinStore.forEach((pinData, index) => {
            createPinElement(pinData, index);
        });
    };

    // Function to update the hidden input field
    const updateDataInput = () => {
        dataInput.value = JSON.stringify(pinStore);
    };

    const onMouseDown = (e) => {
        if (!e.target.classList.contains('map-pin-draggable')) return;
        e.preventDefault(); // Prevent text selection
        
        draggedPin = e.target;
        draggedPin.style.cursor = 'grabbing';
        draggedPin.style.zIndex = '1000';
        
        if (draggedPin.dataset.type === 'new-pin') {
            newPinData = { size: draggedPin.dataset.size }; // Store the size
            
            // Clone the pin from the palette
            const newPin = draggedPin.cloneNode(true);
            newPin.dataset.type = 'existing-pin'; // It's now an "existing" pin
            newPin.style.position = 'absolute';
            
            // Place it on the document, to be positioned by mousemove
            document.body.appendChild(newPin);
            draggedPin = newPin;

            // Calculate offset from cursor
            const rect = draggedPin.getBoundingClientRect();
            offset.x = e.clientX - rect.left - rect.width / 2;
            offset.y = e.clientY - rect.top - rect.height / 2;

        } else {
            // It's an existing pin
            newPinData = null; // Not a new pin
            const rect = draggedPin.getBoundingClientRect();
            offset.x = e.clientX - rect.left - rect.width / 2;
            offset.y = e.clientY - rect.top - rect.height / 2;
            
            // Temporarily attach to body to avoid clipping
            draggedPin.style.position = 'absolute'; // Ensure it's absolute
            document.body.appendChild(draggedPin);
        }
        
        // Move pin to initial cursor position
        draggedPin.style.left = `${e.clientX - offset.x}px`;
        draggedPin.style.top = `${e.clientY - offset.y}px`;

        document.addEventListener('mousemove', onMouseMove);
        document.addEventListener('mouseup', onMouseUp);
    };

    const onMouseMove = (e) => {
        if (!draggedPin) return;
        draggedPin.style.left = `${e.clientX - offset.x}px`;
        draggedPin.style.top = `${e.clientY - offset.y}px`;
    };

    const onMouseUp = (e) => {
        if (!draggedPin) return;
        
        draggedPin.style.cursor = 'grab';
        draggedPin.style.zIndex = '10';

        const mapRect = editor.getBoundingClientRect();
        const paletteRect = palette.getBoundingClientRect();
        
        const isOverMap = (e.clientX >= mapRect.left && e.clientX <= mapRect.right && e.clientY >= mapRect.top && e.clientY <= mapRect.bottom);
        const isOverPalette = (e.clientX >= paletteRect.left && e.clientX <= paletteRect.right && e.clientY >= paletteRect.top && e.clientY <= paletteRect.bottom);

        if (isOverMap) {
            // --- Dropped on the Map ---
            // Calculate percentage-based position
            const x = ((e.clientX - mapRect.left) / mapRect.width) * 100;
            const y = ((e.clientY - mapRect.top) / mapRect.height) * 100;
            
            if (newPinData) {
                // Add new pin object to store
                pinStore.push({ x: x, y: y, size: newPinData.size });
            } else {
                // Update existing pin's position in store
                const index = parseInt(draggedPin.dataset.index, 10);
                if (pinStore[index]) {
                    pinStore[index].x = x;
                    pinStore[index].y = y;
                }
                // Note: size is not changeable by dragging, only by re-dragging from palette
            }
        } else if (isOverPalette) {
            // --- Dropped on the Palette (Delete) ---
            if (!newPinData) {
                // Remove from store
                const index = parseInt(draggedPin.dataset.index, 10);
                pinStore.splice(index, 1);
            }
            draggedPin.remove(); // Remove the element
        } else {
            // --- Dropped somewhere else (Cancel) ---
            if (newPinData) {
                draggedPin.remove(); // Just delete the new pin
            }
            // Existing pins will be re-rendered in their original spot
        }

        // Re-render all pins from the store
        renderPins();
        // Update the hidden input
        updateDataInput();

        // Clean up
        if (draggedPin.parentElement === document.body) {
            // This will fail if it was removed, which is fine
            try { draggedPin.remove(); } catch(err) {}
        }
        draggedPin = null;
        newPinData = null;
        document.removeEventListener('mousemove', onMouseMove);
        document.removeEventListener('mouseup', onMouseUp);
    };
    
    // Initial setup
    renderPins();
    
    // Add listener to the palette and the pin container
    palette.addEventListener('mousedown', onMouseDown);
    container.addEventListener('mousedown', onMouseDown);
}

function renderMappingForm(formContainer, listContainer, addNewBtn, map = {}) {
    const isEditing = !!map.id;
    const title = isEditing ? 'Edit Mapping' : 'Add New Mapping';

    // Check if map_image_url is available
    const mapImageUrl = fsbhoa_lighting_data.map_image_url || '';
    if (mapImageUrl === '') {
        console.error('Map image URL is missing from fsbhoa_lighting_data.');
    }
    const coordsValue = JSON.stringify(map.map_coordinates || []).replace(/'/g, '&#39;');


    const mapEditorHTML = `
        <tr class="form-field">
            <th scope="row" style="vertical-align: top; padding-top: 12px;">Light Pin Locations</th>
            <td style="padding-top: 10px;">
                <style>
                    /* Add some styles for the pins */
                    .map-pin-draggable {
                        border-radius: 50%;
                        cursor: grab;
                        border: 2px solid white;
                        box-shadow: 0 1px 3px rgba(0,0,0,0.5);
                        user-select: none; /* Prevent text selection */
                    }
                    .map-pin-small  { width: 14px; height: 14px; background: #d9534f; }
                    .map-pin-medium { width: 20px; height: 20px; background: #f0ad4e; }
                    .map-pin-large  { width: 26px; height: 26px; background: #007cba; }
                    #map-pin-palette .map-pin-draggable { position: relative; }
                    #map-pin-container .map-pin-draggable { position: absolute; }
                </style>
                <div id="map-pin-editor" style="position: relative; border: 2px solid #ccc; max-width: 800px; min-height: 400px; background: #f4f4f4; overflow: hidden;">
                    <img id="map-editor-image" src="${escapeHTML(mapImageUrl)}" style="display: block; width: 100%; height: auto; user-select: none;">
                    <div id="map-pin-container" style="position: absolute; top: 0; left: 0; width: 100%; height: 100%;"></div>
                </div>
                <div id="map-pin-palette" style="border: 1px dashed #999; padding: 10px; margin-top: 10px; min-height: 50px; background: #f9f9f9; display: flex; align-items: center; gap: 15px; max-width: 800px; box-sizing: border-box;">
                    <span style="user-select: none;">Drag from here:</span>
                    <div class="map-pin-draggable map-pin-small"  data-type="new-pin" data-size="small"  title="Small (e.g., Accent)"></div>
                    <div class="map-pin-draggable map-pin-medium" data-type="new-pin" data-size="medium" title="Medium (e.g., Pathway)"></div>
                    <div class="map-pin-draggable map-pin-large"  data-type="new-pin" data-size="large"  title="Large (e.g., Parking Lot)"></div>
                    <span style="margin-left: auto; font-style: italic; color: #555; user-select: none;">(Drag pins back here to delete)</span>
                </div>
            </td>
        </tr>
    `;

    formContainer.innerHTML = `
        <h2>${title}</h2>
        <form id="mapping-form">
            <input type="hidden" name="mapping_id" value="${map.id || 0}">
            
            <input type="hidden" id="map_coordinates_data" name="map_coordinates_DUMMY" value='${coordsValue}'>
            
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
                    <td><input type="text" id="map-plc-outputs" name="plc_outputs" class="regular-text" 
                         value="${Array.isArray(map.plc_outputs) ? map.plc_outputs.join(',') : ''}" required>
                    </td>
                </tr>
                <tr>
                    <th scope="row"><label for="map-relays">Relays (comma-separated)</label></th>
                    <td><input type="text" id="map-relays" name="relays" class="regular-text" 
                               value="${Array.isArray(map.relays) ? map.relays.join(',') : ''}" required>
                    </td>
                </tr>
                
                ${mapEditorHTML}
                
            </table>
            <button type="submit" class="button button-primary">Save Mapping</button>
            <button type="button" class="button" id="cancel-mapping-edit-btn">Cancel</button>
        </form>
    `;
    listContainer.style.display = 'none';
    addNewBtn.style.display = 'none';
    formContainer.style.display = 'block';

    const hiddenInput = document.getElementById('map_coordinates_data');
    activateMapEditor(hiddenInput.value);
};
