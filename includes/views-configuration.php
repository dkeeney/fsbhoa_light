<?php defined( 'ABSPATH' ) or die( 'Unauthorized Access' ); ?>

<style>
    .section-divider { border-top: 2px solid #ccd0d4; margin-top: 40px; padding-top: 20px; }
    .wp-list-table { margin-top: 20px; }
    .form-table { margin-top: 10px; }
</style>

<div class="wrap">
    <div id="fsbhoa-zone-manager-app">
        <h1>Lighting Zone Management</h1>
        <a href="#" id="add-new-zone-btn" class="page-title-action">Add New Zone</a>
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
