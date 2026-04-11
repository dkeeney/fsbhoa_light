jQuery(document).ready(function($) {
    var $container = $('.fsbhoa-auto-trigger');

    // Only run if we are on the activation screen (Stage 2)
    if ($container.length > 0) {
        var zoneId = $container.data('zone-id');
        var pollInterval;
        var timeoutCounter = 0;
        var maxWaitSeconds = 45; // Stop spinning after 45 seconds if no response

        console.log("FSBHOA: Initializing activation for Zone " + zoneId);

        // 1. SEND INITIAL REQUEST (Create Job)
        $.post(fsbhoa_vars.ajax_url, {
            action: 'fsbhoa_request_lights',
            zone_id: zoneId,
            nonce: fsbhoa_vars.nonce
        }, function(response) {
            if (response.success) {
                console.log("FSBHOA: Job ID " + response.data.job_id + " is " + response.data.status);
                
                // If the job was already successful (reused), skip to finished
                if (response.data.status === 'success') {
                    redirectToFinished('success', response.data.job_id);
                } else {
                    startPolling(response.data.job_id);
                }
            } else {
                console.error("FSBHOA: Initial request failed.");
                redirectToFinished('error', response.data.job_id);
            }
        }).fail(function() {
            redirectToFinished('server_error', response.data.job_id);
        });
    }

    // 2. STATUS POLLING ENGINE
    function startPolling(job_id) {
        $('.fsbhoa-status-msg').text("Request sent. Waiting for controller...");

        pollInterval = setInterval(function() {
            timeoutCounter += 2;

            $.post(fsbhoa_vars.ajax_url, {
                action: 'fsbhoa_check_status',
                job_id: job_id
            }, function(response) {
                if (response.success) {
                    var currentStatus = response.data.status;
                    console.log("FSBHOA: Current Job Status: " + currentStatus);

                    // If status is no longer "waiting", we move to the final UI
                    if (currentStatus !== 'pending' && currentStatus !== 'processing') {
                        stopPolling();
                        redirectToFinished(currentStatus, job_id);
                    }
                }
            });

            // Safety check: Don't let the spinner run forever
            if (timeoutCounter >= maxWaitSeconds) {
                stopPolling();
                redirectToFinished('timeout', job_id);
            }
        }, 2000); // Check every 2 seconds
    }

    function stopPolling() {
        if (pollInterval) clearInterval(pollInterval);
    }

    // 3. THE REDIRECTOR (Stage 2 -> Stage 3)
    function redirectToFinished(status, job_id) {
        // Build the URL for the Stage 3 Finished UI
        // We use window.location.pathname to strip existing query args but keep the base URL
        var finalUrl = window.location.pathname + "?finished=1&status=" + status + "&job_id=" + (job_id || 0);

        console.log("FSBHOA: Redirecting to " + finalUrl);
        window.location.href = finalUrl;
    }
});

