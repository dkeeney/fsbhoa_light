<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>

<style>
    /* --- General Layout --- */
    .section-divider { border-top: 2px solid #ccd0d4; margin-top: 40px; padding-top: 20px; }
    .wp-list-table { margin-top: 20px; }
    .form-table { margin-top: 10px; }

    /* Reduce padding on all table cells */
    .wp-list-table th,
    .wp-list-table td {
        padding-top: 4px;
        padding-bottom: 4px;
        line-height: 1.3;
        vertical-align: middle;
    }

    /* Make the schedule dropdown smaller */
    #fsbhoa-zone-manager-app .wp-list-table td select {
        padding: 2px 4px;
        min-height: auto;
        height: auto;
    }

    /* --- Status Bulb Styles (Diagnostic) --- */
    .monitor-bulb {
        font-size: 20px; /* Slightly smaller for list view */
        width: 20px;
        height: 20px;
        border-radius: 50%;
        background-color: transparent;
        transition: all 0.3s ease;
        display: inline-block;
        vertical-align: middle;
    }

    /* 1. Schedule ON (Auto) -> YELLOW */
    .monitor-bulb.status-auto-on {
        color: #f5a623;
        text-shadow: 0 0 8px #f5a623;
    }

    /* 2. Manual ON (Override) -> ORANGE */
    .monitor-bulb.status-manual-on {
        color: #ff5722;
        text-shadow: 0 0 8px #ff5722;
    }

    /* 3. Manual OFF (Override) -> BLUE */
    .monitor-bulb.status-manual-off {
        color: #2196f3;
        text-shadow: 0 0 8px #2196f3;
    }

    /* 4. Schedule OFF (Normal) -> BLACK/GRAY */
    .monitor-bulb.status-auto-off {
        color: #bbbbbb; /* Light gray for visibility in admin table */
    }

    /* 5. Partial Zone -> PULSING ORANGE */
    .monitor-bulb.status-partial {
        color: #ff5722;
        animation: pulse-bulb 1.5s infinite;
    }

    @keyframes pulse-bulb {
        0% { opacity: 1; transform: scale(1); text-shadow: 0 0 4px #ff5722; }
        50% { opacity: 0.5; transform: scale(0.9); text-shadow: 0 0 0 transparent; }
        100% { opacity: 1; transform: scale(1); text-shadow: 0 0 4px #ff5722; }
    }

    /* --- Print-Only Styles --- */
    @media print {
        /* Hide all the WordPress admin UI */
        #adminmenumain, #wpadminbar, #wpfooter, .notice, .wrap > h1:first-of-type {
            display: none;
        }

        /* Reset the page layout for printing */
        #wpcontent, #wpbody, #wpbody-content, .wrap {
            margin: 0 !important;
            padding: 0 !important;
            width: 100%;
            box-shadow: none;
            background: #fff;
        }

        /* Hide all buttons, forms, and links */
        .page-title-action,
        #zone-form-container,
        #schedule-form-container,
        #mapping-form-container,
        .edit-zone-link,
        .delete-zone-link,
        .edit-schedule-link,
        .delete-schedule-link,
        .edit-mapping-link,
        .delete-mapping-link,
        .test-btn { /* Hide test buttons */
            display: none !important;
        }

        /* Hide the "Actions" column in all tables */
        .wp-list-table th:last-child,
        .wp-list-table td:last-child {
            display: none;
        }

        /* Ensure all other columns are visible */
        .wp-list-table th, .wp-list-table td {
            display: table-cell;
        }

        /* Style for printing */
        h1 {
            font-size: 18pt;
            margin-top: 0;
            padding-top: 0;
        }
        .wp-list-table {
            margin-top: 10px;
        }
        .section-divider {
            margin-top: 0;
            padding-top: 20px;
            border: none;
            page-break-before: always; /* Start new sections on a new page */
        }
    }
</style>

<div class="wrap">
    <div id="fsbhoa-zone-manager-app">
        <h1>Lighting Zone Management</h1>
        <a href="#" id="add-new-zone-btn" class="page-title-action">Add New Zone</a>
        <a href="#" id="fsbhoa-debug-download-btn" class="page-title-action" style="margin-right: 10px; float: right; background: #f0f0f1; color: #0073aa;">Download Config JSON</a>
        <a href="#" id="fsbhoa-print-config-btn" class="page-title-action" style="background: #007cba; color: white; float: right;">Print Configuration</a>
        
        <div id="zones-list-container"></div>
        <div id="zone-form-container" style="display: none;"></div>
        <button id="save-zone-assignments-btn" class="button button-primary" style="margin-top: 20px; display: none;">Save Schedule Assignments</button>
    </div>

    <div id="fsbhoa-schedules-app" class="section-divider">
        <h1>Lighting Schedule Management</h1>
        <a href="#" id="add-new-schedule-btn" class="page-title-action">Add New Schedule</a>
        <div id="schedules-list-container"></div>
        <div id="schedule-form-container" style="display: none;"></div>
    </div>

    <div id="fsbhoa-mapping-manager-app" class="section-divider">
        <h1>PLC Output to Relay Mapping</h1>
        <a href="#" id="add-new-mapping-btn" class="page-title-action">Add New Mapping</a>
        <div id="mappings-list-container"></div>
        <div id="mapping-form-container" style="display: none;"></div>
    </div>
</div>

