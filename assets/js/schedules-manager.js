document.addEventListener('DOMContentLoaded', function () {
    const escapeHTML = (str) => str ? str.toString().replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#039;') : '';
    const formatTime = (timeStr) => {
        if (!timeStr || !timeStr.includes(':')) return '';
        let [hours, minutes] = timeStr.split(':');
        hours = parseInt(hours, 10);
        const ampm = hours >= 12 ? 'PM' : 'AM';
        hours = hours % 12;
        hours = hours ? hours : 12;
        return `${hours}:${minutes} ${ampm}`;
    };

    // =================================================================
    // SCHEDULES MANAGER
    // =================================================================
    const app = document.getElementById('fsbhoa-schedules-app');
    if (app) {
        const listContainer = app.querySelector('#schedules-list-container');
        const formContainer = app.querySelector('#schedule-form-container');
        const addNewBtn = app.querySelector('#add-new-schedule-btn');

        const api = {
            get: () => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/schedules', { headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce } }),
            save: (data) => fetch(fsbhoa_lighting_data.rest_url + 'fsbhoa-lighting/v1/schedules', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json', 'X-WP-Nonce': fsbhoa_lighting_data.nonce },
                body: JSON.stringify(data)
            }),
            delete: (id) => fetch(fsbhoa_lighting_data.rest_url + `fsbhoa-lighting/v1/schedules/${id}`, {
                method: 'DELETE',
                headers: { 'X-WP-Nonce': fsbhoa_lighting_data.nonce }
            })
        };

        const renderTable = (schedules) => {
            const rows = schedules.map(s => {
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
                    </tr>
                `;
            }).join('');
            listContainer.innerHTML = `
                <table class="wp-list-table widefat striped" style="margin-top:20px;">
                    <thead><tr><th>Schedule Name</th><th>Time Spans</th><th>Actions</th></tr></thead>
                    <tbody>${rows}</tbody>
                </table>
            `;
        };

// Helper function to render a single time span row
    const renderSpanRow = (span = {}) => {
        const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
        const dayCheckboxes = days.map(day => {
            const isChecked = span.days_of_week && span.days_of_week.includes(day);
            return `<label title="${day}" style="margin: 0 4px; font-size: 0.9em; white-space: nowrap;"><input type="checkbox" name="days_of_week" value="${day}" ${isChecked ? 'checked' : ''}>${day.substring(0,1)}</label>`;
        }).join('');

        // Define styles for compact controls
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
        
        const renderForm = (schedule = { spans: [] }) => {
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

        const loadSchedules = async () => {
            try {
                const response = await api.get();
                const schedules = await response.json();
                renderTable(schedules);
            } catch (error) { console.error('Error loading schedules:', error); }
        };

        addNewBtn.addEventListener('click', e => { e.preventDefault(); renderForm(); });

        app.addEventListener('change', e => {
            if (e.target.matches('select[name="on_trigger"], select[name="off_trigger"]')) {
                e.target.nextElementSibling.style.display = e.target.value === 'TIME' ? 'inline-block' : 'none';
            }
        });

        app.addEventListener('click', async e => {
            if (e.target.matches('.edit-schedule-link, .delete-schedule-link, .remove-span-btn, #add-span-btn, #cancel-btn')) {
                e.preventDefault();
            }

            if (e.target.matches('#cancel-btn')) {
                formContainer.style.display = 'none';
                listContainer.style.display = 'block';
                addNewBtn.style.display = 'inline-block';
            } else if (e.target.matches('.edit-schedule-link')) {
                const id = e.target.dataset.scheduleId;
                const response = await api.get();
                const schedules = await response.json();
                const scheduleToEdit = schedules.find(s => s.id == id);
                renderForm(scheduleToEdit);
            } else if (e.target.matches('.delete-schedule-link')) {
                const id = e.target.dataset.scheduleId;
                if (confirm('Are you sure?')) {
                    await api.delete(id);
                    loadSchedules();
                }
            } else if (e.target.matches('#add-span-btn')) {
                document.getElementById('schedule-spans-container').insertAdjacentHTML('beforeend', renderSpanRow());
            } else if (e.target.matches('.remove-span-btn')) {
                if (app.querySelectorAll('.schedule-span-row').length > 1) {
                    e.target.closest('.schedule-span-row').remove();
                } else {
                    alert('A schedule must have at least one time span.');
                }
            }
        });

        app.addEventListener('submit', async e => {
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

                await api.save(data);
                
                formContainer.style.display = 'none';
                listContainer.style.display = 'block';
                addNewBtn.style.display = 'inline-block';
                loadSchedules();
            }
        });

        loadSchedules();
    }

});
