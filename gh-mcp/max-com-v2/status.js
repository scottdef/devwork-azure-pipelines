// ═══════════════════════════════════════════════════════════
// Kickbutt Compliance — Trust Center v2
// Status loader, history renderer, auto-refresh
//
// "Clarity is better than cleverness." — Go Proverbs
// ═══════════════════════════════════════════════════════════

const AUTO_REFRESH_SECONDS = 300; // 5 minutes
let refreshCountdown = AUTO_REFRESH_SECONDS;
let refreshTimer = null;

// ── Entry Point ──────────────────────────────────────────────
document.addEventListener('DOMContentLoaded', () => {
    loadStatus();
    startAutoRefresh();
});

async function loadStatus() {
    try {
        const resp = await fetch('status.json?t=' + Date.now());
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const data = await resp.json();

        renderBanner(data);
        renderSummary(data.summary);
        renderControls(data.controls);
        renderCategories(data.summary.categories);
        renderGenerationTime(data.generated_at);
        updateRefreshText('Updated just now');
    } catch (err) {
        console.error('Failed to load status:', err);
        showError();
        updateRefreshText('Update failed');
    }
}

// ── Banner ───────────────────────────────────────────────────
function renderBanner(data) {
    const banner = document.getElementById('status-banner');
    const icon   = document.getElementById('banner-icon');
    const label  = document.getElementById('banner-label');
    const time   = document.getElementById('banner-time');

    // Reset classes
    banner.classList.remove('operational', 'degraded', 'failed');

    const map = {
        operational: { cls: 'operational', ico: '✅', txt: 'All Systems Operational' },
        degraded:    { cls: 'degraded',    ico: '⚠️', txt: 'Partial Degradation Detected' },
        failed:      { cls: 'failed',      ico: '❌', txt: 'Compliance Issues Detected' },
    };
    const s = map[data.overall_status] || { cls: '', ico: '❓', txt: 'Status Unknown' };

    banner.classList.add(s.cls);
    icon.textContent  = s.ico;
    label.textContent = s.txt;
    time.textContent  = 'Last updated: ' + formatTimestamp(data.generated_at);
}

// ── Summary Cards ────────────────────────────────────────────
function renderSummary(summary) {
    setText('sum-total',   summary.total_controls);
    setText('sum-passing', summary.passing);
    setText('sum-failing', summary.failing);
    setText('sum-rate',    summary.average_pass_rate != null ? summary.average_pass_rate + '%' : 'N/A');
}

// ── Controls Table ───────────────────────────────────────────
function renderControls(controls) {
    const tbody = document.getElementById('controls-tbody');
    tbody.innerHTML = '';

    controls.forEach(ctrl => {
        const row = document.createElement('tr');

        // ── Control name + description + category
        const nameCell = document.createElement('td');
        nameCell.innerHTML = `
            <div class="control-name">${formatControlName(ctrl.name)}</div>
            <div class="control-desc">${escapeHtml(ctrl.description || '')}</div>
            <span class="control-category">${escapeHtml(ctrl.category || 'General')}</span>
        `;
        row.appendChild(nameCell);

        // ── History dots (last 5 runs, oldest → newest left-to-right)
        const histCell = document.createElement('td');
        const dotsWrap = document.createElement('div');
        dotsWrap.className = 'history-dots';

        // Reverse so oldest is on the left, newest on the right
        const history = (ctrl.history || []).slice().reverse();

        if (history.length === 0) {
            dotsWrap.innerHTML = '<span style="color:var(--text-3);font-size:0.8rem;">No runs</span>';
        } else {
            history.forEach((run, i) => {
                const dot = document.createElement('div');
                dot.className = `history-dot ${statusClass(run.status)}`;

                // Tooltip
                const tip = document.createElement('div');
                tip.className = 'dot-tooltip';
                tip.innerHTML = `
                    <strong>${run.status.toUpperCase()}</strong><br>
                    Run #${run.run_number || '?'}<br>
                    ${formatTimestamp(run.created_at)}<br>
                    Duration: ${formatDuration(run.duration_seconds)}
                `;
                dot.appendChild(tip);

                // Click navigates to run
                if (run.run_url) {
                    dot.style.cursor = 'pointer';
                    dot.addEventListener('click', () => window.open(run.run_url, '_blank'));
                }

                dotsWrap.appendChild(dot);
            });
        }
        histCell.appendChild(dotsWrap);
        row.appendChild(histCell);

        // ── Current status badge + trend
        const statusCell = document.createElement('td');
        const badge = document.createElement('span');
        badge.className = `status-badge ${statusClass(ctrl.status)}`;
        badge.innerHTML = `${statusIcon(ctrl.status)} ${ctrl.status.toUpperCase()}`;
        statusCell.appendChild(badge);

        if (ctrl.trend && ctrl.trend !== 'unknown') {
            const trend = document.createElement('span');
            trend.className = 'trend-indicator';
            if (ctrl.trend === 'improving')  { trend.className += ' trend-up';   trend.textContent = '▲'; trend.title = 'Improving'; }
            else if (ctrl.trend === 'degrading') { trend.className += ' trend-down'; trend.textContent = '▼'; trend.title = 'Degrading'; }
            else { trend.className += ' trend-flat'; trend.textContent = '—'; trend.title = 'Stable'; }
            statusCell.appendChild(trend);
        }
        row.appendChild(statusCell);

        // ── Pass rate bar
        const rateCell = document.createElement('td');
        rateCell.className = 'rate-cell';

        if (ctrl.pass_rate != null) {
            const tier = ctrl.pass_rate >= 80 ? 'high' : ctrl.pass_rate >= 50 ? 'medium' : 'low';
            rateCell.innerHTML = `
                <div class="rate-bar-wrap">
                    <div class="rate-bar-track">
                        <div class="rate-bar-fill ${tier}" style="width:${ctrl.pass_rate}%"></div>
                    </div>
                    <span class="rate-value ${tier}">${ctrl.pass_rate}%</span>
                </div>
            `;
        } else {
            rateCell.innerHTML = '<span style="color:var(--text-3);">N/A</span>';
        }
        row.appendChild(rateCell);

        // ── Links (docs, all runs, latest run)
        const linksCell = document.createElement('td');
        linksCell.className = 'links-cell';

        if (ctrl.docs_url) {
            linksCell.innerHTML += `<a class="link-item" href="${ctrl.docs_url}" target="_blank"><span class="link-icon">📄</span> Documentation</a>`;
        }
        if (ctrl.all_runs_url) {
            linksCell.innerHTML += `<a class="link-item" href="${ctrl.all_runs_url}" target="_blank"><span class="link-icon">📋</span> All Runs</a>`;
        }
        if (ctrl.run_url) {
            linksCell.innerHTML += `<a class="link-item" href="${ctrl.run_url}" target="_blank"><span class="link-icon">🔗</span> Latest Run</a>`;
        }
        row.appendChild(linksCell);

        tbody.appendChild(row);
    });
}

