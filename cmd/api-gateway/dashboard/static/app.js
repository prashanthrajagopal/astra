/* global Chart */

(function checkAuth() {
  if (!localStorage.getItem('astra_token') && window.location.pathname.indexOf('/superadmin/dashboard') === 0) {
    window.location.href = '/login';
  }
})();

function setStatus(msg, isError) {
  const status = document.getElementById('refresh-status');
  status.textContent = msg;
  document.getElementById('dashboard-meta').classList.toggle('has-error', !!isError);
}

function emptyRow(cols, text) {
  const tr = document.createElement('tr');
  tr.className = 'empty-row';
  const td = document.createElement('td');
  td.colSpan = cols;
  td.className = 'empty-message';
  td.textContent = text;
  tr.appendChild(td);
  return tr;
}

function pick(obj, keys, fallback) {
  if (fallback === undefined) fallback = '';
  for (const k of keys) {
    if (obj && obj[k] !== undefined && obj[k] !== null) return obj[k];
  }
  return fallback;
}

function setText(id, val) {
  const el = document.getElementById(id);
  if (el) el.textContent = val;
}

function authFetch(url, opts) {
  opts = opts || {};
  opts.headers = opts.headers || {};
  var token = localStorage.getItem('astra_token');
  if (token) opts.headers['Authorization'] = 'Bearer ' + token;
  return fetch(url, opts).then(function(r) {
    if ((r.status === 401 || r.status === 403) && window.location.pathname.indexOf('/superadmin/dashboard') === 0) {
      localStorage.removeItem('astra_token');
      window.location.href = '/login';
    }
    return r;
  });
}

// ─── Summary cards ──────────────────────────────────────────────────

function renderSummary(data) {
  const goals = data.jobs && data.jobs.goals ? data.jobs.goals : {};
  const tasks = data.jobs && data.jobs.tasks ? data.jobs.tasks : {};
  const workers = data.workers || [];
  const approvals = data.approvals || [];

  setText('sum-active-goals', goals.active || 0);
  setText('sum-completed-tasks', tasks.completed || 0);
  setText('sum-running-tasks', tasks.running || 0);
  setText('sum-failed-goals', goals.failed || 0);
  var agentCount = data.agent_count != null ? data.agent_count : (Array.isArray(data.agents) ? data.agents.length : 0);
  setText('sum-agents', agentCount);
  setText('agents-badge', agentCount);

  var costRows = (data.cost && data.cost.rows && Array.isArray(data.cost.rows)) ? data.cost.rows : [];
  var tokensIn = 0;
  var tokensOut = 0;
  costRows.forEach(function (r) {
    tokensIn += Number(r.tokens_in != null ? r.tokens_in : r.TokensIn) || 0;
    tokensOut += Number(r.tokens_out != null ? r.tokens_out : r.TokensOut) || 0;
  });
  setText('sum-tokens-in', tokensIn.toLocaleString());
  setText('sum-tokens-out', tokensOut.toLocaleString());

  const activeWorkers = workers.filter(function (w) {
    var s = pick(w, ['status', 'Status'], '').toString().toLowerCase();
    return s === 'active' || s === 'online';
  });
  setText('sum-workers', activeWorkers.length);
  setText('sum-approvals', approvals.length);
}

// ─── Charts ─────────────────────────────────────────────────────────

var taskChart = null;
var goalChart = null;
var serviceChart = null;
var agentChart = null;
var lastSnapshotAgents = [];

/* Kapsicum-inspired palette — teal primary, calm contrast */
var chartColors = {
  created: '#22D3EE',
  pending: '#38BDF8',
  queued: '#2DD4BF',
  scheduled: '#34D399',
  running: '#FBBF24',
  completed: '#34D399',
  failed: '#F87171'
};

function renderTaskChart(tasks) {
  var ctx = document.getElementById('chart-tasks');
  if (!ctx) return;
  var labels = ['created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed'];
  var values = labels.map(function (l) { return tasks[l] || 0; });
  var colors = labels.map(function (l) { return chartColors[l] || '#2DD4BF'; });

  if (taskChart) {
    taskChart.data.datasets[0].data = values;
    taskChart.update();
    return;
  }
  taskChart = new Chart(ctx, {
    type: 'doughnut',
    data: {
      labels: labels.map(function (l) { return l.charAt(0).toUpperCase() + l.slice(1); }),
      datasets: [{ data: values, backgroundColor: colors, borderWidth: 0 }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { position: 'right', labels: { color: getChartTheme().legend, font: { size: 11, family: 'Roboto, sans-serif' } } }
      }
    }
  });
}

function renderGoalChart(goals) {
  var ctx = document.getElementById('chart-goals');
  if (!ctx) return;
  var labels = ['active', 'completed', 'failed', 'pending'];
  var values = labels.map(function (l) { return goals[l] || 0; });
  var colors = ['#FBBF24', '#34D399', '#F87171', '#2DD4BF'];

  if (goalChart) {
    goalChart.data.datasets[0].data = values;
    goalChart.update();
    return;
  }
  goalChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: labels.map(function (l) { return l.charAt(0).toUpperCase() + l.slice(1); }),
      datasets: [{ label: 'Goals', data: values, backgroundColor: colors, borderWidth: 0 }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      scales: {
        x: { ticks: { color: getChartTheme().tick, font: { family: 'Roboto, sans-serif' } }, grid: { color: getChartTheme().grid } },
        y: { beginAtZero: true, ticks: { color: getChartTheme().tick, stepSize: 1, font: { family: 'Roboto, sans-serif' } }, grid: { color: getChartTheme().grid } }
      },
      plugins: { legend: { display: false } }
    }
  });
}

