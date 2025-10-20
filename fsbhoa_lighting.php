<?php
/**
 * Plugin Name:       FSBHOA Lighting Control
 * Description:       A custom plugin to manage and schedule PLC-based lighting control panels.
 * Version:           1.0.0
 * Author:            Your Name
 * License:           GPL-2.0-or-later
 * License URI:       https://www.gnu.org/licenses/gpl-2.0.html
 */

// If this file is called directly, abort.
defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Include the necessary files.
 */
require_once plugin_dir_path( __FILE__ ) . 'includes/shortcodes.php';
require_once plugin_dir_path( __FILE__ ) . 'includes/actions-configuration.php';
require_once plugin_dir_path( __FILE__ ) . 'includes/actions-schedules.php';
require_once plugin_dir_path( __FILE__ ) . 'includes/actions-monitor.php';

/**
 * Create/update the custom database tables on plugin activation.
 */
function fsbhoa_lighting_activate() {
    global $wpdb;
    $charset_collate = $wpdb->get_charset_collate();
    require_once( ABSPATH . 'wp-admin/includes/upgrade.php' );

    // 1. Zones Table
    $table_name_zones = $wpdb->prefix . 'fsbhoa_lighting_zones';
    $sql_zones = "CREATE TABLE $table_name_zones ( id mediumint(9) NOT NULL AUTO_INCREMENT, zone_name varchar(100) NOT NULL, description text, PRIMARY KEY  (id) ) $charset_collate;";
    dbDelta( $sql_zones );

    // 2. PLC Outputs Table (Hardware Map)
    $table_name_outputs = $wpdb->prefix . 'fsbhoa_lighting_plc_outputs';
    $sql_outputs = "CREATE TABLE $table_name_outputs ( id mediumint(9) NOT NULL AUTO_INCREMENT, plc_id tinyint(4) NOT NULL, description varchar(255), plc_outputs json NOT NULL, relays json NOT NULL, PRIMARY KEY  (id) ) $charset_collate;";
    dbDelta( $sql_outputs );

    // 3. Zone to Output Mapping Table
    $table_name_zone_map = $wpdb->prefix . 'fsbhoa_lighting_zone_output_map';
    $sql_zone_map = "CREATE TABLE $table_name_zone_map ( zone_id mediumint(9) NOT NULL, output_id mediumint(9) NOT NULL, PRIMARY KEY  (zone_id, output_id) ) $charset_collate;";
    dbDelta( $sql_zone_map );

    // 4. Schedules Table
    $table_name_schedules = $wpdb->prefix . 'fsbhoa_lighting_schedules';
    $sql_schedules = "CREATE TABLE $table_name_schedules ( id mediumint(9) NOT NULL AUTO_INCREMENT, schedule_name varchar(100) NOT NULL, PRIMARY KEY  (id) ) $charset_collate;";
    dbDelta( $sql_schedules );

    // 4a. Schedule Spans Table
    $table_name_spans = $wpdb->prefix . 'fsbhoa_lighting_schedule_spans';
    $sql_spans = "CREATE TABLE $table_name_spans ( id mediumint(9) NOT NULL AUTO_INCREMENT, schedule_id mediumint(9) NOT NULL, days_of_week JSON NOT NULL, on_trigger varchar(20) NOT NULL, on_time time, off_trigger varchar(20) NOT NULL, off_time time, PRIMARY KEY  (id), KEY schedule_id (schedule_id) ) $charset_collate;";
    dbDelta( $sql_spans );
    
    // 5. Zone to Schedule Mapping Table
    $table_name_schedule_map = $wpdb->prefix . 'fsbhoa_lighting_zone_schedule_map';
    $sql_schedule_map = "CREATE TABLE $table_name_schedule_map ( zone_id mediumint(9) NOT NULL, schedule_id mediumint(9) NOT NULL, PRIMARY KEY  (zone_id, schedule_id) ) $charset_collate;";
    dbDelta( $sql_schedule_map );
}
register_activation_hook( __FILE__, 'fsbhoa_lighting_activate' );

/**
 * Enqueue the JavaScript files for our applications.
 */
function fsbhoa_lighting_enqueue_scripts() {
    // For the configuration page
    if ( is_a( get_post(), 'WP_Post' ) && has_shortcode( get_post()->post_content, 'lighting_configuration' ) ) {
        wp_enqueue_script( 'fsbhoa-zone-manager', plugin_dir_url( __FILE__ ) . 'assets/js/zone-manager.js', array(), '1.0.0', true );
        wp_localize_script( 'fsbhoa-zone-manager', 'fsbhoa_lighting_data', array( 'rest_url' => rest_url(), 'nonce' => wp_create_nonce( 'wp_rest' ) ) );
    }

    // For the schedules page
    if ( is_a( get_post(), 'WP_Post' ) && has_shortcode( get_post()->post_content, 'lighting_schedules' ) ) {
        wp_enqueue_script( 'fsbhoa-schedules-manager', plugin_dir_url( __FILE__ ) . 'assets/js/schedules-manager.js', array(), '1.0.0', true );
        wp_localize_script( 'fsbhoa-schedules-manager', 'fsbhoa_lighting_data', array( 'rest_url' => rest_url(), 'nonce' => wp_create_nonce( 'wp_rest' ) ) );
    }
}
add_action( 'wp_enqueue_scripts', 'fsbhoa_lighting_enqueue_scripts' );

/**
 * Protects the entire front-end of the site.
 */
function fsbhoa_require_admin_globally() {
    if ( is_admin() ) { return; }
    if ( ! current_user_can( 'manage_options' ) ) {
        wp_redirect( wp_login_url() );
        exit;
    }
}
add_action( 'template_redirect', 'fsbhoa_require_admin_globally' );

/**
 * Adds a phpMyAdmin link to the WordPress Admin Bar.
 */
function fsbhoa_lighting_add_admin_bar_link( $wp_admin_bar ) {
    if ( ! current_user_can( 'manage_options' ) ) { return; }
    $args = array( 'id' => 'phpmyadmin-link', 'title' => 'phpMyAdmin', 'href'  => site_url( '/phpmyadmin' ), 'meta'  => array( 'target' => '_blank' ) );
    $wp_admin_bar->add_node( $args );
}
add_action( 'admin_bar_menu', 'fsbhoa_lighting_add_admin_bar_link', 999 );
