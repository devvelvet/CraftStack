/**
 * CraftStack frontend application
 * HTMX dynamic updates + Alpine.js client-side reactivity
 */

// HTMX beforereverse settings
document.addEventListener('DOMContentLoaded', function() {
    // HTMX beforereverse error handler
    document.body.addEventListener('htmx:responseError', function(event) {
        console.error('HTMX error:', event.detail);
        showToast('request failed: ' + event.detail.xhr.status, 'error');
    });

    // HTMX  loading display
    document.body.addEventListener('htmx:beforeRequest', function(event) {
        event.detail.elt.classList.add('htmx-loading');
    });

    document.body.addEventListener('htmx:afterRequest', function(event) {
        event.detail.elt.classList.remove('htmx-loading');
    });
});

/**
 * DaisyUI store component use notification display
 */
function showToast(message, type) {
    type = type || 'info';
    var alertClass = {
        'info': 'alert-info',
        'success': 'alert-success',
        'warning': 'alert-warning',
        'error': 'alert-error'
    }[type] || 'alert-info';

    var toast = document.createElement('div');
    toast.className = 'toast toast-top toast-end z-50';
    toast.innerHTML = '<div class="alert ' + alertClass + '"><span>' + message + '</span></div>';
    document.body.appendChild(toast);

    setTimeout(function() {
        toast.remove();
    }, 4000);
}

/**
 * instance control send command
 */
function controlInstance(instanceId, action) {
    var actionNames = {
        'start': 'start',
        'stop': 'stop',
        'restart': 'restart',
        'kill': 'force shutdown'
    };

    var actionName = actionNames[action] || action;
    showToast(actionName + ' command send during...', 'info');

    fetch('/api/instances/' + instanceId + '/control', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: action })
    })
    .then(function(response) { return response.json(); })
    .then(function(data) {
        showToast(data.message, data.status === 'accepted' ? 'success' : 'error');
        // HTMX partial refresh
        var table = document.querySelector('[hx-get*="instances-table"]');
        if (table) htmx.trigger(table, 'htmx:load');
        var controls = document.getElementById('instance-controls');
        if (controls) htmx.trigger(controls, 'htmx:load');
        // console control command state notification
        if (data.status === 'accepted') {
            document.dispatchEvent(new CustomEvent('instance-controlled', { detail: { action: action } }));
        }
    })
    .catch(function(err) {
        showToast('send command failed: ' + err.message, 'error');
    });
}

/**
 * delete instance request
 */
function deleteInstance(instanceId, instanceName, removeData) {
    var dataMsg = removeData ? '\n\nNote: server data(volume) together delete!' : '';
    if (!confirm('instance "' + instanceName + '"() delete?' + dataMsg)) {
        return;
    }

    showToast('delete instance during...', 'info');

    fetch('/api/instances/' + instanceId, {
        method: 'DELETE',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ remove_data: !!removeData })
    })
    .then(function(response) { return response.json(); })
    .then(function(data) {
        if (data.status === 'success') {
            showToast(data.message, 'success');
            // instance list page if table refresh, detail page if list as move
            if (window.location.pathname === '/instances') {
                var table = document.querySelector('[hx-get*="instances-table"]');
                if (table) htmx.trigger(table, 'htmx:load');
                setTimeout(function() { location.reload(); }, 1000);
            } else {
                setTimeout(function() { window.location.href = '/instances'; }, 1000);
            }
        } else {
            showToast(data.message, 'error');
        }
    })
    .catch(function(err) {
        showToast('delete failed: ' + err.message, 'error');
    });
}

/**
 * restore backup request
 */
function restoreBackup(instanceId, backupPath) {
    if (!confirm('Restore instance from this backup?\n\nPath: ' + backupPath + '\n\nNote: current server data will be overwritten by the backup contents.')) {
        return;
    }

    var stopBefore = confirm('restore before instance stop?\n(if running recommended)');

    showToast('restore backup request during...', 'info');

    fetch('/api/backups/' + instanceId + '/restore', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ backup_path: backupPath, stop_before: stopBefore })
    })
    .then(function(response) { return response.json(); })
    .then(function(data) {
        showToast(data.message, data.status === 'success' ? 'success' : 'error');
    })
    .catch(function(err) {
        showToast('restore request failed: ' + err.message, 'error');
    });
}

/**
 * create backup request
 */
function createBackup(instanceId) {
    showToast('create backup request during...', 'info');

    fetch('/api/backups/' + instanceId, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' }
    })
    .then(function(response) { return response.json(); })
    .then(function(data) {
        showToast(data.message, data.status === 'accepted' ? 'success' : 'error');
        // backup list refresh
        var backupList = document.getElementById('backup-list');
        if (backupList) {
            htmx.trigger(backupList, 'htmx:load');
        }
        var backupDiv = document.getElementById('backups-' + instanceId);
        if (backupDiv) {
            htmx.trigger(backupDiv, 'htmx:load');
        }
    })
    .catch(function(err) {
        showToast('backup request failed: ' + err.message, 'error');
    });
}
