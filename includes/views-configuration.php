<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>

<style>
    .section-divider { border-top: 2px solid #ccd0d4; margin-top: 40px; padding-top: 20px; }
    .wp-list-table { margin-top: 20px; }
    .form-table { margin-top: 10px; }

    /* Reduce padding on all table cells */
    .wp-list-table th,
    .wp-list-table td {
        padding-top: 1px;
        padding-bottom: 1px;
        line-height: 1.3;
    }
    
    /* Make the schedule dropdown smaller */
    #fsbhoa-zone-manager-app .wp-list-table td select {
        padding: 2px 4px;
        min-height: auto;
        height: auto;
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
        .delete-mapping-link {
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
        <a href="#" id="fsbhoa-print-config-btn" class="page-title-action" style="background: #007cba; color: white; float: right;">Print Configuration</a>
        <div id="zones-list-container"></div>
        <div id="zone-form-container" style="display: none;"></div>
        <button id="save-zone-assignments-btn" class="button button-primary" style="margin-top: 20px; display: none;">Save Schedule Assignments</button> </div>
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
