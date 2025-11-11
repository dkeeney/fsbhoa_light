<?php
// If this file is called directly, abort.
defined( 'ABSPATH' ) or die( 'Unauthorized Access' );

/**
 * Handles the admin settings page for the lighting control plugin.
 */
class Fsbhoa_Lighting_Admin_Settings {

    private $option_group = 'fsbhoa_lighting_options';
    private $option_name = 'fsbhoa_lighting_settings';
    private $page_slug = 'fsbhoa-lighting-settings';
    private $config_file_path = '/var/lib/fsbhoa/lighting_service.json';
    private $default_log_path = '/home/fsbhoa/fsbhoa_light/lighting-service/lighting-service.log';

    public function __construct() {
        add_action( 'admin_menu', array( $this, 'add_settings_page' ) );
        add_action( 'admin_init', array( $this, 'register_settings' ) );
        add_action( 'admin_enqueue_scripts', array( $this, 'enqueue_admin_scripts' ) );
        add_action( 'wp_ajax_fsbhoa_manage_lighting_service', array( $this, 'ajax_manage_service' ) );

        // AJAX handler for saving settings
        add_action( 'wp_ajax_fsbhoa_save_lighting_settings', array( $this, 'ajax_save_settings' ) );
        // AJAX handler for restarting the service
        add_action( 'wp_ajax_fsbhoa_restart_lighting_service', array( $this, 'ajax_restart_service' ) );
        // AJAX handler for generating API key
        add_action( 'wp_ajax_fsbhoa_generate_lighting_api_key', array( $this, 'ajax_generate_api_key' ) );

    }

    /**
     * Add a top-level admin menu page.
     */
    public function add_settings_page() {
        add_menu_page(
            'FSBHOA Lighting Settings',  // Page Title (appears in browser tab)
            'FSBHOA Lighting',          // Menu Title (appears in the sidebar)
            'manage_options',           // Capability required
            $this->page_slug,           // Menu Slug (unique ID, e.g., 'fsbhoa-lighting-settings')
            array( $this, 'render_settings_page' ), // Function to render the page
            'dashicons-lightbulb',      // Icon (can choose from Dashicons list)
            26                          // Position
        );
    }

    /**
     * Register settings, sections, and fields.
     */
    public function register_settings() {
        register_setting(
            $this->option_group,
            $this->option_name,
            array( $this, 'sanitize_settings' )
        );

        // --- Sections ---
        add_settings_section('fsbhoa_lighting_section_service', 'Go Service Configuration', null, $this->page_slug);
        add_settings_section('fsbhoa_lighting_section_plcs', 'PLC Network Addresses', null, $this->page_slug);
        add_settings_section('fsbhoa_lighting_section_api', 'API Key for Go Service', null, $this->page_slug);
        add_settings_section('fsbhoa_lighting_section_map', 'Monitor Map Configuration', null, $this->page_slug);


        // --- Fields ---
        add_settings_field('go_service_port', 'Go Service Listen Port', array($this, 'render_field'), $this->page_slug, 'fsbhoa_lighting_section_service', ['id' => 'go_service_port', 'type' => 'number', 'default' => 8085, 'desc' => 'Port for the Go service HTTP server.']);
        add_settings_field(
		'log_file_path', 
		'Go Service Log File Path', 
		array($this, 'render_field'), 
		$this->page_slug, 
		'fsbhoa_lighting_section_service', 
		[
			'id' => 'log_file_path', 
			'type' => 'text', 
			'default' => $this->default_log_path, 
			'desc' => 'Full path to the log file. Use "stdout" to log to console.',
			'placeholder' => $this->default_log_path
		]
	);
        add_settings_field('plc1_address', 'PLC #1 Address (Lodge)', array($this, 'render_field'), $this->page_slug, 'fsbhoa_lighting_section_plcs', ['id' => 'plc1_address', 'placeholder' => 'e.g., 192.168.1.201:502']);
        add_settings_field('plc2_address', 'PLC #2 Address (Pool)', array($this, 'render_field'), $this->page_slug, 'fsbhoa_lighting_section_plcs', ['id' => 'plc2_address', 'placeholder' => 'e.g., 192.168.1.202:502']);
        add_settings_field(
            'go_service_api_key',
            'Go Service API Key',
            array($this, 'render_api_key_field'),
            $this->page_slug,
            'fsbhoa_lighting_section_api',
            ['id' => 'go_service_api_key', 'desc' => 'Secret key used by the Go service to fetch configuration.']
        );

        add_settings_field(
            'map_image_url',
            'Map Background Image',
            array($this, 'render_media_uploader_field'),
            $this->page_slug,
            'fsbhoa_lighting_section_map',
            ['id' => 'map_image_url', 'desc' => 'Upload or select the map image from the Media Library.']
        );
    }


