<?php
/**
 * Plugin Name: FSBHOA Remote Lighting (QR scan)
 * Description: Handles court lighting requests via QR Code (Auto-Trigger).
 * Version: 1.3
 * Author: FSBHOA IT Committee
 * Install with a shortcode: [court_lights]
 */

defined( 'ABSPATH' ) or die( 'Unauthorized Access' );
define( 'FSBHOA_REMOTE_URL', plugin_dir_url( __FILE__ ) );

/**
 * 1. Database Setup & Assets
 */
register_activation_hook( __FILE__, function() {
    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';
    $charset_collate = $wpdb->get_charset_collate();
    $sql = "CREATE TABLE $table_name (
        id mediumint(9) NOT NULL AUTO_INCREMENT,
        zone_id mediumint(9) NOT NULL,
        user_email varchar(100) NOT NULL,
        status varchar(20) DEFAULT 'pending',
        created_at datetime DEFAULT CURRENT_TIMESTAMP,
        updated_at datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
        PRIMARY KEY  (id)
        KEY idx_status (status)
    ) $charset_collate;";
    require_once( ABSPATH . 'wp-admin/includes/upgrade.php' );
    dbDelta( $sql );
});

add_action( 'wp_enqueue_scripts', function() {
    wp_register_script('fsbhoa-remote-js', FSBHOA_REMOTE_URL . 'assets/lighting-remote.js', array('jquery'), '1.5', true);
    wp_localize_script('fsbhoa-remote-js', 'fsbhoa_vars', array(
        'ajax_url' => admin_url( 'admin-ajax.php' ),
        'nonce'    => wp_create_nonce( 'fsbhoa_light_req_nonce' )
    ));
});

/**
 * 2. STAGE 1: THE REDIRECTOR (Prevents White Screen)
 * This runs before the page content is generated.
 */
add_action( 'template_redirect', function() {
    // Only run on our lights page. Update 'lights' if your slug is different.
    if ( ! is_page('lights') ) return;

    $zone_id = isset($_GET['zone_id']) ? intval($_GET['zone_id']) : 0;
    $step    = isset($_GET['step']) ? intval($_GET['step']) : 1;

    // Stage 1 logic: Set cookie and redirect to Step 2
    if ( $step === 1 && $zone_id > 0 ) {
        if ( !empty($_COOKIE) ) {
            foreach ( $_COOKIE as $name => $value ) {
                if ( strpos($name, 'fsbhoa_token_') === 0 ) {
                    setcookie($name, '', time() - 3600, "/");
                }
            }
        }

        $scan_id = time();
        setcookie("fsbhoa_token_" . $scan_id, 'unused', time() + 1800, "/");

        $redirect_url = add_query_arg( array(
            'zone_id' => $zone_id,
            'scan_id' => $scan_id,
            'step'    => 2
        ), get_permalink() );

        wp_redirect( $redirect_url );
        exit;
    }
});

/**
 * 3. THE SHORTCODE ROUTER [court_lights]
 */
function fsbhoa_render_auto_trigger( $atts ) {
    $zone_id = isset($_GET['zone_id']) ? intval($_GET['zone_id']) : 0;

    // --- STAGE 3: RESULT SCREEN ---
    if ( isset($_GET['finished']) ) {
        $status  = isset($_GET['status']) ? sanitize_key($_GET['status']) : 'error';
        return fsbhoa_render_finished_ui($status);
    }

    // --- ERROR CHECK: No Zone ---
    if ( $zone_id === 0 ) {
        return '<p style="color:red; text-align:center;">Error: No valid zone specified.</p>';
    }

    // --- STAGE 2: THE GATE & SPINNER ---
    $scan_id = isset($_GET['scan_id']) ? intval($_GET['scan_id']) : 0;
    $cookie_name = "fsbhoa_token_" . $scan_id;

    // A. Login Check
    if ( ! is_user_logged_in() ) {
        return fsbhoa_render_login_required_ui();
    }

    // B. Back-Button / Security Trap
    if ( $scan_id === 0 || !isset($_COOKIE[$cookie_name]) || $_COOKIE[$cookie_name] === 'spent' ) {
        return fsbhoa_render_expired_ui();
    }

    // C. Burn Ticket & Show Spinner
    setcookie($cookie_name, 'spent', time() + 1800, "/");
    wp_enqueue_script('fsbhoa-remote-js');
    return fsbhoa_render_spinner_ui($zone_id);
}
add_shortcode( 'court_lights', 'fsbhoa_render_auto_trigger' );

/**
 * 4. UI SPECIALISTS
 */
