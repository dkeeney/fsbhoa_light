<?php
/**
 * API Endpoint: Wait for Lighting Job
 * * This script performs "Long Polling". It holds the connection open for up to 45 seconds
 * checking the database for new 'pending' jobs.
 * * Authentication: Requires 'X-API-Key' header.
 */

// 1. Load WordPress Environment
// We need to find wp-load.php. Since this file is deep in a plugin, we look up.
$path = preg_replace( '/wp-content.*$/', '', __DIR__ );
if ( file_exists( $path . 'wp-load.php' ) ) {
    require_once( $path . 'wp-load.php' );
} else {
    // Fallback for standard setups if regex fails
    require_once( $_SERVER['DOCUMENT_ROOT'] . '/wp-load.php' );
}

// 2. Authentication
$headers = getallheaders();
// Note: You must define FSBHOA_LIGHTING_API_KEY in your Bluehost wp-config.php 
// OR hardcode the key here for simplicity.
$valid_key = defined('FSBHOA_LIGHTING_API_KEY') ? FSBHOA_LIGHTING_API_KEY : '733KjVR4jkBGnoBEDLbC1rvCqxF7gMdz6ygbxRjCM+Y=';

// HEADER CHECK: Look in $_SERVER first (Standard), then fallback to headers
$provided_key = '';
if (isset($_SERVER['HTTP_X_API_KEY'])) {
    $provided_key = $_SERVER['HTTP_X_API_KEY'];
} elseif (function_exists('getallheaders')) {
    $headers = getallheaders();
    if (isset($headers['X-API-Key'])) {
        $provided_key = $headers['X-API-Key'];
    }
}

if ( $provided_key !== $valid_key ) {
    http_response_code(403);
    // Debugging hint (visible if you curl manually)
    die(json_encode(["error" => "Unauthorized", "received" => $provided_key]));
}

// 3. Setup Headers & Time Limits
header('Cache-Control: no-cache');
header('Content-Type: application/json');

// Allow script to run longer than the standard 30s
if (function_exists('set_time_limit')) {
    set_time_limit(60); 
}

global $wpdb;
$table_name = $wpdb->prefix . 'lighting_queue';

$start_time = time();
$max_wait = 45; // Wait window (seconds)

// 4. The Polling Loop
while ((time() - $start_time) < $max_wait) {

    // A. Start Transaction (Atomic Lock)
    $wpdb->query("START TRANSACTION");

    // B. Find & Lock the Oldest Pending Job
    // 'FOR UPDATE' prevents race conditions if you have multiple Go services (unlikely, but safe)
    $job = $wpdb->get_row("
        SELECT * FROM $table_name 
        WHERE status = 'pending' 
        ORDER BY created_at ASC 
        LIMIT 1 
        FOR UPDATE
    ");

    if ($job) {
        // C. Claim the Job (Mark as processing)
        $wpdb->update(
            $table_name,
            array('status' => 'processing'),
            array('id' => $job->id)
        );

        // D. Commit Transaction
        $wpdb->query("COMMIT");

        // E. Return JSON to Go Service
        echo json_encode([
            "status" => "found",
            "job_id" => (int)$job->id,
            "court"  => $job->court_name,
            "email"  => $job->user_email
        ]);
        exit; // Terminate immediately
    } else {
        // No job found, Rollback lock
        $wpdb->query("ROLLBACK");
    }

    // F. Sleep & Retry
    // Sleep briefly to reduce CPU usage
    sleep(2);
    
    // Clear PHP internal cache to ensure next DB read is fresh
    clearstatcache();
}

// 5. Timeout Reached
// Return "timeout" status so Go knows to reconnect
echo json_encode(["status" => "timeout"]);
exit;