    /**
     * Generic field rendering callback.
     */
    public function render_field($args) {
        $options = get_option($this->option_name, []);
        $id      = $args['id'];
        $type    = $args['type'] ?? 'text';
        $default = $args['default'] ?? '';
        $desc    = $args['desc'] ?? '';
        $value   = isset($options[$id]) ? $options[$id] : $default;
        $placeholder = $args['placeholder'] ?? '';

        printf(
            '<input type="%s" name="%s[%s]" value="%s" class="%s" placeholder="%s">',
            esc_attr($type),
            esc_attr($this->option_name), // e.g., fsbhoa_lighting_settings[go_service_port]
            esc_attr($id),
            esc_attr($value),
            ($type === 'number') ? 'small-text' : 'regular-text',
            esc_attr($placeholder)
        );
        if ($desc) {
            echo '<p class="description">' . esc_html($desc) . '</p>';
        }
    }
    
    /**
     * --- Renders the media uploader button and preview ---
     */
    public function render_media_uploader_field($args) {
        $options = get_option($this->option_name, []);
        $id      = $args['id'];
        $desc    = $args['desc'] ?? '';
        $value   = isset($options[$id]) ? $options[$id] : '';
        ?>
        <div class="fsbhoa-media-uploader-wrapper">
            <input type="text" 
                   name="<?php echo esc_attr($this->option_name); ?>[<?php echo esc_attr($id); ?>]" 
                   id="<?php echo esc_attr($id); ?>" 
                   value="<?php echo esc_attr($value); ?>" 
                   class="regular-text"
                   placeholder="Click Upload to select an image"
                   />
            <button type="button" class="button" id="fsbhoa-upload-<?php echo esc_attr($id); ?>-button">Upload/Select Image</button>
            <p class="description"><?php echo esc_html($desc); ?></p>
            
            <div id="<?php echo esc_attr($id); ?>-preview-wrapper" style="margin-top: 10px;">
                <?php if ($value): ?>
                    <img src="<?php echo esc_url($value); ?>" style="max-width: 400px; height: auto; border: 1px solid #ddd;">
                <?php endif; ?>
            </div>
        </div>
        <?php
    }

    /**
     * Sanitize settings before saving.
     */
    public function sanitize_settings( $input ) {
        $output = get_option($this->option_name, []);
        // Sanitize each field appropriately
        $output['go_service_port'] = isset( $input['go_service_port'] ) ? absint( $input['go_service_port'] ) : 8085;
        $output['log_file_path'] = isset( $input['log_file_path'] ) ? sanitize_text_field( $input['log_file_path'] ) : $this->default_log_path;
        $output['plc1_address'] = isset( $input['plc1_address'] ) ? sanitize_text_field( $input['plc1_address'] ) : '';
        $output['plc2_address'] = isset( $input['plc2_address'] ) ? sanitize_text_field( $input['plc2_address'] ) : '';
        $output['go_service_api_key'] = isset( $input['go_service_api_key'] ) ? sanitize_text_field( $input['go_service_api_key'] ) : ($output['go_service_api_key'] ?? '');
        $output['map_image_url'] = isset( $input['map_image_url'] ) ? esc_url_raw( $input['map_image_url'] ) : '';

        return $output;
    }

