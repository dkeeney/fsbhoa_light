jQuery(document).ready(function($) {
    'use strict';
    console.log('admin-settings.js loaded'); // Debug message
    
    let mediaFrame;

    // --- NEW: Media Uploader Button Handler ---
    $('#fsbhoa-upload-map_image_url-button').on('click', function(e) {
        e.preventDefault();
        
        const $button = $(this);
        const $input = $('#map_image_url');
        const $previewWrapper = $('#map_image_url-preview-wrapper');

        // If the frame already exists, open it
        if (mediaFrame) {
            mediaFrame.open();
            return;
        }

        // Create a new media frame
        mediaFrame = wp.media({
            title: 'Select or Upload Map Image',
            button: {
                text: 'Use this image'
            },
            multiple: false
        });

        // When an image is selected, run a callback
        mediaFrame.on('select', function() {
            // Get the attachment details
            const attachment = mediaFrame.state().get('selection').first().toJSON();
            
            // Update the input field with the URL
            $input.val(attachment.url);
            
            // Update the preview image
            $previewWrapper.html('<img src="' + attachment.url + '" style="max-width: 400px; height: auto; border: 1px solid #ddd;">');
        });

        // Finally, open the frame
        mediaFrame.open();
    });


    // --- Save Settings Button Handler ---
    $('#fsbhoa-save-lighting-settings-button').on('click', function() {
        const saveButton = $(this);
        const feedbackSpan = $('#fsbhoa-save-feedback');

        feedbackSpan.text('Saving...').css('color', 'blue').show();
        saveButton.prop('disabled', true);

        // Collect all input values from the page
        const options = [];
        $('#fsbhoa-lighting-settings-page .form-table input, #fsbhoa-lighting-settings-page .form-table select').each(function() {
            const input = $(this);
            const nameAttr = input.attr('name');
            // Extract the simple option name (e.g., 'go_service_port')
            const nameMatch = nameAttr ? nameAttr.match(/fsbhoa_lighting_settings\[(.*?)\]/) : null;
            if (nameMatch && nameMatch[1]) {
                 options.push({
                     name: nameMatch[1],
                     value: input.is(':checkbox') ? (input.is(':checked') ? 'on' : 'off') : input.val()
                 });
            }
        });

        const dataToSend = {
            action: 'fsbhoa_save_lighting_settings',
            nonce: fsbhoa_lighting_admin_vars.save_nonce,
            options: options
        };

        $.post(fsbhoa_lighting_admin_vars.ajax_url, dataToSend)
            .done(function(response) {
                if (response.success) {
                    feedbackSpan.text('Success! Settings saved. Config file updated.').css('color', 'green');
                      setTimeout(function() { feedbackSpan.fadeOut(); }, 3000);
                } else {
                    feedbackSpan.text('Error: ' + (response.data || 'Unknown error')).css('color', 'red');
                }
            })
            .fail(function() {
                feedbackSpan.text('Request failed. Check network or server logs.').css('color', 'red');
            })
            .always(function() {
                 saveButton.prop('disabled', false);
            });
    });

    // --- Generate API Key Button Handler ---
    // Ensure ID matches the one rendered in PHP
    $('#fsbhoa-generate-go_service_api_key-button').on('click', function(e) {
        e.preventDefault();
        var $button = $(this);
        // Ensure input ID matches the one rendered in PHP
        var $input = $('#go_service_api_key');

        if (!$input.length) {
            console.error('API Key input field #go_service_api_key not found!');
            alert('Error: API Key input field not found on the page.');
            return;
        }

        if (!confirm('Generate a new API key? The Go service will need to be restarted with the new key.')) return;

        $button.prop('disabled', true).text('Generating...');

        $.post(fsbhoa_lighting_admin_vars.ajax_url, {
            action: 'fsbhoa_generate_lighting_api_key', // Correct AJAX action
            nonce: fsbhoa_lighting_admin_vars.save_nonce // Reuse save nonce
        })
        .done(function(response) {
            console.log('Generate API Key AJAX response:', response);
            if (response.success && response.data && response.data.api_key) {
                $input.val(response.data.api_key);
                alert('New API Key generated. Click "Save Settings" to store it.');
            } else {
                alert('Error generating key: ' + (response.data || 'Unknown error from server'));
            }
        })
        .fail(function(jqXHR, textStatus, errorThrown) {
             console.error('Generate API Key AJAX failed:', textStatus, errorThrown);
             alert('Failed to generate key. Check console.');
         })
        .always(function() { $button.prop('disabled', false).text('Generate New Key'); });
    });

    // --- NEW: Service Management Section ---

    const serviceName = 'fsbhoa-lighting.service'; // Hardcoded for this page
    const statusSpan = $('#status-' + serviceName.replace(/\./g, '\\.')); // Escape dots for jQuery selector
    const cmdFeedbackSpan = $('#fsbhoa-service-cmd-feedback');

    // Function to update the status indicator UI
    function updateStatusUI(statusData) {
        statusSpan.removeClass('is-running is-stopped'); // Clear previous classes
        console.log(`Updating UI for '${serviceName}' with status: '${statusData.status}'`); // Debug log

        if (statusData.status === 'running') {
            statusSpan.text('Running').addClass('is-running');
            // Disable start, enable stop/restart
            $('.service-command-btn[data-command="start"]').prop('disabled', true);
            $('.service-command-btn[data-command="stop"], .service-command-btn[data-command="restart"]').prop('disabled', false);
        } else { // Includes stopped, failed, inactive etc.
            statusSpan.text('Stopped').addClass('is-stopped');
            // Enable start, disable stop/restart
            $('.service-command-btn[data-command="start"]').prop('disabled', false);
            $('.service-command-btn[data-command="stop"], .service-command-btn[data-command="restart"]').prop('disabled', true);
        }
    }

    // Function to check the service status via AJAX
    function checkServiceStatus() {
        if (!statusSpan.length) {
            console.error("Status span not found!"); // Debug: Check if element exists
            return;
        }
        statusSpan.text('Checking...').removeClass('is-running is-stopped');
        cmdFeedbackSpan.hide(); // Hide previous command feedback

        $.post(fsbhoa_lighting_admin_vars.ajax_url, {
            action: 'fsbhoa_manage_lighting_service', // Ensure this matches the PHP action hook
            nonce: fsbhoa_lighting_admin_vars.manage_nonce,
            command: 'status'
        })
        .done(function(response) {
            if (response.success && response.data) {
                updateStatusUI(response.data);
            } else {
                statusSpan.text('Error').removeClass('is-running is-stopped');
                cmdFeedbackSpan.text('Error checking status: ' + (response.data || 'Unknown error')).css('color', 'red').show();
                console.error("Status check failed:", response); // Log error details
            }
        })
        .fail(function(jqXHR, textStatus, errorThrown) {
            statusSpan.text('Error').removeClass('is-running is-stopped');
            cmdFeedbackSpan.text('Failed to check status (AJAX error).').css('color', 'red').show();
            console.error("Status check AJAX failed:", textStatus, errorThrown); // Log AJAX error
        });
    }

    // Click handler for Start, Stop, Restart buttons
    $('.service-command-btn').on('click', function(e) {
        e.preventDefault();
        const button = $(this);
        const command = button.data('command');

        cmdFeedbackSpan.text(`Sending ${command} command...`).css('color', 'blue').show();
        $('.service-command-btn').prop('disabled', true); // Disable all buttons during action

        $.post(fsbhoa_lighting_admin_vars.ajax_url, {
            action: 'fsbhoa_manage_lighting_service', // Ensure this matches the PHP action hook
            nonce: fsbhoa_lighting_admin_vars.manage_nonce,
            command: command
        })
        .done(function(response) {
            if (response.success) {
                cmdFeedbackSpan.text(`Command '${command}' sent. Checking status...`).css('color', 'green');
                console.log(`Command '${command}' response output:`, response.data.output); // Log output for debugging
                // Wait a moment for service to change state, then update status
                setTimeout(checkServiceStatus, (command === 'restart' ? 3000 : 2000)); // Longer delay for restart
            } else {
                cmdFeedbackSpan.text(`Error sending ${command}: ` + (response.data || 'Unknown error')).css('color', 'red');
                console.error(`Command '${command}' failed:`, response.data); // Log error details
                 // Re-enable buttons based on last known status on failure (or just run check again)
                 checkServiceStatus();
            }
        })
        .fail(function(jqXHR, textStatus, errorThrown) {
             cmdFeedbackSpan.text(`Failed to send ${command} command (AJAX error).`).css('color', 'red');
             console.error(`Command '${command}' AJAX failed:`, textStatus, errorThrown); // Log AJAX error
             checkServiceStatus(); // Re-enable buttons based on last known status
         });
    });

    // Initial status check on page load, only if the status span exists
    if (statusSpan.length) {
       checkServiceStatus();
    } else {
        console.error("Initial status check skipped: Status span not found."); // Debug: Check if element exists on load
    }

}); // End document ready