function renderServiceChart(services) {
  var ctx = document.getElementById('chart-services');
  if (!ctx) return;
  if (!services || services.length === 0) return;
  var labels = services.map(function (s) { return s.name || ''; });
  var healthy = services.map(function (s) { return s.healthy ? 1 : 0; });
  var unhealthy = services.map(function (s) { return s.healthy ? 0 : 1; });

  if (serviceChart) {
    serviceChart.data.labels = labels;
    serviceChart.data.datasets[0].data = healthy;
    serviceChart.data.datasets[1].data = unhealthy;
    serviceChart.update();
    return;
  }
  serviceChart = new Chart(ctx, {
    type: 'bar',
    data: {
      labels: labels,
      datasets: [
        { label: 'Healthy', data: healthy, backgroundColor: '#34D399', borderWidth: 0 },
        { label: 'Unhealthy', data: unhealthy, backgroundColor: '#F87171', borderWidth: 0 }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      indexAxis: 'y',
      scales: {
        x: { stacked: true, max: 1, ticks: { display: false }, grid: { color: getChartTheme().grid } },
        y: { stacked: true, ticks: { color: getChartTheme().tick, font: { size: 10, family: 'Roboto, sans-serif' } }, grid: { display: false } }
      },
      plugins: { legend: { labels: { color: getChartTheme().legend, font: { size: 11, family: 'Roboto, sans-serif' } } } }
    }
  });
}

function renderAgentChart(agents) {
  var ctx = document.getElementById('chart-agents');
  if (!ctx) return;
  var list = Array.isArray(agents) ? agents : [];
  var byStatus = {};
  list.forEach(function (a) {
    var s = (pick(a, ['status', 'Status']) || 'unknown').toString().toLowerCase();
    byStatus[s] = (byStatus[s] || 0) + 1;
  });
  var labels = Object.keys(byStatus).length ? Object.keys(byStatus) : ['none'];
  var values = labels.map(function (l) { return byStatus[l] || 0; });
  var colors = ['#34D399', '#FBBF24', '#38BDF8', '#2DD4BF', '#F87171'];
  labels.forEach(function (_, i) {
    if (!colors[i]) colors[i] = '#2DD4BF';
  });

  if (agentChart) {
    agentChart.data.labels = labels.map(function (l) { return l.charAt(0).toUpperCase() + l.slice(1); });
    agentChart.data.datasets[0].data = values;
    agentChart.data.datasets[0].backgroundColor = values.map(function (_, i) { return colors[i % colors.length]; });
    agentChart.update();
    return;
  }
  agentChart = new Chart(ctx, {
    type: 'doughnut',
    data: {
      labels: labels.map(function (l) { return l.charAt(0).toUpperCase() + l.slice(1); }),
      datasets: [{ data: values, backgroundColor: values.map(function (_, i) { return colors[i % colors.length]; }), borderWidth: 0 }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: { position: 'right', labels: { color: getChartTheme().legend, font: { size: 11, family: 'Roboto, sans-serif' } } }
      }
    }
  });
}

function getChartTheme() {
  var light = document.documentElement.getAttribute('data-theme') === 'light';
  return {
    legend: light ? '#57534E' : '#A8A29E',
    tick: light ? '#78716C' : '#78716C',
    grid: light ? 'rgba(0,0,0,0.06)' : 'rgba(255,255,255,0.08)'
  };
}

function syncDashboardChartsTheme() {
  var t = getChartTheme();
  if (taskChart && taskChart.options && taskChart.options.plugins && taskChart.options.plugins.legend) {
    taskChart.options.plugins.legend.labels.color = t.legend;
    taskChart.update('none');
  }
  if (goalChart && goalChart.options && goalChart.options.scales) {
    var x = goalChart.options.scales.x;
    var y = goalChart.options.scales.y;
    if (x && x.ticks) x.ticks.color = t.tick;
    if (x && x.grid) x.grid.color = t.grid;
    if (y && y.ticks) y.ticks.color = t.tick;
    if (y && y.grid) y.grid.color = t.grid;
    goalChart.update('none');
  }
  if (serviceChart && serviceChart.options && serviceChart.options.scales) {
    var sx = serviceChart.options.scales.x;
    var sy = serviceChart.options.scales.y;
    if (sx && sx.grid) sx.grid.color = t.grid;
    if (sy && sy.ticks) sy.ticks.color = t.tick;
    if (serviceChart.options.plugins && serviceChart.options.plugins.legend) serviceChart.options.plugins.legend.labels.color = t.legend;
    serviceChart.update('none');
  }
  if (agentChart && agentChart.options && agentChart.options.plugins && agentChart.options.plugins.legend) {
    agentChart.options.plugins.legend.labels.color = t.legend;
    agentChart.update('none');
  }
}

// ─── Sidebar widgets ─────────────────────────────────────────────────

var healthDonutChart = null;

function renderHealthSummary(services) {
  var el = document.getElementById('health-summary-text');
  if (!el) return;
  if (!services || services.length === 0) {
    el.textContent = 'No services';
    return;
  }
  var healthy = services.filter(function (s) { return s.healthy; }).length;
  var unhealthy = services.length - healthy;
  el.textContent = healthy + ' healthy / ' + unhealthy + ' unhealthy';

  var canvas = document.getElementById('chart-health-donut');
  if (!canvas) return;
  if (healthDonutChart) {
    healthDonutChart.data.datasets[0].data = [healthy, unhealthy];
    healthDonutChart.update();
    return;
  }
  healthDonutChart = new Chart(canvas, {
    type: 'doughnut',
    data: {
      labels: ['Healthy', 'Unhealthy'],
      datasets: [{
        data: [healthy, unhealthy],
        backgroundColor: ['#34D399', '#F87171'],
        borderWidth: 0
      }]
    },
    options: {
      responsive: true,
      maintainAspectRatio: true,
      plugins: { legend: { display: false } }
    }
  });
}

function renderTaskQueueSummary(tasks) {
  var waiting = (tasks.pending || 0) + (tasks.queued || 0) + (tasks.scheduled || 0);
  var running = tasks.running || 0;
  setText('task-queue-waiting', waiting);
  setText('task-queue-running', running);
}

function renderCostSummary(cost) {
  var el = document.getElementById('cost-summary-total');
  if (!el) return;
  var rows = (cost && cost.rows && Array.isArray(cost.rows)) ? cost.rows : [];
  var total = 0;
  rows.forEach(function (r) {
    var v = parseFloat(r.cost_dollars);
    if (!isNaN(v)) total += v;
  });
  el.textContent = (document.body.classList.contains('dashboard-redesign') ? '$' : 'Total: $') + total.toFixed(2);
}

function renderWorkerUtilization(workers, tasks) {
  var el = document.getElementById('worker-util-summary');
  if (!el) return;
  var active = (workers || []).filter(function (w) {
    var s = pick(w, ['status', 'Status'], '').toString().toLowerCase();
    return s === 'active' || s === 'online';
  }).length;
  var running = (tasks && tasks.running) ? tasks.running : 0;
  el.textContent = active + ' active workers, ' + running + ' tasks running';
}

function renderApprovalsSummary(approvals) {
  var textEl = document.getElementById('approvals-summary-text');
  var listEl = document.getElementById('approvals-summary-list');
  if (!textEl) return;
  var list = approvals || [];
  if (list.length === 0) {
    textEl.textContent = 'No pending approvals';
    if (listEl) listEl.innerHTML = '';
    return;
  }
  textEl.textContent = list.length + ' pending';
  if (listEl) {
    listEl.innerHTML = '';
    list.slice(0, 3).forEach(function (a) {
      var li = document.createElement('li');
      li.textContent = (a.tool_name || a.id || '—').toString();
      listEl.appendChild(li);
    });
  }
}

// ─── Tables ─────────────────────────────────────────────────────────

var agentsList = [];
var agentsPage = 1;
var AGENTS_PAGE_SIZE = 10;

function renderAgentsPage() {
  var tbody = document.getElementById('tbody-agents');
  var prevBtn = document.getElementById('agents-prev');
  var nextBtn = document.getElementById('agents-next');
  var infoEl = document.getElementById('agents-page-info');
  if (!tbody) return;
  var total = agentsList.length;
  var totalPages = Math.max(1, Math.ceil(total / AGENTS_PAGE_SIZE));
  var page = Math.min(Math.max(1, agentsPage), totalPages);
  agentsPage = page;
  var start = (page - 1) * AGENTS_PAGE_SIZE;
  var slice = agentsList.slice(start, start + AGENTS_PAGE_SIZE);

  tbody.innerHTML = '';
  if (slice.length === 0) {
    tbody.appendChild(emptyRow(5, total === 0 ? 'No agents' : 'No agents on this page'));
  } else {
    slice.forEach(function (a) {
      var tr = document.createElement('tr');
      var id = pick(a, ['id', 'ID'], '');
      var name = pick(a, ['name', 'Name'], '') || pick(a, ['actor_type'], '');
      var actorType = pick(a, ['actor_type', 'actor_type'], '') || '';
      var status = (pick(a, ['status', 'Status'], '') || '').toLowerCase();
      var statusClass = status === 'active' ? 'active' : (status || '');
      var statusPill = '<span class="status ' + statusClass + '"><span class="status-dot"></span>' + (pick(a, ['status', 'Status'], '') || '—') + '</span>';
      var actionsHtml = '<td style="text-align:right">' +
        '<div class="actions">' +
        '<button type="button" class="action-btn agent-action-btn agent-action-edit" data-agent-id="' + escapeHtml(id || '') + '" data-action="edit" aria-label="Edit" title="Edit">✎</button>' +
        '<button type="button" class="action-btn agent-action-btn agent-action-enable" data-agent-id="' + escapeHtml(id || '') + '" data-action="enable" aria-label="Enable" title="Enable">▶</button>' +
        '<button type="button" class="action-btn agent-action-btn agent-action-disable" data-agent-id="' + escapeHtml(id || '') + '" data-action="disable" aria-label="Disable" title="Disable">⏸</button>' +
        '<button type="button" class="action-btn agent-action-btn agent-action-delete" data-agent-id="' + escapeHtml(id || '') + '" data-action="delete" aria-label="Delete" title="Delete">✕</button>' +
        '</div></td>';
      tr.innerHTML = '<td class="mono">' + (id ? id.substring(0, 8) : '') + '</td><td style="color:var(--text-primary);font-weight:500">' + escapeHtml(name || '—') + '</td><td class="mono">' + escapeHtml(actorType || '—') + '</td><td>' + statusPill + '</td>' + actionsHtml;
      tbody.appendChild(tr);
    });
  }
  if (prevBtn) prevBtn.disabled = page <= 1;
  if (nextBtn) nextBtn.disabled = page >= totalPages;
  if (infoEl) infoEl.textContent = 'Page ' + page + ' of ' + totalPages + (total ? ' · ' + total + ' agents' : '');
}

function renderAgents(agents) {
  var prevPage = agentsPage;
  agentsList = Array.isArray(agents) ? agents : [];
  var totalPages = Math.max(1, Math.ceil(agentsList.length / AGENTS_PAGE_SIZE));
  /* Keep current page across auto-refresh; clamp if list shrank */
  agentsPage = Math.min(Math.max(1, prevPage), totalPages);
  renderAgentsPage();
}

function renderRecentGoals(recentGoals) {
  var tbody = document.getElementById('tbody-goals');
  tbody.innerHTML = '';
  if (!recentGoals || recentGoals.length === 0) return tbody.appendChild(emptyRow(6, 'No goals yet'));
  recentGoals.forEach(function (g) {
    var tr = document.createElement('tr');
    tr.setAttribute('data-goal-id', g.id || '');
    var st = (g.status || '').toLowerCase();
    var isCancellable = ['pending', 'active', 'running', 'queued', 'scheduled', 'created'].indexOf(st) !== -1;
    var statusPill = '<span class="status ' + st + '"><span class="status-dot"></span>' + (g.status || '') + '</span>';
    var actionsHtml = isCancellable
      ? '<td style="text-align:right"><div class="actions"><button type="button" class="action-btn cancel-goal-btn" data-goal-id="' + escapeHtml(g.id || '') + '" title="Cancel goal">✕</button></div></td>'
      : '<td style="text-align:right"></td>';
    tr.innerHTML = '<td class="mono">' + (g.id || '').substring(0, 8) + '</td>' +
      '<td style="color:var(--text-primary);font-weight:500">' + escapeHtml((g.agent_id || '').substring(0, 12)) + '</td>' +
      '<td class="goal-text-cell" title="' + (g.goal_text || '').replace(/"/g, '&quot;') + '">' + escapeHtml(g.goal_text || '') + '</td>' +
      '<td>' + statusPill + '</td>' +
      '<td class="mono" style="font-size:11px">' + (g.created_at || '') + '</td>' +
      actionsHtml;
    tbody.appendChild(tr);
  });
}

function renderServices(services) {
  var tbody = document.getElementById('tbody-services');
  var badgeEl = document.getElementById('services-badge');
  tbody.innerHTML = '';
  if (!services || services.length === 0) {
    if (badgeEl) badgeEl.textContent = '—';
    return tbody.appendChild(emptyRow(5, 'No services configured'));
  }
  var healthy = services.filter(function (s) { return s.healthy; }).length;
  if (badgeEl) badgeEl.textContent = healthy === services.length ? 'All Healthy' : healthy + '/' + services.length;
  services.forEach(function (s) {
    var tr = document.createElement('tr');
    tr.className = 'tr-service';
    var latencyClass = Number(s.latency_ms) > 10 ? 'latency-warning' : '';
    var statusPill = '<span class="status ' + (s.healthy ? 'healthy' : 'failed') + '"><span class="status-dot"></span>' + (s.healthy ? 'Healthy' : 'Unhealthy') + '</span>';
    tr.innerHTML = '<td style="color:var(--text-primary);font-weight:500">' + escapeHtml(s.name || '') + '</td><td class="mono">' + (s.port || '—') + '</td><td>' + (s.type || '') + '</td>' +
      '<td>' + statusPill + '</td>' +
      '<td class="' + latencyClass + '">' + (s.latency_ms != null ? s.latency_ms : '') + '</td>';
    tbody.appendChild(tr);
  });
}

function renderWorkers(workers) {
  var tbody = document.getElementById('tbody-workers');
  var badgeEl = document.getElementById('workers-badge');
  tbody.innerHTML = '';
  if (!workers || workers.length === 0) {
    if (badgeEl) badgeEl.textContent = '0';
    return tbody.appendChild(emptyRow(5, 'No workers registered'));
  }
  var activeCount = workers.filter(function (w) {
    var s = pick(w, ['status', 'Status'], '').toString().toLowerCase();
    return s === 'active' || s === 'online';
  }).length;
  if (badgeEl) badgeEl.textContent = String(activeCount);
  workers.forEach(function (w) {
    var tr = document.createElement('tr');
    var id = pick(w, ['id', 'ID']);
    var hostname = pick(w, ['hostname', 'Hostname']);
    var statusText = pick(w, ['status', 'Status']);
    var capabilities = pick(w, ['capabilities', 'Capabilities'], []);
    var lastHeartbeat = pick(w, ['last_heartbeat', 'LastHeartbeat', 'lastHeartbeat']);
    var status = statusText.toString().toLowerCase();
    var statusPill = '<span class="status ' + (status === 'active' ? 'active' : '') + '"><span class="status-dot"></span>' + statusText + '</span>';
    tr.innerHTML = '<td class="mono">' + (id ? id.substring(0, 13) + '…' : '') + '</td><td style="color:var(--text-primary);font-weight:500">' + escapeHtml(hostname || '') + '</td><td>' + statusPill + '</td>' +
      '<td class="mono" style="font-size:11px">' + (Array.isArray(capabilities) ? capabilities.join(',') : '') + '</td><td class="mono" style="font-size:11px">' + (lastHeartbeat || '') + '</td>';
    tbody.appendChild(tr);
  });
}

function renderApprovals(items) {
  var tbody = document.getElementById('tbody-approvals');
  tbody.innerHTML = '';
  if (!items || items.length === 0) return tbody.appendChild(emptyRow(7, 'No pending approvals'));
  items.forEach(function (a) {
    var tr = document.createElement('tr');
    tr.className = 'approval-row';
    tr.setAttribute('data-approval-id', a.id || '');
    var reqType = (a.request_type || 'risky_task').toLowerCase();
    var typeLabel = reqType === 'plan' ? 'Plan' : 'Risky task';
    var toolOrSummary = reqType === 'plan' ? (a.summary || '—') : (a.tool_name || '');
    var st = (a.status || 'pending').toString().toLowerCase();
    var statusPill = '<span class="status ' + st + '"><span class="status-dot"></span>' + (a.status || '') + '</span>';
    tr.innerHTML = '<td class="td-type">' + typeLabel + '</td><td class="mono">' + (a.id ? a.id.substring(0, 8) : '') + '</td><td class="tool-summary-cell">' + escapeHtml((toolOrSummary || '').toString().substring(0, 80)) + (toolOrSummary && toolOrSummary.length > 80 ? '…' : '') + '</td><td>' + escapeHtml((a.action_summary || '').toString().substring(0, 60)) + '</td>' +
      '<td>' + statusPill + '</td><td class="mono" style="font-size:11px">' + (a.requested_at || '') + '</td>' +
      '<td><div class="actions">' +
      '<button type="button" class="action-btn approval-icon-btn approval-action-view view" data-id="' + (a.id || '') + '" title="View" aria-label="View">\uD83D\uDC41</button>' +
      '<button type="button" class="action-btn approval-icon-btn approval-action-approve" data-action="approve" data-id="' + (a.id || '') + '" title="Approve" aria-label="Approve">\u2713</button>' +
      '<button type="button" class="action-btn approval-icon-btn approval-action-reject" data-action="reject" data-id="' + (a.id || '') + '" title="Reject" aria-label="Reject">\u2715</button></div></td>';
    tbody.appendChild(tr);
  });
}

function renderCost(cost) {
  var tbody = document.getElementById('tbody-cost');
  tbody.innerHTML = '';
  var rows = (cost && Array.isArray(cost.rows)) ? cost.rows : [];
  if (rows.length === 0) return tbody.appendChild(emptyRow(6, 'No cost data'));
  rows.forEach(function (r) {
    var tr = document.createElement('tr');
    var costCell = r.cost_dollars != null ? r.cost_dollars : '';
    tr.innerHTML = '<td class="mono">' + (r.day || '') + '</td><td style="color:var(--text-primary);font-weight:500">' + escapeHtml(r.agent_id || '') + '</td><td class="mono">' + (r.model || '') + '</td>' +
      '<td class="mono">' + (r.tokens_in != null ? Number(r.tokens_in).toLocaleString() : '') + '</td><td class="mono">' + (r.tokens_out != null ? Number(r.tokens_out).toLocaleString() : '') + '</td>' +
      '<td class="mono cell-green" style="text-align:right">' + costCell + '</td>';
    tbody.appendChild(tr);
  });
}

function renderLogs(logs) {
  var container = document.getElementById('logs-container');
  container.innerHTML = '';
  var names = logs ? Object.keys(logs) : [];
  if (names.length === 0) {
    var empty = document.createElement('div');
    empty.className = 'empty-state';
    empty.textContent = 'No logs available';
    container.appendChild(empty);
    return;
  }
  names.sort().forEach(function (name) {
    var lines = Array.isArray(logs[name]) ? logs[name] : [];
    var group = document.createElement('div');
    group.className = 'log-group';
    var header = document.createElement('div');
    header.className = 'log-header';
    header.innerHTML = '<span class="chevron">▸</span><span class="log-service-dot"></span>' + escapeHtml(name) + '<span class="log-count">last ' + lines.length + ' lines</span>';
    var body = document.createElement('div');
    body.className = 'log-body';
    body.textContent = lines.length > 0 ? lines.join('\n') : 'No logs available';
    group.appendChild(header);
    group.appendChild(body);
    container.appendChild(group);
  });
}

function renderPids(pids) {
  var tbody = document.getElementById('tbody-pids');
  tbody.innerHTML = '';
  var names = pids ? Object.keys(pids) : [];
  if (names.length === 0) return tbody.appendChild(emptyRow(2, 'No PID data'));
  names.sort().forEach(function (name) {
    var tr = document.createElement('tr');
    tr.innerHTML = '<td style="color:var(--text-primary);font-weight:500">' + escapeHtml(name) + '</td><td class="mono" style="text-align:right">' + (pids[name] != null ? pids[name] : '') + '</td>';
    tbody.appendChild(tr);
  });
}

// ─── Fetching / actions ─────────────────────────────────────────────

var inFlight = false;
var approvalActionInFlight = false;

function submitApprovalAction(id, action) {
  if (!id || approvalActionInFlight) return;
  approvalActionInFlight = true;
  setStatus('Submitting ' + action + ' for ' + id, false);
  authFetch('/superadmin/api/dashboard/approvals/' + encodeURIComponent(id) + '/' + action, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ decided_by: 'dashboard-ui' })
  }).then(function (res) {
    if (!res.ok) throw new Error('status ' + res.status);
    return fetchSnapshot();
  }).catch(function (e) {
    setStatus('Error: ' + (e.message || e), true);
  }).finally(function () {
    approvalActionInFlight = false;
  });
}