    /**
     * Writes the Go service config file.
     */
    private function write_go_service_config() {
        $options = get_option($this->option_name, []);
        $config = [
            'ListenPort' => ':' . ($options['go_service_port'] ?? 8085), // Go expects ":port" format
            'LogFilePath' => $options['log_file_path'] ?? $this->default_log_path,
            'PLCs'       => [
                // Store PLC addresses directly in the format Go expects (map[int]string)
                1 => $options['plc1_address'] ?? '',
                2 => $options['plc2_address'] ?? '',
            ],
            'WordPressAPIKey' => $options['go_service_api_key'] ?? '',
            'WordPressAPIBaseURL' => site_url(),
        ];

        $json_data = json_encode($config, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES);
        $config_dir = dirname($this->config_file_path);
        if (!is_dir($config_dir)) {
            mkdir($config_dir, 0755, true); // Create /var/lib/fsbhoa if needed
        }
        // Ensure the directory is writable by the web server
        chown($config_dir, 'www-data');
        chgrp($config_dir, 'www-data');

        file_put_contents($this->config_file_path, $json_data);
         // Set file permissions so Go service (running as 'pi') can read it
        chmod($this->config_file_path, 0644);
    }

    /**
     * AJAX handler to save settings and write config file.
     */
    public function ajax_save_settings() {
        check_ajax_referer('fsbhoa_lighting_settings_nonce', 'nonce');
        if (!current_user_can('manage_options')) { wp_send_json_error('Permission denied.', 403); }

        $options_input = isset($_POST['options']) ? $_POST['options'] : [];
        $sanitized_options = [];
        // Rebuild the input array in the format expected by sanitize_settings
        foreach($options_input as $opt) {
            $sanitized_options[$opt['name']] = $opt['value'];
        }

        $updated_options = $this->sanitize_settings($sanitized_options);
        update_option($this->option_name, $updated_options);

        $this->write_go_service_config(); // Write the JSON file after saving options
        // We do NOT trigger a sync here, as this requires a service restart.

        wp_send_json_success('Settings saved. Config file updated.');
    }



    /**
     * Renders the special read-only field for the API key with a generate button.
     */
    public function render_api_key_field($args) {
        $options = get_option($this->option_name, []);
        $id      = $args['id'];
        $value   = isset($options[$id]) ? $options[$id] : '';
        $desc    = $args['desc'] ?? '';
        ?>
        <input type="text" name="<?php echo esc_attr($this->option_name); ?>[<?php echo esc_attr($id); ?>]" id="<?php echo esc_attr($id); ?>" value="<?php echo esc_attr($value); ?>" class="regular-text" readonly="readonly" placeholder="Click generate to create a new key" />
        <button type="button" class="button" id="fsbhoa-generate-<?php echo esc_attr( $id ); ?>-button">Generate New Key</button>
        <p class="description"><?php echo esc_html($desc); ?></p>
        <?php
    }

    /**
     * AJAX handler to generate a new API key.
     */
    public function ajax_generate_api_key() {
        check_ajax_referer('fsbhoa_lighting_settings_nonce', 'nonce'); // Use the save nonce for simplicity
        if (!current_user_can('manage_options')) { wp_send_json_error('Permission denied.', 403); }
        $new_key = base64_encode(random_bytes(32));
        wp_send_json_success(['api_key' => $new_key]);
    }