function fsbhoa_render_spinner_ui($zone_id) {
    ob_start(); ?>
    <div class="fsbhoa-auto-trigger" data-zone-id="<?php echo $zone_id; ?>" style="text-align: center; padding: 20px;">
        <h3>Activating Zone #<?php echo $zone_id; ?></h3>
        <div style="border: 4px solid #f3f3f3; border-top: 4px solid #3498db; border-radius: 50%; width: 30px; height: 30px; animation: spin 1s linear infinite; margin: 20px auto;"></div>
        <div class="fsbhoa-status-msg">Connecting to Lighting Controller...</div>
        <style>@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }</style>
    </div>
    <?php return ob_get_clean();
}

function fsbhoa_render_finished_ui($status) {
    ob_start(); ?>
    <div style="text-align:center; padding:30px;">
        <?php if ( $status === 'success' ): ?>
            <div style="font-size: 60px; color: green;">&#10003;</div>
            <h2>Lights are ON!</h2>
            <p>Timer is started. Lights are on when dark and within 5am to 10pm.</p>

        <?php elseif ( $status === 'outside_qr_window' ): ?>
            <div style="font-size: 60px;">☀️</div>
            <h2>Too Early for Lights</h2>
            <p>QR activation is only available during scheduled night hours within 5am to 10pm.</p>

        <?php elseif ( $status === 'qr_not_available' || $status === 'schedule_not_found' ): ?>
            <div style="font-size: 60px;">❌</div>
            <h2>QR Not Enabled</h2>
            <p>This zone is not currently configured for QR code activation. Please contact the IT Committee if you believe this is an error.</p>

        <?php else: ?>
            <div style="font-size: 60px; color: red;">&#10007;</div>
            <h2>Access Denied</h2>
            <p>We could not activate the lights. (Status: <?php echo esc_html($status); ?>)</p>
        <?php endif; ?>

        <p style="margin-top:20px; font-size: 0.8em; color: #999;">Session Ended. You may close the tab.</p>
    </div>
    <?php return ob_get_clean();
}


function fsbhoa_render_login_required_ui() {
    return '
                <div style="text-align:center; padding:40px;">
                    <div style="font-size: 50px;">??</div>
                    <h2>Login Required</h2>
                    <p>You must be logged into the FSBHOA website to control the lights.</p>
                    <a href="'.home_url('/log-in/').'" class="button" style="display:inline-block; background:#3498db; color:#fff; padding:15px 30px; border-radius:5px; text-decoration:none;">
                        Log In Here
                    </a>
                    <p style="margin-top:20px; font-size:0.9em; color:#666;">
                        After logging in, please return to the court and <strong>re-scan the QR code</strong>.
                    </p>
                </div>';
}

function fsbhoa_render_expired_ui() {
    return '<div style="text-align:center; padding:40px;"><h2>⏳ Session Expired</h2><p>Please re-scan the QR code.</p></div>';
}

/**
 * 5. AJAX HANDLERS (Preserved from your code)
 */
