<?php
/**
 * API Endpoint: Update Lighting Job Status
 */

// 1. Load WordPress
$path = preg_replace( '/wp-content.*$/', '', __DIR__ );

if ( file_exists( $path . 'wp-load.php' ) ) {
    require_once( $path . 'wp-load.php' );
} elseif ( isset($_SERVER['DOCUMENT_ROOT']) && file_exists( $_SERVER['DOCUMENT_ROOT'] . '/wp-load.php' ) ) {
    require_once( $_SERVER['DOCUMENT_ROOT'] . '/wp-load.php' );
} else {
    // If all else fails, look up 4 levels (Standard plugin structure)
    require_once( dirname(__DIR__, 4) . '/wp-load.php' );
}

// Release the session lock so other pages can load
if ( session_id() ) {
    session_write_close();
}


// 2. Authentication (Same as wait_for_job.php)
$valid_key = defined('FSBHOA_LIGHTING_API_KEY') ? FSBHOA_LIGHTING_API_KEY : '733KjVR4jkBGnoBEDLbC1rvCqxF7gMdz6ygbxRjCM+Y=';
$provided_key = $_SERVER['HTTP_X_API_KEY'] ?? '';

if ( $provided_key !== $valid_key ) {
    http_response_code(403);
    die(json_encode(["error" => "Unauthorized"]));
}

// 3. Update the Job
global $wpdb;
$table_name = $wpdb->prefix . 'lighting_queue';

$job_id = isset($_GET['job_id']) ? intval($_GET['job_id']) : 0;
$status = isset($_GET['status']) ? sanitize_key($_GET['status']) : '';

if ($job_id > 0 && !empty($status)) {
    $updated = $wpdb->update(
        $table_name,
        array('status' => $status),
        array('id' => $job_id)
    );

    echo json_encode(["success" => $updated !== false]);
} else {
    echo json_encode(["error" => "Invalid parameters"]);
}
