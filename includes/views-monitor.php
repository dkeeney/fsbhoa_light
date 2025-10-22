<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>
<style>
    /* Styles for the monitor page */
    #fsbhoa-monitor-app .wrap { /* Target the wrap inside our specific app ID */
        max-width: 700px; /* Adjust this value as needed */
        margin-left: auto;
        margin-right: auto;
    }
    .lighting-monitor-wrapper h1 { margin-bottom: 20px; }
    .wp-list-table th, .wp-list-table td { padding: 8px 10px; vertical-align: middle; }

    /* Override Icon Styles */
    a.override-link {
        font-size: 1.8em; /* Make icons slightly larger */
        text-decoration: none;
        margin-right: 15px;
        cursor: pointer;
        color: #f5a623; /* Yellow/Orange for ON state */
        opacity: 1;
        transition: opacity 0.2s ease-in-out;
    }
    a.override-link.is-off {
        color: #000000; /* Black color for OFF state */
    }
    a.override-link.is-disabled {
        color: #555555; /* Lighter gray when disabled */
        cursor: not-allowed;
        opacity: 0.6;
    }
    a.override-link:hover:not(.is-disabled) {
        opacity: 0.7;
    }
</style>

<div class="wrap" id="fsbhoa-monitor-app">
    <h1>Live Lighting Status</h1>
    
    <div id="status-container">
        <p>Loading status...</p>
    </div>
</div>
