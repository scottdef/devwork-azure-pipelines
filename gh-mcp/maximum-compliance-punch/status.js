// Trust Center Status Loader
// Simple, elegant, Pike-approved JavaScript

const CONTROLS = [
    'control-org-settings.yml',
    'control-repo-visibility.yml',
    'control-repo-rulesets.yml',
    'control-org-custom-role.yml'
];

// Load status data and update the page
async function loadStatus() {
    try {
        const response = await fetch('status.json');
        const data = await response.json();
        
        updateOverallStatus(data);
        updateControlsTable(data.controls);
        updateGenerationTime(data.generated_at);
    } catch (error) {
        console.error('Failed to load status:', error);
        showError();
    }
}

function updateOverallStatus(data) {
    const indicator = document.getElementById('status-indicator');
    const lastUpdated = document.getElementById('last-updated');
    
    const statusIcon = indicator.querySelector('.status-icon');
    const statusText = indicator.querySelector('.status-text');
    
    // Remove all status classes
    statusIcon.className = 'status-icon';
    statusText.className = 'status-text';
    
    if (data.overall_status === 'operational') {
        statusIcon.textContent = '✅';
        statusText.textContent = 'All Systems Operational';
        statusText.classList.add('status-operational');
    } else if (data.overall_status === 'degraded') {
        statusIcon.textContent = '⚠️';
        statusText.textContent = 'Degraded Performance';
        statusText.classList.add('status-degraded');
    } else if (data.overall_status === 'failed') {
        statusIcon.textContent = '❌';
        statusText.textContent = 'System Issues Detected';
        statusText.classList.add('status-failed');
    } else {
        statusIcon.textContent = '❓';
        statusText.textContent = 'Status Unknown';
    }
    
    lastUpdated.textContent = `Last updated: ${formatTimestamp(data.generated_at)}`;
}

function updateControlsTable(controls) {
    const tbody = document.getElementById('controls-tbody');
    tbody.innerHTML = '';
    
    controls.forEach(control => {
        const row = document.createElement('tr');
        
        // Control name
        const nameCell = document.createElement('td');
        nameCell.textContent = formatControlName(control.name);
        row.appendChild(nameCell);
        
        // Status
        const statusCell = document.createElement('td');
        const statusSpan = document.createElement('span');
        statusSpan.className = `control-status ${getStatusClass(control.status)}`;
        statusSpan.innerHTML = `${getStatusIcon(control.status)} ${control.status.toUpperCase()}`;
        statusCell.appendChild(statusSpan);
        row.appendChild(statusCell);
        
        // Last run
        const lastRunCell = document.createElement('td');
        lastRunCell.textContent = control.last_run ? formatTimestamp(control.last_run) : 'Never';
        row.appendChild(lastRunCell);
        
        // Details link
        const detailsCell = document.createElement('td');
        if (control.run_url) {
            const link = document.createElement('a');
            link.href = control.run_url;
            link.className = 'control-link';
            link.textContent = 'View Run →';
            link.target = '_blank';
            detailsCell.appendChild(link);
        } else {
            detailsCell.textContent = 'N/A';
        }
        row.appendChild(detailsCell);
        
        tbody.appendChild(row);
    });
}

function updateGenerationTime(timestamp) {
    const genTime = document.getElementById('generation-time');
    genTime.textContent = formatTimestamp(timestamp);
}

function formatControlName(filename) {
    // Convert control-org-settings.yml to "Organization Settings"
    return filename
        .replace('.yml', '')
        .replace('control-', '')
        .split('-')
        .map(word => word.charAt(0).toUpperCase() + word.slice(1))
        .join(' ');
}

function getStatusClass(status) {
    const statusMap = {
        'success': 'success',
        'failure': 'failure',
        'pending': 'pending',
        'in_progress': 'pending'
    };
    return statusMap[status] || 'unknown';
}

function getStatusIcon(status) {
    const iconMap = {
        'success': '✓',
        'failure': '✗',
        'pending': '○',
        'in_progress': '○'
    };
    return iconMap[status] || '?';
}

function formatTimestamp(timestamp) {
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now - date;
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins} minute${diffMins > 1 ? 's' : ''} ago`;
    
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours} hour${diffHours > 1 ? 's' : ''} ago`;
    
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays} day${diffDays > 1 ? 's' : ''} ago`;
    
    return date.toLocaleDateString('en-US', { 
        year: 'numeric', 
        month: 'short', 
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
}

function showError() {
    const tbody = document.getElementById('controls-tbody');
    tbody.innerHTML = '<tr><td colspan="4" class="loading">Failed to load status data. Please try again later.</td></tr>';
}

// Load status when page loads
document.addEventListener('DOMContentLoaded', loadStatus);