add_action( 'wp_ajax_fsbhoa_request_lights', 'fsbhoa_handle_light_request' );
function fsbhoa_handle_light_request() {
    check_ajax_referer( 'fsbhoa_light_req_nonce', 'nonce' );
    $zone_id = isset($_POST['zone_id']) ? intval($_POST['zone_id']) : 0;
    $current_user = wp_get_current_user();
    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';

    $existing_job = $wpdb->get_row( $wpdb->prepare( "
        SELECT id, status FROM $table_name
        WHERE zone_id = %d AND user_email = %s
        AND (status IN ('pending', 'processing') OR (status = 'success' AND updated_at > DATE_SUB(NOW(), INTERVAL 2 MINUTE)))
        ORDER BY created_at DESC LIMIT 1
    ", $zone_id, $current_user->user_email ) );

    if ( $existing_job ) {
        wp_send_json_success( array('job_id' => $existing_job->id, 'status' => $existing_job->status) );
    }

    $result = $wpdb->insert($table_name, array('zone_id' => $zone_id, 'user_email' => $current_user->user_email, 'status' => 'pending'));
    if ( $result ) {
        wp_send_json_success( array( 'job_id' => $wpdb->insert_id ) );
    } else {
        wp_send_json_error( 'Database error.' );
    }
}

add_action( 'wp_ajax_fsbhoa_check_status', function() {
    $job_id = isset($_POST['job_id']) ? intval($_POST['job_id']) : 0;
    global $wpdb;
    $status = $wpdb->get_var( $wpdb->prepare( "SELECT status FROM " . $wpdb->prefix . "lighting_queue WHERE id = %d", $job_id ) );
    wp_send_json_success( array( 'status' => $status ) );
});


/**
 * 6. REST API ENDPOINTS    (Accessed by fsbhoa-lighting service)
 */
/**
 * CORRECTED REST API REGISTRATION
 */
add_action( 'rest_api_init', function () {

    // The namespace for our routes
    $namespace = 'lights/v1';

    // 1. Wait for Job Route
    register_rest_route( $namespace, '/wait-for-job', array(
        'methods'             => 'GET',
        'callback'            => 'fsbhoa_handle_rest_wait_for_job',
        'permission_callback' => '__return_true',
    ));

    // 2. Update Job Route
    register_rest_route( $namespace, '/update-job', array(
        'methods'             => 'POST',
        'callback'            => 'fsbhoa_handle_rest_update_job',
        'permission_callback' => '__return_true',
    ));

    // 3. Debug Route
    register_rest_route( $namespace, '/debug-db', array(
        'methods'  => 'GET',
        'callback' => function() {
            global $wpdb;
            $table = $wpdb->prefix . 'lighting_queue';
            $results = $wpdb->get_results("SELECT * FROM $table ORDER BY id DESC LIMIT 10");
            return rest_ensure_response($results);
        },
        'permission_callback' => '__return_true',
    ));
});

// Implementation of wait_for_job.php
function fsbhoa_handle_rest_wait_for_job( $request ) {
    if ( session_id() ) {
        session_write_close();
    }

    $valid_key = defined('FSBHOA_LIGHTING_API_KEY') ? FSBHOA_LIGHTING_API_KEY : '733KjVR4jkBGnoBEDLbC1rvCqxF7gMdz6ygbxRjCM+Y=';
    $provided_key = $request->get_header('X-API-Key');

    if ( $provided_key !== $valid_key ) {
        return new WP_Error( 'unauthorized', 'Invalid API Key', array( 'status' => 403 ) );
    }

    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';
    $start_time = time();
    $max_wait = 15;

    // database cleanup
    $wpdb->query( "DELETE FROM $table_name WHERE updated_at < DATE_SUB(NOW(), INTERVAL 1 HOUR)" );

    while ((time() - $start_time) < $max_wait) {
        $wpdb->query("START TRANSACTION");
        $job = $wpdb->get_row("SELECT * FROM $table_name WHERE status = 'pending' ORDER BY created_at ASC LIMIT 1 FOR UPDATE");

        try {
            if ($job) {
                $wpdb->update($table_name, array('status' => 'processing'), array('id' => $job->id));
                if ($updated === false) {
                    // DATABASE ERROR: Rollback immediately!
                    $wpdb->query("ROLLBACK");
                    error_log("FSBHOA LIGHTING ERROR: Failed to update job ID " . $job->id);
                    return new WP_Error( 'db_error', 'Database update failed', array( 'status' => 500 ) );
                }
                $wpdb->query("COMMIT");
                return new WP_REST_Response(array(
                    "status" => "found",
                    "job_id" => (int)$job->id,
                    "zone_id"  => (int)$job->zone_id,
                    "email"  => $job->user_email
                ), 200);
            }
            $wpdb->query("ROLLBACK");

       } catch (Exception $e) {
            // EMERGENCY CLEANUP
            $wpdb->query("ROLLBACK");
            error_log("FSBHOA LIGHTING EXCEPTION: " . $e->getMessage());
            // Wait a bit before retrying to avoid hammering a sick DB
            sleep(1); 
       }
       // wait before next poll of database.
       sleep(2);

    }
    return new WP_REST_Response(array("status" => "timeout"), 200);
}

// Implementation of update_job.php
function fsbhoa_handle_rest_update_job( $request ) {
    if ( session_id() ) {
        session_write_close();
    }

    $valid_key = defined('FSBHOA_LIGHTING_API_KEY') ? FSBHOA_LIGHTING_API_KEY : '733KjVR4jkBGnoBEDLbC1rvCqxF7gMdz6ygbxRjCM+Y=';
    $provided_key = $request->get_header('X-API-Key');

    if ( $provided_key !== $valid_key ) {
        return new WP_Error( 'unauthorized', 'Invalid API Key', array( 'status' => 403 ) );
    }

    $job_id = $request->get_param('job_id');
    $status = sanitize_key($request->get_param('status'));

    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';
    $updated = $wpdb->update($table_name, array('status' => $status), array('id' => $job_id));

    return new WP_REST_Response(array("success" => $updated !== false), 200);
}