function fetchSnapshot() {
  if (inFlight) return;
  inFlight = true;
  setStatus('Refreshing', false);
  return authFetch('/superadmin/api/dashboard/snapshot', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderSummary(d);

      var tasks = d.jobs && d.jobs.tasks ? d.jobs.tasks : {};
      var goals = d.jobs && d.jobs.goals ? d.jobs.goals : {};
      var recentGoals = d.jobs && d.jobs.recent_goals ? d.jobs.recent_goals : [];

      lastSnapshotAgents = d.agents || [];
      renderTaskChart(tasks);
      renderGoalChart(goals);
      renderServiceChart(d.services || []);
      renderAgentChart(d.agents || []);
      renderRecentGoals(recentGoals);
      renderAgents(d.agents || []);
    renderServices(d.services || []);
    renderWorkers(d.workers || []);
    renderApprovals(d.approvals || []);
    renderCost(d.cost || { rows: [] });
    renderLogs(d.logs || {});
    renderPids(d.pids || {});

      renderHealthSummary(d.services || []);
      renderTaskQueueSummary(d.jobs && d.jobs.tasks ? d.jobs.tasks : {});
      renderCostSummary(d.cost || { rows: [] });
      renderWorkerUtilization(d.workers || [], d.jobs && d.jobs.tasks ? d.jobs.tasks : {});
      renderApprovalsSummary(d.approvals || []);
      fetchChatSessions();

      document.getElementById('last-updated').textContent = 'Last updated: ' + new Date().toISOString();
    setStatus('Idle', false);
      var ban = document.getElementById('llm-sat-banner');
      if (ban) {
        authFetch('/superadmin/api/dashboard/llm-saturation', { cache: 'no-store' }).then(function (r) { return r.ok ? r.json() : null; }).then(function (s) {
          if (s && s.saturated) ban.hidden = false; else ban.hidden = true;
        }).catch(function () { if (ban) ban.hidden = true; });
      }
    })
    .catch(function (e) {
      setStatus('Error: ' + (e.message || e), true);
    })
    .finally(function () {
    inFlight = false;
    });
}

// ─── Goal detail modal ─────────────────────────────────────────────

function openGoalModal(goalId) {
  var modal = document.getElementById('goal-modal');
  var body = document.getElementById('goal-modal-body');
  if (!modal || !body) return;
  modal.dataset.goalId = goalId;
  modal.hidden = false;
  body.innerHTML = '<p class="goal-modal-loading">Loading…</p>';
  authFetch('/superadmin/api/dashboard/goals/' + encodeURIComponent(goalId), { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      body.innerHTML = renderGoalModalContent(d);
    })
    .catch(function (e) {
      body.innerHTML = '<p class="goal-modal-loading">Error: ' + (e.message || e) + '</p>';
    });
}

