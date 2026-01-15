<?php
/**
 * Plugin Name: FSBHOA Remote Lighting (Bluehost)
 * Description: Handles court lighting requests via QR Code (Auto-Trigger).
 * Version: 1.1
 * Author: FSBHOA IT Committee
 * Install with a shortcode: [court_lights]
 */

defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

define( 'FSBHOA_REMOTE_URL', plugin_dir_url( __FILE__ ) );

/**
 * 1. Database Setup (Same as before)
 */
function fsbhoa_remote_lighting_install() {
    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';
    $charset_collate = $wpdb->get_charset_collate();

    $sql = "CREATE TABLE $table_name (
        id mediumint(9) NOT NULL AUTO_INCREMENT,
        court_name varchar(50) NOT NULL,
        user_email varchar(100) NOT NULL,
        status varchar(20) DEFAULT 'pending',
        created_at datetime DEFAULT CURRENT_TIMESTAMP,
        updated_at datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
        PRIMARY KEY  (id)
    ) $charset_collate;";

    require_once( ABSPATH . 'wp-admin/includes/upgrade.php' );
    dbDelta( $sql );
}
register_activation_hook( __FILE__, 'fsbhoa_remote_lighting_install' );

/**
 * 2. Auto-Trigger Shortcode
 * Usage on Page: [court_lights]
 * The URL determines the court: fsbhoa.com/lights/?court=pickleball
 */
function fsbhoa_render_auto_trigger( $atts ) {
    // 1. Force Login first
    if ( ! is_user_logged_in() ) {
        // Redirects to login and brings them back to this URL afterwards
        auth_redirect(); 
    }

    // 2. Determine Court (URL param overrides shortcode attribute)
    $a = shortcode_atts( array( 'court' => '' ), $atts );
    
    // Check $_GET['court'] first (e.g., ?court=tennis)
    $court = isset($_GET['court']) ? sanitize_key($_GET['court']) : sanitize_key($a['court']);

    if ( empty($court) ) {
        return '<p style="color:red;">Error: No court specified in URL.</p>';
    }

    // 3. Load Javascript
    wp_enqueue_script( 
        'fsbhoa-remote-js', 
        FSBHOA_REMOTE_URL . 'assets/lighting-remote.js', 
        array('jquery'), 
        '1.1', 
        true 
    );

    wp_localize_script( 'fsbhoa-remote-js', 'fsbhoa_vars', array(
        'ajax_url' => admin_url( 'admin-ajax.php' ),
        'nonce'    => wp_create_nonce( 'fsbhoa_light_req_nonce' )
    ));

    // 4. Render "Connecting" UI (No Button)
    ob_start();
    ?>
    <div class="fsbhoa-auto-trigger" data-court="<?php echo esc_attr($court); ?>" style="text-align: center; padding: 20px; border: 1px solid #ddd; max-width: 400px; margin: 0 auto;">
        <h3>Accessing <?php echo ucfirst($court); ?> Court...</h3>
        <div class="fsbhoa-loader" style="margin: 20px 0;">
            <div style="border: 4px solid #f3f3f3; border-top: 4px solid #3498db; border-radius: 50%; width: 30px; height: 30px; animation: spin 1s linear infinite; margin: 0 auto;"></div>
        </div>
        <div class="fsbhoa-status-msg" style="font-weight: bold; color: #555;">
            Connecting to Lighting Controller...
        </div>
        <style>@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }</style>
    </div>
    <?php
    return ob_get_clean();
}
add_shortcode( 'court_lights', 'fsbhoa_render_auto_trigger' );

/**
 * 3. Request Handler (Same as before)
 */
function fsbhoa_handle_light_request() {
    check_ajax_referer( 'fsbhoa_light_req_nonce', 'nonce' );

    if ( ! is_user_logged_in() ) {
        wp_send_json_error( 'You must be logged in.' );
    }

    $court = isset($_POST['court']) ? sanitize_key($_POST['court']) : '';
    $current_user = wp_get_current_user();
    $email = $current_user->user_email;

    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';

    $result = $wpdb->insert(
        $table_name,
        array( 'court_name' => $court, 'user_email' => $email, 'status' => 'pending' )
    );

    if ( $result ) {
        wp_send_json_success( array( 'job_id' => $wpdb->insert_id ) );
    } else {
        wp_send_json_error( 'Database error.' );
    }
}
add_action( 'wp_ajax_fsbhoa_request_lights', 'fsbhoa_handle_light_request' );

/**
 * 4. Status Check (Same as before)
 */
function fsbhoa_check_job_status() {
    $job_id = isset($_POST['job_id']) ? intval($_POST['job_id']) : 0;
    global $wpdb;
    $table_name = $wpdb->prefix . 'lighting_queue';
    
    $status = $wpdb->get_var( $wpdb->prepare( "SELECT status FROM $table_name WHERE id = %d", $job_id ) );

    if ( $status ) {
        wp_send_json_success( array( 'status' => $status ) );
    } else {
        wp_send_json_error( 'Job not found' );
    }
}
add_action( 'wp_ajax_fsbhoa_check_status', 'fsbhoa_check_job_status' );

