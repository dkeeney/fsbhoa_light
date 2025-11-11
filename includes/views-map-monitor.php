<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>

<style>
    #fsbhoa-map-monitor-app {
        max-width: 1200px; /* Adjust to your map's ideal width */
        margin: 0 auto;
    }
    #map-monitor-wrapper {
        position: relative;
        border: 2px solid #ccc;
        background: #f4f4f4;
        overflow: hidden;
        /* Prevent layout shift while image loads */
        min-height: 600px; 
    }
    #map-monitor-image {
        display: block;
        width: 100%;
        height: auto;
    }
    #map-pin-overlay {
        position: absolute;
        top: 0;
        left: 0;
        width: 100%;
        height: 100%;
    }
    #map-status-indicator {
        margin-top: 10px;
        font-style: italic;
        color: #555;
    }

    /* --- Pin Styles (must match admin editor) --- */
    .map-pin-live {
        position: absolute;
        border-radius: 50%;
        border: 2px solid white;
        box-shadow: 0 1px 3px rgba(0,0,0,0.5);
        transition: background-color 0.3s ease;
    }
    .map-pin-small  { width: 14px; height: 14px; }
    .map-pin-medium { width: 20px; height: 20px; }
    .map-pin-large  { width: 26px; height: 26px; }

    /* --- Pin Status Colors --- */
    .map-pin-live.is-on {
        background-color: #f5a623; /* Bright yellow/orange */
        /* Add a glow for ON state */
        box-shadow: 0 0 8px 3px #f5a623;
    }
    .map-pin-live.is-off {
        background-color: #555555; /* Dark Gray */
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