function renderGoalModalContent(d) {
  var goal = d.goal || {};
  var tasks = Array.isArray(d.tasks) ? d.tasks : [];
  var html = '';
  html += '<div class="goal-detail-meta">ID: ' + escapeHtml(goal.id || '') + ' &middot; Agent: ' + escapeHtml(goal.agent_id || '') + ' &middot; Status: ' + escapeHtml(goal.status || '') + ' &middot; Created: ' + escapeHtml(goal.created_at || '') + '</div>';
  html += '<div class="goal-detail-text">' + escapeHtml(goal.goal_text || '') + '</div>';
  html += '<div class="goal-detail-tasks-title">Actions (' + tasks.length + ')</div>';
  if (tasks.length === 0) {
    html += '<p class="goal-modal-loading">No tasks for this goal.</p>';
  } else {
    tasks.forEach(function (t) {
      var statusClass = (t.status || '').toLowerCase() === 'failed' ? ' status-failed' : '';
      var isCodeGen = (t.type || '').toLowerCase() === 'code_generate';
      var clickableClass = isCodeGen ? ' goal-detail-task-clickable' : '';
      var resultAttr = isCodeGen && t.result ? " data-task-result='" + String(JSON.stringify(t.result)).replace(/'/g, '&#39;') + "'" : '';
      var isCancellable = ['pending', 'queued', 'scheduled', 'running', 'created'].indexOf((t.status || '').toLowerCase()) !== -1;
      html += '<div class="goal-detail-task' + statusClass + clickableClass + '"' + resultAttr + ' data-task-type="' + escapeHtml(t.type || '') + '">';
      html += '<strong>' + escapeHtml(t.type || 'task') + '</strong> &middot; ' + escapeHtml(t.status || '') + ' (updated: ' + escapeHtml(t.updated_at || '') + ')';
      if (isCodeGen) html += ' <span class="goal-detail-task-hint">— click to view code</span>';
      if (isCancellable) html += ' <button class="cancel-task-btn" data-task-id="' + escapeHtml(t.id || '') + '" title="Cancel task">✕</button>';
      if ((t.status || '').toLowerCase() === 'failed' && t.result) {
        var resultStr = typeof t.result === 'string' ? t.result : (typeof t.result === 'object' ? JSON.stringify(t.result, null, 2) : String(t.result));
        html += '<div class="goal-detail-task-failure">' + escapeHtml(resultStr) + '</div>';
      }
      html += '</div>';
    });
  }
  return html;
}

function openCodeModal(taskResult) {
  var modal = document.getElementById('code-modal');
  var body = document.getElementById('code-modal-body');
  if (!modal || !body) return;
  var result = taskResult || {};
  var generated = result.generated_files || [];
  var filesWritten = result.files_written || [];
  var html = '';
  if (generated.length > 0) {
    generated.forEach(function (f) {
      var path = (f.path != null) ? f.path : (f.Path != null ? f.Path : '');
      var content = (f.content != null) ? f.content : (f.Content != null ? f.Content : '');
      html += '<div class="code-modal-file">';
      html += '<div class="code-modal-file-path">' + escapeHtml(path) + '</div>';
      html += '<pre class="code-modal-file-content"><code>' + escapeHtml(content) + '</code></pre>';
      html += '</div>';
    });
  } else if (filesWritten.length > 0) {
    html += '<p class="code-modal-no-content">Files written: ' + escapeHtml(filesWritten.join(', ')) + '</p>';
    html += '<p class="code-modal-no-content-hint">Generated code is not stored for this task. Newer runs will show code here.</p>';
  } else {
    html += '<p class="code-modal-no-content">No generated files for this task.</p>';
  }
  body.innerHTML = html;
  modal.hidden = false;
}

function closeCodeModal() {
  var modal = document.getElementById('code-modal');
  if (modal) modal.hidden = true;
}

function escapeHtml(s) {
  if (s == null) return '';
  var div = document.createElement('div');
  div.textContent = s;
  return div.innerHTML;
}

function closeGoalModal() {
  var modal = document.getElementById('goal-modal');
  if (modal) modal.hidden = true;
}

// ─── Approval detail modal ───────────────────────────────────────────────

var currentApprovalId = null;

function openApprovalModal(approvalId) {
  var modal = document.getElementById('approval-modal');
  var body = document.getElementById('approval-modal-body');
  if (!modal || !body) return;
  currentApprovalId = approvalId;
  modal.hidden = false;
  body.innerHTML = '<p class="approval-modal-loading">Loading…</p>';
  authFetch('/superadmin/api/dashboard/approvals/' + encodeURIComponent(approvalId), { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      body.innerHTML = renderApprovalModalContent(d);
    })
    .catch(function (e) {
      body.innerHTML = '<p class="approval-modal-loading">Error: ' + escapeHtml(e.message || e) + '</p>';
    });
}

function renderApprovalModalContent(d) {
  var reqType = (d.request_type || 'risky_task').toLowerCase();
  var html = '';
  if (reqType === 'risky_task') {
    html += '<div class="approval-detail-meta">';
    html += '<p><strong>Tool:</strong> ' + escapeHtml(d.tool_name || '') + '</p>';
    html += '<p><strong>Action summary:</strong> ' + escapeHtml(d.action_summary || '') + '</p>';
    html += '<p><strong>Task ID:</strong> ' + escapeHtml(d.task_id || '—') + '</p>';
    html += '<p><strong>Worker ID:</strong> ' + escapeHtml(d.worker_id || '—') + '</p>';
    html += '<p><strong>Requested at:</strong> ' + escapeHtml(d.requested_at || '') + '</p>';
    html += '</div>';
  } else {
    var payload = d.plan_payload || {};
    html += '<div class="approval-detail-meta">';
    html += '<p><strong>Goal ID:</strong> ' + escapeHtml(payload.goal_id || d.goal_id || '') + '</p>';
    html += '<p><strong>Graph ID:</strong> ' + escapeHtml(payload.graph_id || d.graph_id || '') + '</p>';
    html += '<p><strong>Goal text:</strong></p><div class="approval-detail-goal-text">' + escapeHtml(payload.goal_text || '') + '</div>';
    var tasks = payload.tasks || [];
    html += '<p><strong>Tasks (' + tasks.length + '):</strong></p><ul class="approval-detail-tasks">';
    tasks.forEach(function (t) {
      var desc = (t.payload && t.payload.description) ? t.payload.description : (t.type || 'task');
      html += '<li><strong>' + escapeHtml(t.type || 'task') + '</strong>: ' + escapeHtml(desc) + '</li>';
    });
    html += '</ul></div>';
  }
  return html;
}

function closeApprovalModal() {
  var modal = document.getElementById('approval-modal');
  if (modal) modal.hidden = true;
  currentApprovalId = null;
}

function approvalDialogApprove() {
  if (!currentApprovalId) return;
  var id = currentApprovalId;
  closeApprovalModal();
  submitApprovalAction(id, 'approve');
}

function approvalDialogReject() {
  if (!currentApprovalId) return;
  var id = currentApprovalId;
  closeApprovalModal();
  submitApprovalAction(id, 'reject');
}

// ─── Chat ─────────────────────────────────────────────────────────────

var currentChatSessionId = null;
var chatSessionsList = [];

function loadSettings() {
  authFetch('/superadmin/api/dashboard/settings', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) return;
      return res.json();
    })
    .then(function (d) {
      var toggle = document.getElementById('toggle-auto-approve-plans');
      var note = document.getElementById('auto-approve-note');
      if (toggle && d && typeof d.auto_approve_plans === 'boolean') {
        toggle.checked = d.auto_approve_plans;
        toggle.disabled = true;
        if (note) note.textContent = '(Read-only: set AUTO_APPROVE_PLANS in goal-service to change)';
      } else if (note) {
        note.textContent = '(AUTO_APPROVE_PLANS env in goal-service controls plan auto-approve)';
      }
    })
    .catch(function () {});
}

// ─── Chat agents ─────────────────────────────────────────────────────

var currentChatSessionId = null;
var chatSessionsList = [];

function fetchChatSessions() {
  authFetch('/superadmin/api/dashboard/chat/sessions', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      chatSessionsList = d.sessions || [];
      renderChatSessions();
    })
    .catch(function (e) {
      chatSessionsList = [];
      renderChatSessions();
    });
}

function renderChatSessions() {
  var listEl = document.getElementById('chat-sessions-list');
  if (!listEl) return;
  listEl.innerHTML = '';
  if (chatSessionsList.length === 0) {
    listEl.innerHTML = '<p class="chat-empty-hint">No sessions yet. Click "New chat" to start.</p>';
    return;
  }
  chatSessionsList.forEach(function (s) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'chat-session-btn' + (currentChatSessionId === s.id ? ' chat-session-btn-active' : '');
    btn.setAttribute('data-session-id', s.id);
    btn.setAttribute('data-agent-id', s.agent_id || '');
    btn.textContent = (s.title || 'Chat') + ' (' + (s.agent_id ? s.agent_id.substring(0, 8) : '') + ')';
    listEl.appendChild(btn);
  });
}

function openChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  var loadingEl = document.getElementById('chat-new-loading');
  var listEl = document.getElementById('chat-new-agent-list');
  var emptyEl = document.getElementById('chat-new-empty');
  if (!modal) return;
  modal.hidden = false;
  if (loadingEl) loadingEl.hidden = false;
  if (listEl) { listEl.hidden = true; listEl.innerHTML = ''; }
  if (emptyEl) emptyEl.hidden = true;
  authFetch('/superadmin/api/dashboard/chat/agents', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      var agents = d.agents || [];
      if (loadingEl) loadingEl.hidden = true;
      if (agents.length === 0) {
        if (emptyEl) emptyEl.hidden = false;
      } else {
        if (listEl) {
          listEl.hidden = false;
          agents.forEach(function (a) {
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'chat-agent-btn';
            btn.setAttribute('data-agent-id', a.id || '');
            btn.setAttribute('data-agent-name', a.name || '');
            btn.textContent = (a.name || a.id || 'Agent');
            listEl.appendChild(btn);
          });
        }
      }
    })
    .catch(function (e) {
      if (loadingEl) loadingEl.textContent = 'Error: ' + (e.message || e);
    });
}

function closeChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  if (modal) modal.hidden = true;
}

function createChatSession(agentId, agentName) {
  closeChatNewModal();
  authFetch('/superadmin/api/dashboard/chat/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agent_id: agentId, title: agentName || 'Chat' })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      fetchChatSessions();
      selectChatSession(d.id, d.agent_id);
    })
    .catch(function (e) {
      setStatus('Chat create failed: ' + (e.message || e), true);
    });
}

function selectChatSession(sessionId, agentId) {
  currentChatSessionId = sessionId;
  renderChatSessions();
  var placeholder = document.getElementById('chat-placeholder');
  var chatView = document.getElementById('chat-view');
  if (placeholder) placeholder.hidden = true;
  if (chatView) chatView.hidden = false;
  fetchChatMessages(sessionId);
}

