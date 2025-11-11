<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>
<style>
    /* Styles for the monitor page */
    #fsbhoa-monitor-app { /* Target the wrap inside our specific app ID */
        max-width: 500px; /* Adjust this value as needed */
        margin-left: auto;
        margin-right: auto;
    }
    /* --- Center align specific columns --- */
    #fsbhoa-monitor-app .wp-list-table th:nth-child(2),
    #fsbhoa-monitor-app .wp-list-table td:nth-child(2),
    #fsbhoa-monitor-app .wp-list-table th:nth-child(3),
    #fsbhoa-monitor-app .wp-list-table td:nth-child(3) {
        text-align: center;
    }

    .lighting-monitor-wrapper h1 { margin-bottom: 20px; }
    .wp-list-table th, .wp-list-table td { padding: 8px 10px; vertical-align: middle; }

    /* --- Lightbulb Status Styles --- */
    .monitor-bulb {
        font-size: 2em; /* Makes the bulb large */
        vertical-align: middle;
        transition: color 0.3s ease;
    }
    .monitor-bulb.is-on {
        color: #f5a623; /* Bright yellow/orange for ON */
    }
    .monitor-bulb.is-off {
        color: #555555; /* Dark gray for OFF */
    }

    /* --- Override Link Styles --- */
    a.override-link {
        font-weight: bold;
        text-decoration: none;
        padding: 4px 8px;
    }
    a.override-link.is-disabled {
        color: #999;
        text-decoration: none;
        cursor: not-allowed;
        opacity: 0.6;
    }
    a.override-link:not(.is-disabled):hover {
        background-color: #f0f0f0;
        border-radius: 3px;
    }
</style>

<div class="wrap" id="fsbhoa-monitor-app">
    <h1>Live Lighting Status</h1>
    
    <div id="status-container">
        <p>Loading status...</p>
    </div>
</div>