// ── Category Breakdown ───────────────────────────────────────
function renderCategories(categories) {
    if (!categories || categories.length === 0) return;

    const section = document.getElementById('categories-section');
    const grid    = document.getElementById('categories-grid');
    section.style.display = '';
    grid.innerHTML = '';

    categories.forEach(cat => {
        const pct = cat.total > 0 ? Math.round((cat.passing / cat.total) * 100) : 0;
        const tier = pct >= 80 ? 'high' : pct >= 50 ? 'medium' : 'low';
        const color = tier === 'high' ? 'var(--green)' : tier === 'medium' ? 'var(--amber)' : 'var(--red)';

        const card = document.createElement('div');
        card.className = 'category-card';
        card.innerHTML = `
            <div class="category-name">${escapeHtml(cat.name)}</div>
            <div class="category-bar-track">
                <div class="category-bar-fill" style="width:${pct}%;background:${color}"></div>
            </div>
            <div class="category-stat">${cat.passing}/${cat.total} controls passing · ${pct}%</div>
        `;
        grid.appendChild(card);
    });
}

// ── Generation Timestamp ─────────────────────────────────────
function renderGenerationTime(ts) {
    setText('generation-time', formatTimestamp(ts));
}

// ── Auto-Refresh ─────────────────────────────────────────────
function startAutoRefresh() {
    refreshCountdown = AUTO_REFRESH_SECONDS;

    if (refreshTimer) clearInterval(refreshTimer);
    refreshTimer = setInterval(() => {
        refreshCountdown--;
        if (refreshCountdown <= 0) {
            loadStatus();
            refreshCountdown = AUTO_REFRESH_SECONDS;
        }
        updateRefreshText(`Refreshing in ${formatCountdown(refreshCountdown)}`);
    }, 1000);
}

function formatCountdown(secs) {
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return m > 0 ? `${m}m ${s}s` : `${s}s`;
}

function updateRefreshText(text) {
    setText('refresh-text', text);
}

// ── Helpers ──────────────────────────────────────────────────
function setText(id, text) {
    const el = document.getElementById(id);
    if (el) el.textContent = text;
}

function formatControlName(filename) {
    return filename
        .replace('.yml', '')
        .replace(/^control-/, '')
        .split('-')
        .map(w => w.charAt(0).toUpperCase() + w.slice(1))
        .join(' ');
}

function statusClass(status) {
    const map = { success: 'success', failure: 'failure', pending: 'pending', in_progress: 'pending' };
    return map[status] || 'unknown';
}

function statusIcon(status) {
    const map = { success: '✓', failure: '✗', pending: '○', in_progress: '○' };
    return map[status] || '?';
}

function formatTimestamp(ts) {
    if (!ts) return '—';
    const d   = new Date(ts);
    const now = new Date();
    const ms  = now - d;
    const min = Math.floor(ms / 60000);

    if (min < 1) return 'Just now';
    if (min < 60) return `${min}m ago`;
    const hrs = Math.floor(min / 60);
    if (hrs < 24) return `${hrs}h ${min % 60}m ago`;
    const days = Math.floor(hrs / 24);
    if (days < 7) return `${days}d ago`;

    return d.toLocaleDateString('en-US', {
        month: 'short', day: 'numeric',
        hour: '2-digit', minute: '2-digit'
    });
}

function formatDuration(secs) {
    if (!secs && secs !== 0) return '—';
    if (secs < 60) return `${secs}s`;
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m}m ${s}s`;
}

function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function showError() {
    const tbody = document.getElementById('controls-tbody');
    if (tbody) {
        tbody.innerHTML = '<tr><td colspan="5" class="loading-cell">Failed to load status data. Will retry automatically.</td></tr>';
    }
}