function fetchChatMessages(sessionId) {
  var container = document.getElementById('chat-messages');
  if (!container) return;
  authFetch('/superadmin/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderChatMessages(d.messages || []);
    })
    .catch(function (e) {
      container.innerHTML = '<p class="chat-empty-hint">Error loading messages: ' + escapeHtml(e.message || e) + '</p>';
    });
}

function renderChatMessages(messages) {
  var container = document.getElementById('chat-messages');
  if (!container) return;
  container.innerHTML = '';
  messages.forEach(function (m) {
    var div = document.createElement('div');
    div.className = 'chat-msg chat-msg-' + (m.role || 'user');
    div.innerHTML = '<span class="chat-msg-role">' + escapeHtml(m.role || '') + '</span><pre class="chat-msg-content">' + escapeHtml(m.content || '') + '</pre>';
    container.appendChild(div);
  });
  container.scrollTop = container.scrollHeight;
}

function sendChatMessage() {
  var sessionId = currentChatSessionId;
  var input = document.getElementById('chat-input');
  if (!sessionId || !input) return;
  var content = (input.value || '').trim();
  if (!content) return;
  input.value = '';
  input.disabled = true;
  authFetch('/superadmin/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: content })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return fetchChatMessages(sessionId);
    })
    .catch(function (e) {
      setStatus('Send failed: ' + (e.message || e), true);
    })
    .finally(function () {
      if (input) input.disabled = false;
    });
}

// ─── Floating Chat Widget ───────────────────────────────────────────

var widgetSessionId = null;
var widgetAgentId = null;
var widgetOpen = false;

function initChatWidget() {
  authFetch('/superadmin/api/dashboard/chat/agents', { cache: 'no-store' })
    .then(function (res) { return res.ok ? res.json() : Promise.reject('no agents'); })
    .then(function (d) {
      var agents = d.agents || [];
      var chatAgent = agents.find(function (a) {
        return (a.name || '').toLowerCase().indexOf('chat assistant') !== -1;
      });
      if (!chatAgent) {
        var widget = document.getElementById('chat-widget');
        if (widget) widget.style.display = 'none';
        return;
      }
      widgetAgentId = chatAgent.id;

      var savedSessionId = localStorage.getItem('astra_chat_widget_session');
      if (savedSessionId) {
        authFetch('/superadmin/api/dashboard/chat/sessions/' + encodeURIComponent(savedSessionId), { cache: 'no-store' })
          .then(function (res) { return res.ok ? res.json() : null; })
          .then(function (session) {
            if (session && session.id) {
              widgetSessionId = session.id;
              widgetSetStatus('Connected');
            } else {
              localStorage.removeItem('astra_chat_widget_session');
              widgetCreateSession();
            }
          })
          .catch(function () { widgetCreateSession(); });
      } else {
        widgetCreateSession();
      }
    })
    .catch(function () {
      var widget = document.getElementById('chat-widget');
      if (widget) widget.style.display = 'none';
    });
}

function widgetCreateSession() {
  if (!widgetAgentId) return;
  authFetch('/superadmin/api/dashboard/chat/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agent_id: widgetAgentId, title: 'Dashboard Chat' })
  })
    .then(function (res) { return res.ok ? res.json() : Promise.reject('create failed'); })
    .then(function (d) {
      widgetSessionId = d.id;
      localStorage.setItem('astra_chat_widget_session', d.id);
      widgetSetStatus('Connected');
    })
    .catch(function () {
      widgetSetStatus('Offline');
    });
}

function widgetSetStatus(text) {
  var el = document.getElementById('chat-widget-status');
  if (el) el.textContent = text;
}

function widgetToggle() {
  widgetOpen = !widgetOpen;
  var panel = document.getElementById('chat-widget-panel');
  if (panel) panel.hidden = !widgetOpen;
  if (widgetOpen && widgetSessionId) {
    widgetLoadMessages();
    var badge = document.getElementById('chat-widget-badge');
    if (badge) badge.hidden = true;
  }
}

function widgetLoadMessages() {
  if (!widgetSessionId) return;
  var container = document.getElementById('chat-widget-messages');
  if (!container) return;
  authFetch('/superadmin/api/dashboard/chat/sessions/' + encodeURIComponent(widgetSessionId) + '/messages', { cache: 'no-store' })
    .then(function (res) { return res.ok ? res.json() : Promise.reject('load failed'); })
    .then(function (d) {
      widgetRenderMessages(d.messages || []);
    })
    .catch(function () {
      container.innerHTML = '<div class="chat-widget-msg chat-widget-msg-system">Could not load messages.</div>';
    });
}

function widgetRenderMessages(messages) {
  var container = document.getElementById('chat-widget-messages');
  if (!container) return;
  container.innerHTML = '';
  if (messages.length === 0) {
    container.innerHTML = '<div class="chat-widget-msg chat-widget-msg-system">Send a message to start chatting.</div>';
    return;
  }
  messages.forEach(function (m) {
    var div = document.createElement('div');
    div.className = 'chat-widget-msg chat-widget-msg-' + (m.role || 'user');
    div.textContent = m.content || '';
    container.appendChild(div);
  });
  container.scrollTop = container.scrollHeight;
}

function widgetSend() {
  if (!widgetSessionId) return;
  var input = document.getElementById('chat-widget-input');
  if (!input) return;
  var content = (input.value || '').trim();
  if (!content) return;
  input.value = '';

  var container = document.getElementById('chat-widget-messages');
  var system = container.querySelector('.chat-widget-msg-system');
  if (system && system.textContent.indexOf('Send a message') !== -1) system.remove();

  var userDiv = document.createElement('div');
  userDiv.className = 'chat-widget-msg chat-widget-msg-user';
  userDiv.textContent = content;
  container.appendChild(userDiv);
  container.scrollTop = container.scrollHeight;

  var typing = document.createElement('div');
  typing.className = 'chat-widget-typing';
  typing.textContent = 'Thinking...';
  container.appendChild(typing);
  container.scrollTop = container.scrollHeight;

  var sendBtn = document.getElementById('chat-widget-send');
  if (sendBtn) sendBtn.disabled = true;

  authFetch('/superadmin/api/dashboard/chat/sessions/' + encodeURIComponent(widgetSessionId) + '/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: content })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return widgetLoadMessages();
    })
    .catch(function (e) {
      typing.textContent = 'Failed: ' + (e.message || e);
      typing.className = 'chat-widget-msg chat-widget-msg-system';
    })
    .finally(function () {
      if (sendBtn) sendBtn.disabled = false;
      if (typing.parentNode) typing.remove();
    });
}

function applyDashboardTheme(theme) {
  var root = document.documentElement;
  var t = (theme || localStorage.getItem('astra_dashboard_theme') || 'dark');
  root.setAttribute('data-theme', t);
  var sunEl = document.querySelector('.theme-icon-sun');
  var moonEl = document.querySelector('.theme-icon-moon');
  if (sunEl) sunEl.hidden = t === 'light';
  if (moonEl) moonEl.hidden = t !== 'light';
  syncDashboardChartsTheme();
}

function toggleDashboardTheme() {
  var cur = document.documentElement.getAttribute('data-theme') || 'dark';
  var next = cur === 'dark' ? 'light' : 'dark';
  localStorage.setItem('astra_dashboard_theme', next);
  applyDashboardTheme(next);
}

