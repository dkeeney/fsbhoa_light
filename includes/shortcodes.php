<?php
// If this file is called directly, abort.
defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Registers all the shortcodes for the lighting control plugin.
 */
function fsbhoa_lighting_register_shortcodes() {
    add_shortcode( 'lighting_configuration', 'fsbhoa_lighting_config_page_view' );
    add_shortcode( 'lighting_schedules', 'fsbhoa_lighting_schedules_page_view' );
    add_shortcode( 'lighting_status_monitor', 'fsbhoa_lighting_monitor_page_view' );
}
add_action( 'init', 'fsbhoa_lighting_register_shortcodes' );

function fsbhoa_lighting_config_page_view() {
    ob_start();
    require_once plugin_dir_path( __FILE__ ) . 'views-configuration.php';
    return ob_get_clean();
}

function fsbhoa_lighting_schedules_page_view() {
    ob_start();
    require_once plugin_dir_path( __FILE__ ) . 'views-schedules.php';
    return ob_get_clean();
}

function fsbhoa_lighting_monitor_page_view() {
    ob_start();
    require_once plugin_dir_path( __FILE__ ) . 'views-monitor.php';
    return ob_get_clean();
}
