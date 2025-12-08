<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>

<style>
    #fsbhoa-map-monitor-app {
        max-width: 1200px;
        margin: 0 auto;
    }
    #map-monitor-wrapper {
        position: relative;
        border: 2px solid #ccc;
        background: #f4f4f4;
        overflow: hidden;
        min-height: 600px;
    }
    #map-monitor-image {
        display: block;
        width: 100%;
        height: auto;
    }
    #map-pin-overlay {
        position: absolute;
        top: 0; left: 0; width: 100%; height: 100%;
    }
    #map-status-indicator {
        margin-top: 10px;
        font-style: italic;
        color: #555;
    }

    /* --- Pin Base --- */
    .map-pin-live {
        position: absolute;
        border-radius: 50%;
        border: 2px solid white;
        box-shadow: 0 1px 3px rgba(0,0,0,0.5);
        transition: background-color 0.3s ease, box-shadow 0.3s ease;
        cursor: pointer;
    }
    /* Sizes */
    .map-pin-small  { width: 14px; height: 14px; }
    .map-pin-medium { width: 20px; height: 20px; }
    .map-pin-large  { width: 26px; height: 26px; }

    /* --- State Colors (Matching List View) --- */

    /* 1. Auto ON (Yellow) */
    .map-pin-live.status-auto-on {
        background-color: #f5a623;
        box-shadow: 0 0 12px 4px #f5a623; /* Glow */
    }

    /* 2. Manual ON (Orange) */
    .map-pin-live.status-manual-on {
        background-color: #ff5722;
        box-shadow: 0 0 12px 4px #ff5722; /* Glow */
    }

    /* 3. Manual OFF (Blue) - Should be ON but is OFF */
    .map-pin-live.status-manual-off {
        background-color: #2196f3;
        border-color: #ddd;
    }

    /* 4. Auto OFF (Black/Gray) */
    .map-pin-live.status-auto-off {
        background-color: #444444;
    }

    /* 5. Partial / Error (Pulsing Orange) */
    .map-pin-live.status-partial {
        background-color: #ff5722;
        animation: map-pulse 1.5s infinite;
    }

    @keyframes map-pulse {
        0% { opacity: 1; transform: translate(-50%, -50%) scale(1); }
        50% { opacity: 0.7; transform: translate(-50%, -50%) scale(0.8); }
        100% { opacity: 1; transform: translate(-50%, -50%) scale(1); }
    }
</style>


<div class="wrap" id="fsbhoa-map-monitor-app">
    <h1>Live Lighting Map</h1>

    <div id="map-monitor-wrapper">
        <img id="map-monitor-image" src="" alt="Loading map...">
        <div id="map-pin-overlay"></div>
    </div>
    
    <div id="map-status-indicator">
        <p>Loading status...</p>
    </div>
</div>
