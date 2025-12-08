<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>
<style>
    #fsbhoa-monitor-app { max-width: 800px; margin-top: 15px; }
    
    /* --- Compact Table Override --- */
    .compact-monitor-table {
        border-spacing: 0;
    }

    /* Maximum Squish */
    .compact-monitor-table td, 
    .compact-monitor-table th {
        padding: 4px 6px !important; /* Very tight padding */
        vertical-align: middle !important;
        height: 28px; /* Force rows to be short */
        line-height: 1 !important;
    }

    /* Alignments */
    .state-wrapper { display: flex; align-items: center; }
    .monitor-bulb { margin-right: 5px; font-size: 16px !important; width: 16px !important; height: 16px !important; line-height: 16px !important; display:block;}
    .state-label { font-size: 10px; font-weight: 700; color: #555; text-transform: uppercase; letter-spacing: 0.5px; }

    /* --- Override Links (Micro Buttons) --- */
    .override-link {
        text-decoration: none;
        font-weight: bold;
        padding: 1px 6px;
        border-radius: 2px;
        font-size: 10px;
        border: 1px solid #ddd;
        background: #f7f7f7;
        display: inline-block;
        line-height: 16px;
    }
    .override-link[data-state="on"] { color: #46b450; } /* Green */
    .override-link[data-state="off"] { color: #b32d2e; } /* Red */
    .override-link:hover { background: #fff; border-color: #999; }
    .override-link.is-disabled {
        color: #ddd !important;
        pointer-events: none;
        border-color: #f0f0f1;
        background: none;
    }

    /* --- Bulb Colors --- */
    .monitor-bulb.status-auto-on { color: #f5a623; text-shadow: 0 0 5px #f5a623; }
    .monitor-bulb.status-manual-on { color: #ff5722; text-shadow: 0 0 5px #ff5722; }
    .monitor-bulb.status-manual-off { color: #2196f3; text-shadow: 0 0 5px #2196f3; }
    .monitor-bulb.status-auto-off { color: #cccccc; }
    
    .monitor-bulb.status-pulsing {
        color: #ff5722;
        animation: pulse-bulb 1.5s infinite;
    }
    @keyframes pulse-bulb {
        0% { opacity: 1; transform: scale(1); }
        50% { opacity: 0.5; transform: scale(0.8); }
        100% { opacity: 1; transform: scale(1); }
    }

    /* --- Legend Styles --- */
    .fsbhoa-legend {
        margin-top: 20px;
        padding: 15px;
        background: #fff;
        border: 1px solid #e5e5e5;
        border-radius: 4px;
        display: flex;
        flex-wrap: wrap;
        gap: 20px;
        align-items: center;
        justify-content: center;
    }
    .legend-item {
        display: flex;
        align-items: center;
        font-size: 12px;
        color: #555;
    }
    /* Override bulb size for legend to be consistent */
    .legend-item .monitor-bulb {
        margin-right: 6px;
        width: 18px; 
        height: 18px;
        font-size: 18px;
        line-height: 18px;
    }
</style>

<div class="wrap" id="fsbhoa-monitor-app">
    <h1 class="wp-heading-inline">Live Status Monitor</h1>
    <a href="#" id="fsbhoa-manual-sync-btn" class="page-title-action">Clear Overrides (Sync)</a>
    <hr class="wp-header-end">

    <div id="status-container">
        <p>Loading status...</p>
    </div>
    <div class="fsbhoa-legend">
        <div class="legend-item">
            <span class="dashicons dashicons-lightbulb monitor-bulb status-auto-on"></span>
            <strong>Auto ON</strong>&nbsp;(Schedule)
        </div>
        <div class="legend-item">
            <span class="dashicons dashicons-lightbulb monitor-bulb status-manual-on"></span>
            <strong>Manual ON</strong>&nbsp;(Override)
        </div>
        <div class="legend-item">
            <span class="dashicons dashicons-lightbulb monitor-bulb status-manual-off"></span>
            <strong>Manual OFF</strong>&nbsp;(Override)
        </div>
        <div class="legend-item">
            <span class="dashicons dashicons-lightbulb monitor-bulb status-auto-off"></span>
            <strong>OFF</strong>
        </div>
        <div class="legend-item">
            <span class="dashicons dashicons-lightbulb monitor-bulb status-partial status-pulsing"></span>
            <strong>Partial / Error</strong>
        </div>
    </div>
</div>