    /**
     * Render the settings page HTML, including the service control panel.
     */
    public function render_settings_page() {
        ?>
        <div class="wrap" id="fsbhoa-lighting-settings-page">
            <h1>FSBHOA Lighting Control Settings</h1>
            <?php settings_errors(); ?>
            <?php do_settings_sections( $this->page_slug ); ?>
            <p class="submit">
                <button type="button" id="fsbhoa-save-lighting-settings-button" class="button button-primary">Save Settings</button>
                <span id="fsbhoa-save-feedback" style="display: none; margin-left: 10px; vertical-align: middle;"></span>
            </p>

            <hr>
            <h2>Service Control</h2>
            <p>Manage the background Go service that communicates with the PLCs.</p>
            <table class="form-table">
                <tbody>
                    <tr>
                        <th scope="row">Lighting Service Status</th>
                        <td>
                            <span id="status-fsbhoa-lighting.service" class="fsbhoa-status-indicator">Checking...</span>
                        </td>
                    </tr>
                    <tr>
                        <th scope="row">Actions</th>
                        <td>
                            <button class="button service-command-btn" data-service="fsbhoa-lighting.service" data-command="start">Start</button>
                            <button class="button service-command-btn" data-service="fsbhoa-lighting.service" data-command="stop">Stop</button>
                            <button class="button service-command-btn button-primary" data-service="fsbhoa-lighting.service" data-command="restart">Restart</button>
                            <span id="fsbhoa-service-cmd-feedback" style="display: none; margin-left: 10px; vertical-align: middle;"></span>
                        </td>
                    </tr>
                </tbody>
            </table>
            <style>
                /* Simple status indicator styles */
                .fsbhoa-status-indicator { padding: 3px 8px; border-radius: 3px; color: white; font-weight: bold; }
                .fsbhoa-status-indicator.is-running { background-color: #228B22; } /* ForestGreen */
                .fsbhoa-status-indicator.is-stopped { background-color: #DC143C; } /* Crimson */
            </style>
        </div>
        <?php
    }

    /**
     * AJAX handler to manage (start, stop, restart, status) the Go service.
     */
    public function ajax_manage_service() {
        check_ajax_referer('fsbhoa_manage_lighting_nonce', 'nonce'); // Use a new nonce
        if (!current_user_can('manage_options')) { wp_send_json_error('Permission denied.', 403); }

        $service = 'fsbhoa-lighting.service'; // Hardcoded for this handler
        $command = isset($_POST['command']) ? sanitize_key($_POST['command']) : 'status';
        $allowed_commands = ['start', 'stop', 'restart', 'status'];

        if (!in_array($command, $allowed_commands)) {
            wp_send_json_error('Invalid command.', 400);
        }

        // Validate service name just in case (optional but good practice)
        if ($service !== 'fsbhoa-lighting.service') {
             wp_send_json_error('Invalid service specified.', 400);
        }

        $sys_command = sprintf('sudo /bin/systemctl %s %s 2>&1', escapeshellarg($command), escapeshellarg($service));
        $output = [];
        $return_var = 0;
        exec($sys_command, $output, $return_var);

        // Parse status specifically
        if ($command === 'status') {
            $is_running = $return_var === 0; // systemctl status returns 0 if active/running
            wp_send_json_success([
                'status' => $is_running ? 'running' : 'stopped',
                'output' => implode("\n", $output)
            ]);
        } elseif ($return_var === 0) {
            wp_send_json_success(['message' => "Command '{$command}' sent successfully.", 'output' => implode("\n", $output)]);
        } else {
            wp_send_json_error('Command failed: ' . implode("\n", $output), 500);
        }
    }


    /**
     * Enqueue JavaScript for the settings page.
     */
    public function enqueue_admin_scripts($hook) {
        if ($hook !== 'toplevel_page_' . $this->page_slug) { return; } // Correct hook for top-level

        wp_enqueue_media();

        $script_handle = 'fsbhoa-lighting-settings-script';
        wp_enqueue_script($script_handle, plugin_dir_url(__FILE__) . '../assets/js/admin-settings.js', ['jquery', 'media-upload'], '1.1.0', true); // Added media-upload
        wp_localize_script($script_handle, 'fsbhoa_lighting_admin_vars', [
            'ajax_url' => admin_url('admin-ajax.php'),
            'save_nonce' => wp_create_nonce('fsbhoa_lighting_settings_nonce'),
            'manage_nonce' => wp_create_nonce('fsbhoa_manage_lighting_nonce') // New nonce for manage actions
        ]);
    }
} // End of class Fsbhoa_Lighting_Admin_Settings


// Instantiate the class
new Fsbhoa_Lighting_Admin_Settings();

// Function to check if sudoers rule likely exists (basic check)
function fsbhoa_lighting_check_sudoers() {
    // Attempt to run the status command via sudo -n (non-interactive) as www-data.
    // This is not foolproof but gives an indication.
    exec('sudo -n -u www-data /bin/systemctl status fsbhoa-lighting.service 2>&1', $output, $return_var);
    // If return_var is 0, permission likely exists. If not 0, it likely failed.
    return $return_var === 0;
}

// Hook to display admin notice if check fails
function fsbhoa_lighting_sudoers_notice() {
    // Only show to admins who can manage options
    if ( current_user_can( 'manage_options' ) && ! fsbhoa_lighting_check_sudoers() ) {
        ?>
        <div class="notice notice-warning is-dismissible">
            <p><strong>FSBHOA Lighting Control:</strong> The web server user (www-data) may lack the necessary permissions to restart the Go service. Please refer to the plugin's README for instructions on configuring <code>/etc/sudoers.d/</code>.</p>
        </div>
        <?php
    }
}
add_action( 'admin_notices', 'fsbhoa_lighting_sudoers_notice' );