document.addEventListener('DOMContentLoaded', function () {
  applyDashboardTheme();
  var btnTheme = document.getElementById('btn-theme-toggle');
  if (btnTheme) btnTheme.addEventListener('click', toggleDashboardTheme);
  document.getElementById('btn-refresh').addEventListener('click', fetchSnapshot);
  var tbodyApprovals = document.getElementById('tbody-approvals');
  if (tbodyApprovals) {
    tbodyApprovals.addEventListener('click', function (e) {
      var t = e.target;
      if (!(t instanceof HTMLElement)) return;
      if (t.classList && t.classList.contains('view') && t.dataset && t.dataset.id) {
        e.preventDefault();
        openApprovalModal(t.dataset.id);
        return;
      }
      var tr = t.closest && t.closest('tr.approval-row');
      if (tr && tr.dataset && tr.dataset.approvalId && (!t.dataset.action || !t.classList.contains('approve') && !t.classList.contains('reject'))) {
        openApprovalModal(tr.dataset.approvalId);
        return;
      }
      if (t.dataset && t.dataset.action && t.dataset.id) {
        submitApprovalAction(t.dataset.id, t.dataset.action);
      }
    });
  }
  var modal = document.getElementById('approval-modal');
  if (modal) {
    document.getElementById('approval-modal-close').addEventListener('click', closeApprovalModal);
    modal.querySelector('.approval-modal-backdrop').addEventListener('click', closeApprovalModal);
    document.getElementById('approval-dialog-approve').addEventListener('click', approvalDialogApprove);
    document.getElementById('approval-dialog-reject').addEventListener('click', approvalDialogReject);
  }
  loadSettings();
  var tbodyGoals = document.getElementById('tbody-goals');
  if (tbodyGoals) {
    tbodyGoals.addEventListener('click', function (e) {
      var cancelGoalBtn = e.target && e.target.closest && e.target.closest('.cancel-goal-btn');
      if (cancelGoalBtn && cancelGoalBtn.dataset && cancelGoalBtn.dataset.goalId) {
        e.stopPropagation();
        if (!confirm('Cancel this goal and all its tasks?')) return;
        authFetch('/superadmin/api/dashboard/goals/' + encodeURIComponent(cancelGoalBtn.dataset.goalId) + '/cancel', { method: 'POST' })
          .then(function (r) {
            if (r.ok) {
              fetchSnapshot();
            } else {
              r.text().then(function (t) { alert('Cancel failed: ' + t); });
            }
          });
        return;
      }
      var tr = e.target && e.target.closest && e.target.closest('tr[data-goal-id]');
      if (tr && tr.dataset && tr.dataset.goalId) openGoalModal(tr.dataset.goalId);
    });
  }
  var goalModal = document.getElementById('goal-modal');
  if (goalModal) {
    document.getElementById('goal-modal-close').addEventListener('click', closeGoalModal);
    goalModal.querySelector('.goal-modal-backdrop').addEventListener('click', closeGoalModal);
    goalModal.addEventListener('click', function (e) {
      var cancelBtn = e.target && e.target.closest && e.target.closest('.cancel-task-btn');
      if (cancelBtn && cancelBtn.dataset && cancelBtn.dataset.taskId) {
        e.stopPropagation();
        if (!confirm('Cancel this task?')) return;
        authFetch('/superadmin/api/dashboard/tasks/' + encodeURIComponent(cancelBtn.dataset.taskId) + '/cancel', { method: 'POST' })
          .then(function (r) {
            if (r.ok) {
              cancelBtn.disabled = true;
              cancelBtn.textContent = '✓';
              var goalId = document.getElementById('goal-modal').dataset.goalId;
              if (goalId) setTimeout(function () { openGoalModal(goalId); }, 500);
            } else {
              r.text().then(function (t) { alert('Cancel failed: ' + t); });
            }
          })
          .catch(function (err) { alert('Cancel failed: ' + (err.message || err)); });
        return;
      }
      var taskEl = e.target && e.target.closest && e.target.closest('.goal-detail-task-clickable[data-task-result]');
      if (taskEl && taskEl.getAttribute('data-task-result')) {
        try {
          var taskResult = JSON.parse(taskEl.getAttribute('data-task-result'));
          openCodeModal(taskResult);
        } catch (err) { /* ignore */ }
      }
    });
  }
  var codeModalClose = document.getElementById('code-modal-close');
  var codeModalBackdrop = document.getElementById('code-modal-backdrop');
  if (codeModalClose) codeModalClose.addEventListener('click', closeCodeModal);
  if (codeModalBackdrop) codeModalBackdrop.addEventListener('click', closeCodeModal);
  var chatNewModal = document.getElementById('chat-new-modal');
  if (chatNewModal) {
    document.getElementById('chat-new-modal-close')?.addEventListener('click', closeChatNewModal);
    document.getElementById('chat-new-modal-backdrop')?.addEventListener('click', closeChatNewModal);
  }
  document.getElementById('chat-new-btn')?.addEventListener('click', openChatNewModal);
  document.getElementById('chat-send-btn')?.addEventListener('click', sendChatMessage);
  var chatSessionsListEl = document.getElementById('chat-sessions-list');
  if (chatSessionsListEl) {
    chatSessionsListEl.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-session-btn');
      if (btn && btn.dataset && btn.dataset.sessionId) selectChatSession(btn.dataset.sessionId, btn.dataset.agentId);
    });
  }
  var chatAgentListEl = document.getElementById('chat-new-agent-list');
  if (chatAgentListEl) {
    chatAgentListEl.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-agent-btn');
      if (btn && btn.dataset && btn.dataset.agentId) createChatSession(btn.dataset.agentId, btn.dataset.agentName || '');
    });
  }
  var chatInput = document.getElementById('chat-input');
  if (chatInput) {
    chatInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChatMessage(); }
    });
  }
  // Chat widget event listeners
  document.getElementById('chat-widget-toggle')?.addEventListener('click', widgetToggle);
  document.getElementById('chat-widget-minimize')?.addEventListener('click', widgetToggle);
  document.getElementById('chat-widget-send')?.addEventListener('click', widgetSend);
  var widgetInput = document.getElementById('chat-widget-input');
  if (widgetInput) {
    widgetInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); widgetSend(); }
    });
  }
  initChatWidget();
  var agentsPrev = document.getElementById('agents-prev');
  var agentsNext = document.getElementById('agents-next');
  if (agentsPrev) agentsPrev.addEventListener('click', function () { agentsPage = Math.max(1, agentsPage - 1); renderAgentsPage(); });
  if (agentsNext) agentsNext.addEventListener('click', function () { agentsPage = Math.min(Math.ceil(agentsList.length / AGENTS_PAGE_SIZE) || 1, agentsPage + 1); renderAgentsPage(); });
  var tableAgents = document.getElementById('table-agents');
  if (tableAgents) {
    tableAgents.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.agent-action-btn');
      if (!btn || !btn.dataset || !btn.dataset.agentId) return;
      var agentId = btn.dataset.agentId;
      var action = (btn.dataset.action || '').toLowerCase();
      if (action === 'edit') {
        openEditAgentModal(agentId);
      } else if (action === 'enable') {
        authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentId) + '/status', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: 'active' }) })
          .then(function (r) { if (r.ok) fetchSnapshot(); else r.text().then(function (t) { setStatus('Agent enable failed: ' + t, true); }); });
      } else if (action === 'disable') {
        authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentId) + '/status', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: 'stopped' }) })
          .then(function (r) { if (r.ok) fetchSnapshot(); else r.text().then(function (t) { setStatus('Agent disable failed: ' + t, true); }); });
      } else if (action === 'delete') {
        if (!confirm('Delete this agent and all its goals and tasks?')) return;
        authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentId), { method: 'DELETE' })
          .then(function (r) { if (r.ok) fetchSnapshot(); else r.text().then(function (t) { setStatus('Agent delete failed: ' + t, true); }); });
      }
    });
  }
  fetchSnapshot();
  /* Auto-refresh dashboard data; 30s avoids fighting with agents pagination / UI focus */
  setInterval(fetchSnapshot, 30000);

  // ─── Tab navigation ────────────────────────────────────────────────
  document.querySelectorAll('.nav-tab').forEach(function(tab) {
    tab.addEventListener('click', function() {
      document.querySelectorAll('.nav-tab').forEach(function(t) { t.classList.remove('active'); });
      tab.classList.add('active');
      var target = tab.dataset.tab;
      document.querySelectorAll('.tab-panel').forEach(function(p) { p.hidden = true; });
      var panel = document.getElementById('tab-' + target);
      if (panel) panel.hidden = false;
      if (target === 'slack') loadSlackConfig();
    });
  });

  // ─── Slack config ───────────────────────────────────────
  function loadSlackConfig() {
    authFetch('/superadmin/api/slack/config')
      .then(function(r) { return r.json(); })
      .then(function(d) {
        document.getElementById('slack-signing-secret') && (document.getElementById('slack-signing-secret').value = '');
        document.getElementById('slack-client-id') && (document.getElementById('slack-client-id').value = d.client_id || '');
        document.getElementById('slack-client-secret') && (document.getElementById('slack-client-secret').value = '');
        document.getElementById('slack-oauth-redirect') && (document.getElementById('slack-oauth-redirect').value = d.oauth_redirect_url || '');
        var statusEl = document.getElementById('slack-config-status');
        if (statusEl) statusEl.textContent = (d.signing_secret === '********' || d.client_secret === '********') ? 'Configured (secrets hidden)' : '';
      }).catch(function() {});
  }
  var btnSaveSlackConfig = document.getElementById('btn-save-slack-config');
  if (btnSaveSlackConfig) btnSaveSlackConfig.addEventListener('click', function() {
    var payload = {};
    var v = document.getElementById('slack-signing-secret').value; if (v) payload.signing_secret = v;
    v = document.getElementById('slack-client-id').value; if (v) payload.client_id = v;
    v = document.getElementById('slack-client-secret').value; if (v) payload.client_secret = v;
    v = document.getElementById('slack-oauth-redirect').value; if (v) payload.oauth_redirect_url = v;
    var statusEl = document.getElementById('slack-config-status');
    if (Object.keys(payload).length === 0) { if (statusEl) statusEl.textContent = 'Enter at least one field.'; return; }
    btnSaveSlackConfig.disabled = true; if (statusEl) statusEl.textContent = 'Saving...';
    authFetch('/superadmin/api/slack/config', { method: 'PUT', headers: {'Content-Type':'application/json'}, body: JSON.stringify(payload) })
      .then(function(r) { return r.json().then(function(d) { return {ok:r.ok,data:d}; }); })
      .then(function(res) {
        btnSaveSlackConfig.disabled = false;
        if (statusEl) statusEl.textContent = res.ok ? 'Saved.' : (res.data.error || 'Save failed');
        if (res.ok) loadSlackConfig();
      })
      .catch(function() { btnSaveSlackConfig.disabled = false; if (statusEl) statusEl.textContent = 'Request failed'; });
  });

  // ─── Edit Agent Modal (data source & Slack) ───────────────────────────
  var agentEditModal = document.getElementById('agent-edit-modal');
  var agentEditId = null;

  function openEditAgentModal(agentId) {
    agentEditId = agentId;
    document.getElementById('agent-edit-name').value = 'Loading…';
    document.getElementById('agent-edit-actor-type').textContent = '';
    document.getElementById('agent-edit-config').value = '';
    document.getElementById('agent-edit-ingest-source-type').value = '';
    document.getElementById('agent-edit-ingest-source-config').value = '';
    document.getElementById('agent-edit-slack-notifications').checked = false;
    document.getElementById('agent-edit-chat-capable').checked = false;
    document.getElementById('agent-edit-prompt').value = '';
    document.getElementById('agent-edit-modal-error').hidden = true;
    updateIngestSourceHint('agent-edit');
    agentEditModal.hidden = false;
    var errEl = document.getElementById('agent-edit-modal-error');
    var saveBtn = document.getElementById('agent-edit-modal-save');
    if (saveBtn) saveBtn.disabled = true;
    authFetch('/agents/' + encodeURIComponent(agentId) + '/profile', { cache: 'no-store' })
      .then(function(r) {
        if (!r.ok) return r.text().then(function(t) { throw new Error(r.status === 404 ? 'Agent not found' : (t || 'Failed to load profile')); });
        return r.json();
      })
      .then(function(p) {
        document.getElementById('agent-edit-name').value = p.name || '';
        document.getElementById('agent-edit-actor-type').textContent = p.actor_type || '—';
        var configVal = p.config;
        document.getElementById('agent-edit-config').value = (typeof configVal === 'object' && configVal !== null) ? JSON.stringify(configVal) : (typeof configVal === 'string' ? configVal : (configVal ? JSON.stringify(configVal) : ''));
        document.getElementById('agent-edit-chat-capable').checked = !!p.chat_capable;
        document.getElementById('agent-edit-slack-notifications').checked = !!p.slack_notifications_enabled;
        document.getElementById('agent-edit-ingest-source-type').value = p.ingest_source_type || '';
        var cfg = p.ingest_source_config;
        document.getElementById('agent-edit-ingest-source-config').value = (typeof cfg === 'object' && cfg !== null) ? JSON.stringify(cfg) : (typeof cfg === 'string' ? cfg : '');
        document.getElementById('agent-edit-prompt').value = p.system_prompt || '';
        document.getElementById('agent-edit-drain').checked = !!p.drain_mode;
        document.getElementById('agent-edit-max-goals').value = p.max_concurrent_goals != null ? p.max_concurrent_goals : '';
        document.getElementById('agent-edit-budget').value = p.daily_token_budget != null ? p.daily_token_budget : '';
        document.getElementById('agent-edit-priority').value = p.priority != null ? p.priority : 0;
        updateIngestSourceHint('agent-edit');
        if (errEl) { errEl.hidden = true; errEl.textContent = ''; }
        if (saveBtn) saveBtn.disabled = false;
        var allowed = Array.isArray(p.allowed_tools) ? p.allowed_tools : [];
        authFetch('/superadmin/api/dashboard/tools', { cache: 'no-store' }).then(function (r) { return r.json(); }).then(function (td) {
          var wrap = document.getElementById('agent-edit-tools-wrap');
          if (!wrap || !td.tools) return;
          wrap.innerHTML = '';
          td.tools.forEach(function (t) {
            var id = 'tool-' + t.name + '-' + t.version;
            var key = t.name + '@' + t.version;
            var lab = document.createElement('label');
            var cb = document.createElement('input');
            cb.type = 'checkbox'; cb.id = id; cb.value = key;
            cb.checked = allowed.length === 0 || allowed.indexOf(key) >= 0 || allowed.indexOf(t.name + '@*') >= 0 || allowed.indexOf('*') >= 0;
            lab.appendChild(cb);
            lab.appendChild(document.createTextNode(' ' + t.name + '@' + t.version + ' (' + t.risk_tier + ')'));
            wrap.appendChild(lab);
          });
        }).catch(function () {});
        authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentId) + '/revisions', { cache: 'no-store' }).then(function (r) { return r.json(); }).then(function (rev) {
          var ul = document.getElementById('agent-edit-rev-list');
          if (!ul || !rev.revisions) return;
          ul.innerHTML = '';
          rev.revisions.forEach(function (x) {
            var li = document.createElement('li');
            li.textContent = 'rev ' + x.revision + ' — ' + (x.created_at || '');
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.textContent = 'Activate';
            btn.addEventListener('click', function () {
              authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentId) + '/revisions/' + x.revision + '/activate', { method: 'POST' }).then(function () { openEditAgentModal(agentId); });
            });
            li.appendChild(btn);
            ul.appendChild(li);
          });
        }).catch(function () {});
      })
      .catch(function(e) {
        if (errEl) { errEl.textContent = e && e.message ? e.message : 'Failed to load profile'; errEl.hidden = false; }
        document.getElementById('agent-edit-name').value = '';
        if (saveBtn) saveBtn.disabled = false;
      });
  }

  var editIngestTypeSel = document.getElementById('agent-edit-ingest-source-type');
  if (editIngestTypeSel) editIngestTypeSel.addEventListener('change', function() { updateIngestSourceHint('agent-edit'); });

  if (agentEditModal) {
    var btnPlat = document.getElementById('agent-edit-save-platform');
    if (btnPlat) btnPlat.addEventListener('click', function () {
      if (!agentEditId) return;
      var tools = [];
      document.querySelectorAll('#agent-edit-tools-wrap input[type=checkbox]:checked').forEach(function (c) { tools.push(c.value); });
      var body = {
        drain_mode: document.getElementById('agent-edit-drain').checked,
        max_concurrent_goals: parseInt(document.getElementById('agent-edit-max-goals').value, 10) || null,
        daily_token_budget: parseInt(document.getElementById('agent-edit-budget').value, 10) || null,
        priority: parseInt(document.getElementById('agent-edit-priority').value, 10) || 0,
        allowed_tools: tools.length ? tools : null
      };
      authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentEditId) + '/platform', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
        .then(function (r) { return r.ok ? r.json() : r.text().then(function (t) { throw new Error(t); }); })
        .then(function () { alert('Platform settings saved'); })
        .catch(function (e) { alert(e.message || 'Failed'); });
    });
    var btnRev = document.getElementById('agent-edit-new-revision');
    if (btnRev) btnRev.addEventListener('click', function () {
      if (!agentEditId) return;
      var prompt = document.getElementById('agent-edit-prompt').value;
      authFetch('/superadmin/api/dashboard/agents/' + encodeURIComponent(agentEditId) + '/revisions', {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ payload: { system_prompt: prompt }, created_by: 'dashboard' })
      }).then(function (r) { return r.ok ? r.json() : r.text().then(function (t) { throw new Error(t); }); })
        .then(function () { openEditAgentModal(agentEditId); })
        .catch(function (e) { alert(e.message || 'Failed'); });
    });
    document.getElementById('agent-edit-modal-close').addEventListener('click', function() { agentEditModal.hidden = true; });
    agentEditModal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { agentEditModal.hidden = true; });
    document.getElementById('agent-edit-modal-save').addEventListener('click', function() {
      if (!agentEditId) return;
      var errEl = document.getElementById('agent-edit-modal-error');
      var name = document.getElementById('agent-edit-name').value.trim();
      var configStr = document.getElementById('agent-edit-config').value.trim();
      var ingestConfigStr = document.getElementById('agent-edit-ingest-source-config').value.trim();
      var ingestType = document.getElementById('agent-edit-ingest-source-type').value.trim();
      if (!name) { errEl.textContent = 'Name is required'; errEl.hidden = false; return; }
      var configVal = null;
      if (configStr) {
        try { configVal = JSON.parse(configStr); } catch (e) { errEl.textContent = 'Config must be valid JSON'; errEl.hidden = false; return; }
      }
      var ingestConfigVal = null;
      if (ingestConfigStr) {
        try { ingestConfigVal = JSON.parse(ingestConfigStr); } catch (e) { errEl.textContent = 'Data source config must be valid JSON'; errEl.hidden = false; return; }
      }
      errEl.hidden = true;
      var payload = {
        name: name,
        system_prompt: document.getElementById('agent-edit-prompt').value,
        chat_capable: document.getElementById('agent-edit-chat-capable').checked,
        slack_notifications_enabled: document.getElementById('agent-edit-slack-notifications').checked,
        ingest_source_type: ingestType || '',
        ingest_source_config: ingestConfigVal
      };
      if (configVal !== null) payload.config = configVal;
      if (payload.ingest_source_type === '') delete payload.ingest_source_config;
      var btn = document.getElementById('agent-edit-modal-save');
      btn.disabled = true;
      authFetch('/agents/' + encodeURIComponent(agentEditId), { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
        .then(function(r) {
          return r.text().then(function(t) {
            var d = {};
            if (t) { try { d = JSON.parse(t); } catch (e) { d = { error: t }; } }
            return { ok: r.ok, data: d };
          });
        })
        .then(function(res) {
          btn.disabled = false;
          if (res.ok) { agentEditModal.hidden = true; if (typeof fetchSnapshot === 'function') fetchSnapshot(); }
          else { errEl.textContent = res.data.error || 'Save failed'; errEl.hidden = false; }
        })
        .catch(function(e) { btn.disabled = false; errEl.textContent = e && e.message ? e.message : 'Request failed'; errEl.hidden = false; });
    });
  }

  // ─── Create Agent Modal ─────────────────────────────────
  var ingestSourceHints = {
    '': { hint: '', placeholder: '' },
    redis_pubsub: { hint: 'Redis: channel name. Example: {"channel":"alerts"}', placeholder: '{"channel":"alerts"}' },
    gcp_pubsub: { hint: 'GCP: project and subscription. Example: {"project":"my-project","subscription":"my-sub"}', placeholder: '{"project":"my-project","subscription":"my-sub"}' },
    websocket: { hint: 'WebSocket: endpoint URL. Example: {"url":"wss://example.com/events"}', placeholder: '{"url":"wss://example.com/events"}' }
  };
  function updateIngestSourceHint(prefix) {
    var typeEl = document.getElementById(prefix + '-ingest-source-type');
    var configEl = document.getElementById(prefix + '-ingest-source-config');
    var hintEl = document.getElementById(prefix + '-ingest-hint');
    if (!typeEl || !configEl || !hintEl) return;
    var val = (typeEl.value || '').trim();
    var rec = ingestSourceHints[val] || ingestSourceHints[''];
    hintEl.textContent = rec.hint;
    configEl.placeholder = rec.placeholder;
  }

  var agentModal = document.getElementById('agent-modal');
  var agentUploadedContent = null;
  var agentUploadedFilename = '';

  function getJwtClaims() {
    try {
      var token = localStorage.getItem('astra_token');
      if (!token) return {};
      var parts = token.split('.');
      if (parts.length < 2) return {};
      var payload = atob(parts[1].replace(/-/g, '+').replace(/_/g, '/'));
      return JSON.parse(payload);
    } catch (e) { return {}; }
  }

  function showAgentError(msg) {
    var el = document.getElementById('agent-modal-error');
    if (el) { el.textContent = msg; el.hidden = false; }
  }

  if (agentModal) {
    document.getElementById('agent-modal-close').addEventListener('click', function() { agentModal.hidden = true; });
    agentModal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { agentModal.hidden = true; });
  }

  var btnCreateAgent = document.getElementById('btn-create-agent');
  if (btnCreateAgent) btnCreateAgent.addEventListener('click', function() {
    document.getElementById('agent-create-name').value = '';
    document.getElementById('agent-create-actor-type').value = '';
    document.getElementById('agent-create-config').value = '';
    document.getElementById('agent-create-chat-capable').checked = false;
    document.getElementById('agent-create-slack-notifications').checked = false;
    document.getElementById('agent-create-ingest-source-type').value = '';
    document.getElementById('agent-create-ingest-source-config').value = '';
    document.getElementById('agent-create-prompt-type').value = 'full_prompt';
    document.getElementById('agent-create-prompt').value = '';
    document.getElementById('agent-create-upload-type').value = 'full_prompt';
    var fileInput = document.getElementById('agent-create-file');
    if (fileInput) fileInput.value = '';
    document.getElementById('agent-create-file-info').textContent = 'Click to upload';
    document.getElementById('agent-create-file-clear').hidden = true;
    agentUploadedContent = null;
    agentUploadedFilename = '';
    document.getElementById('agent-modal-error').hidden = true;
    document.getElementById('agent-modal-title').textContent = 'Create Agent';
    updateIngestSourceHint('agent-create');
    agentModal.hidden = false;
  });

  var goalCreateModal = document.getElementById('goal-create-modal');
  var goalCreateAgentSelect = document.getElementById('goal-create-agent-id');
  var goalCreateText = document.getElementById('goal-create-text');
  var goalCreateWorkspace = document.getElementById('goal-create-workspace');
  var goalCreateError = document.getElementById('goal-create-modal-error');
  var goalCreateSaveBtn = document.getElementById('goal-create-modal-save');
  if (document.getElementById('btn-create-goal')) {
    document.getElementById('btn-create-goal').addEventListener('click', function() {
      if (!goalCreateAgentSelect) return;
      goalCreateAgentSelect.innerHTML = '<option value="">Select an agent…</option>';
      lastSnapshotAgents.forEach(function(a) {
        var id = (a.id || a.agent_id || '').toString();
        var name = (a.name || a.actor_type || id || '—').toString();
        if (id) {
          var opt = document.createElement('option');
          opt.value = id;
          opt.textContent = name;
          goalCreateAgentSelect.appendChild(opt);
        }
      });
      if (goalCreateText) goalCreateText.value = '';
      if (goalCreateWorkspace) goalCreateWorkspace.value = './workspace/demo';
      if (goalCreateError) { goalCreateError.hidden = true; goalCreateError.textContent = ''; }
      if (goalCreateModal) goalCreateModal.hidden = false;
    });
  }
  if (document.getElementById('goal-create-modal-close')) {
    document.getElementById('goal-create-modal-close').addEventListener('click', function() { if (goalCreateModal) goalCreateModal.hidden = true; });
  }
  if (goalCreateModal && goalCreateModal.querySelector('.goal-modal-backdrop')) {
    goalCreateModal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { goalCreateModal.hidden = true; });
  }
  if (goalCreateSaveBtn) {
    goalCreateSaveBtn.addEventListener('click', function() {
      var agentId = goalCreateAgentSelect && goalCreateAgentSelect.value ? goalCreateAgentSelect.value.trim() : '';
      var text = goalCreateText ? goalCreateText.value.trim() : '';
      var workspace = goalCreateWorkspace ? goalCreateWorkspace.value.trim() : '';
      if (!agentId) { if (goalCreateError) { goalCreateError.textContent = 'Select an agent.'; goalCreateError.hidden = false; } return; }
      if (!text) { if (goalCreateError) { goalCreateError.textContent = 'Enter a goal description.'; goalCreateError.hidden = false; } return; }
      if (goalCreateError) goalCreateError.hidden = true;
      goalCreateSaveBtn.disabled = true;
      goalCreateSaveBtn.textContent = 'Creating…';
      var payload = { goal_text: text };
      if (workspace) payload.workspace = workspace;
      authFetch('/agents/' + encodeURIComponent(agentId) + '/goals', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      })
        .then(function(r) {
          if (r.ok) { if (goalCreateModal) goalCreateModal.hidden = true; fetchSnapshot(); return; }
          return r.text().then(function(t) {
            var msg = 'Create goal failed';
            try {
              var j = JSON.parse(t);
              if (j && (j.error || j.message)) msg = j.message || j.error;
              else if (t) msg = t;
            } catch (e) {
              if (t) msg = t;
            }
            throw new Error(msg);
          });
        })
        .catch(function(e) {
          if (goalCreateError) { goalCreateError.textContent = e && e.message ? e.message : 'Create goal failed'; goalCreateError.hidden = false; }
        })
        .finally(function() {
          goalCreateSaveBtn.disabled = false;
          goalCreateSaveBtn.textContent = 'Create Goal';
        });
    });
  }

  var createIngestTypeSel = document.getElementById('agent-create-ingest-source-type');
  if (createIngestTypeSel) createIngestTypeSel.addEventListener('change', function() { updateIngestSourceHint('agent-create'); });

  var agentFileInput = document.getElementById('agent-create-file');
  if (agentFileInput) agentFileInput.addEventListener('change', function() {
    var file = agentFileInput.files[0];
    if (!file) return;
    var ext = file.name.split('.').pop().toLowerCase();
    if (['md', 'txt', 'markdown'].indexOf(ext) === -1) {
      showAgentError('Only .md, .txt, or .markdown files are allowed');
      agentFileInput.value = '';
      return;
    }
    if (file.size > 1048576) {
      showAgentError('File must be 1 MB or smaller');
      agentFileInput.value = '';
      return;
    }
    document.getElementById('agent-modal-error').hidden = true;
    var reader = new FileReader();
    reader.onload = function(e) {
      agentUploadedContent = e.target.result;
      agentUploadedFilename = file.name;
      document.getElementById('agent-create-file-info').textContent = file.name + ' (' + (file.size / 1024).toFixed(1) + ' KB)';
      document.getElementById('agent-create-file-clear').hidden = false;
    };
    reader.readAsText(file, 'UTF-8');
  });

  var agentCreateFileZone = document.getElementById('agent-create-file-zone');
  if (agentCreateFileZone && agentFileInput) {
    agentCreateFileZone.addEventListener('click', function(e) {
      if (!e.target.closest('.agent-file-clear')) agentFileInput.click();
    });
  }
  var agentFileClear = document.getElementById('agent-create-file-clear');
  if (agentFileClear) agentFileClear.addEventListener('click', function(e) {
    e.preventDefault();
    e.stopPropagation();
    agentUploadedContent = null;
    agentUploadedFilename = '';
    var fileInput = document.getElementById('agent-create-file');
    if (fileInput) fileInput.value = '';
    document.getElementById('agent-create-file-info').textContent = 'Click to upload';
    agentFileClear.hidden = true;
  });

  function docTypeFromContentType(ct) {
    if (ct === 'rule') return 'rule';
    if (ct === 'skill') return 'skill';
    return 'context_doc';
  }

  var agentSaveBtn = document.getElementById('agent-modal-save');
  if (agentSaveBtn) agentSaveBtn.addEventListener('click', function() {
    var name = document.getElementById('agent-create-name').value.trim();
    var actorType = document.getElementById('agent-create-actor-type').value.trim();
    var config = document.getElementById('agent-create-config').value.trim();
    var chatCapable = document.getElementById('agent-create-chat-capable').checked;
    var promptType = document.getElementById('agent-create-prompt-type').value;
    var promptContent = document.getElementById('agent-create-prompt').value;
    var uploadType = document.getElementById('agent-create-upload-type').value;

    if (!name) { showAgentError('Name is required'); return; }
    if (!actorType) { showAgentError('Actor type is required'); return; }
    if (!promptContent.trim() && !agentUploadedContent) { showAgentError('Provide an agent prompt or upload a document'); return; }
    if (config) {
      try { JSON.parse(config); } catch (e) { showAgentError('Config must be valid JSON'); return; }
    }

    var systemPrompt = '';
    if (promptContent.trim() && promptType === 'full_prompt') {
      systemPrompt = promptContent;
    } else if (agentUploadedContent && uploadType === 'full_prompt' && !systemPrompt) {
      systemPrompt = agentUploadedContent;
    }

    agentSaveBtn.disabled = true;
    agentSaveBtn.textContent = 'Creating...';
    document.getElementById('agent-modal-error').hidden = true;

    var ingestSourceType = document.getElementById('agent-create-ingest-source-type').value.trim();
    var ingestSourceConfigStr = document.getElementById('agent-create-ingest-source-config').value.trim();
    var slackNotifications = document.getElementById('agent-create-slack-notifications').checked;
    var ingestSourceConfig = null;
    if (ingestSourceConfigStr) {
      try { ingestSourceConfig = JSON.parse(ingestSourceConfigStr); } catch (e) { showAgentError('Data source config must be valid JSON'); agentSaveBtn.disabled = false; agentSaveBtn.textContent = 'Create Agent'; return; }
    }

    authFetch('/agents', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ actor_type: actorType, name: name, system_prompt: systemPrompt, config: config || '{}', chat_capable: chatCapable })
    })
      .then(function(r) { return r.json().then(function(d) { return { ok: r.ok, data: d }; }); })
      .then(function(res) {
        if (!res.ok) { showAgentError(res.data.error || 'Failed to create agent'); agentSaveBtn.disabled = false; agentSaveBtn.textContent = 'Create Agent'; return; }
        var agentId = res.data.actor_id;
        var patchPayload = {};
        if (ingestSourceType) patchPayload.ingest_source_type = ingestSourceType;
        if (ingestSourceConfig != null) patchPayload.ingest_source_config = ingestSourceConfig;
        patchPayload.slack_notifications_enabled = slackNotifications;
        var docPromises = [];

        if (promptContent.trim()) {
          docPromises.push(
            authFetch('/agents/' + agentId + '/documents', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ doc_type: docTypeFromContentType(promptType), name: name + '-prompt', content: promptContent, priority: promptType === 'full_prompt' ? 10 : 50 })
            })
          );
        }

        if (agentUploadedContent) {
          docPromises.push(
            authFetch('/agents/' + agentId + '/documents', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({ doc_type: docTypeFromContentType(uploadType), name: agentUploadedFilename || (name + '-upload'), content: agentUploadedContent, priority: uploadType === 'full_prompt' ? 10 : 50 })
            })
          );
        }

        var patchPromise = (Object.keys(patchPayload).length > 0)
          ? authFetch('/agents/' + agentId, { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(patchPayload) })
          : Promise.resolve();
        return Promise.all(docPromises).then(function() { return patchPromise; }).then(function() {
          agentModal.hidden = true;
          agentSaveBtn.disabled = false;
          agentSaveBtn.textContent = 'Create Agent';
          if (typeof fetchSnapshot === 'function') fetchSnapshot();
        });
      })
      .catch(function(e) {
        showAgentError(e.message || 'Failed');
        agentSaveBtn.disabled = false;
        agentSaveBtn.textContent = 'Create Agent';
      });
  });
});

function debounce(fn, ms) {
  var t; return function() { clearTimeout(t); t = setTimeout(fn, ms); };
}

function esc(s) { if (!s) return ''; var d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
