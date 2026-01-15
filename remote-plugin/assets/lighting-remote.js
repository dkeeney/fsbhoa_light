jQuery(document).ready(function($) {
    
    // Check if the "Auto Trigger" container exists on the page
    var $triggerContainer = $('.fsbhoa-auto-trigger');

    if ($triggerContainer.length > 0) {
        var court = $triggerContainer.data('court');
        var $msg = $triggerContainer.find('.fsbhoa-status-msg');
        var $loader = $triggerContainer.find('.fsbhoa-loader');
        
        // Execute immediately
        initiateAutoTrigger(court, $msg, $loader);
    }

    function initiateAutoTrigger(court, $msg, $loader) {
        // 1. Send Request
        $.post(fsbhoa_vars.ajax_url, {
            action: 'fsbhoa_request_lights',
            nonce: fsbhoa_vars.nonce,
            court: court
        }, function(response) {
            if (response.success) {
                // 2. Request Stashed - Start Polling
                $msg.text('Controller Contacted. Waiting for confirmation...');
                pollStatus(response.data.job_id, $msg, $loader);
            } else {
                showError($msg, $loader, response.data || 'Error sending request.');
            }
        }).fail(function() {
            showError($msg, $loader, 'Network error. Please reload page.');
        });
    }

    function pollStatus(jobId, $msg, $loader) {
        var attempts = 0;
        var maxAttempts = 30; // 60 seconds max wait

        var interval = setInterval(function() {
            attempts++;
            if (attempts > maxAttempts) {
                clearInterval(interval);
                showError($msg, $loader, 'Timeout: Controller did not respond.');
                return;
            }

            $.post(fsbhoa_vars.ajax_url, {
                action: 'fsbhoa_check_status',
                job_id: jobId
            }, function(response) {
                if (response.success) {
                    var status = response.data.status;
                    
                    if (status === 'success') {
                        // SUCCESS
                        clearInterval(interval);
                        $loader.html('<span style="font-size: 50px; color: green;">&#10003;</span>'); // Big Checkmark
                        $msg.text('Success! Lights are on.').css('color', 'green');
                        
                    } else if (status.indexOf('denied') !== -1) {
                        // DENIED
                        clearInterval(interval);
                        var errorText = 'Access Denied.';
                        if (status === 'denied_no_swipe') {
                            errorText = 'Denied: No recent gate swipe found.';
                        } else if (status === 'denied_swipe_required') {
                             errorText = 'Denied: Please swipe at West Gate first.';
                        }
                        showError($msg, $loader, errorText);
                    }
                }
            });
        }, 2000); // Poll every 2 seconds
    }

    function showError($msg, $loader, text) {
        $loader.html('<span style="font-size: 50px; color: red;">&#10007;</span>'); // Big X
        $msg.text(text).css('color', 'red');
    }
});

