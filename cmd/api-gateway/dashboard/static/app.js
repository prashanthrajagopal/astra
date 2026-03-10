/* global Chart */

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

// ─── Summary cards ──────────────────────────────────────────────────

function renderSummary(data) {
  const goals = data.jobs && data.jobs.goals ? data.jobs.goals : {};
  const tasks = data.jobs && data.jobs.tasks ? data.jobs.tasks : {};
  const workers = data.workers || [];
  const approvals = data.approvals || [];

  setText('sum-active-goals', goals.active || 0);
  setText('sum-completed-tasks', tasks.completed || 0);
  setText('sum-running-tasks', tasks.running || 0);
  setText('sum-failed-tasks', tasks.failed || 0);

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

/* M3 theme palette for Chart.js (aligned with style.css tokens) */
var chartColors = {
  created: '#8e9099',
  pending: '#c8b8ff',
  queued: '#a8c7fa',
  scheduled: '#9ecbf5',
  running: '#e6c547',
  completed: '#7dd87d',
  failed: '#f2b8b5'
};

function renderTaskChart(tasks) {
  var ctx = document.getElementById('chart-tasks');
  if (!ctx) return;
  var labels = ['created', 'pending', 'queued', 'scheduled', 'running', 'completed', 'failed'];
  var values = labels.map(function (l) { return tasks[l] || 0; });
  var colors = labels.map(function (l) { return chartColors[l] || '#8e9099'; });

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
        legend: { position: 'right', labels: { color: '#e6e1e5', font: { size: 11, family: 'Roboto, sans-serif' } } }
      }
    }
  });
}

function renderGoalChart(goals) {
  var ctx = document.getElementById('chart-goals');
  if (!ctx) return;
  var labels = ['active', 'completed', 'failed', 'pending'];
  var values = labels.map(function (l) { return goals[l] || 0; });
  var colors = ['#e6c547', '#7dd87d', '#f2b8b5', '#c8b8ff'];

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
        x: { ticks: { color: '#c4c6d0', font: { family: 'Roboto, sans-serif' } }, grid: { color: '#44474e' } },
        y: { beginAtZero: true, ticks: { color: '#c4c6d0', stepSize: 1, font: { family: 'Roboto, sans-serif' } }, grid: { color: '#44474e' } }
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
        { label: 'Healthy', data: healthy, backgroundColor: '#7dd87d', borderWidth: 0 },
        { label: 'Unhealthy', data: unhealthy, backgroundColor: '#f2b8b5', borderWidth: 0 }
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      indexAxis: 'y',
      scales: {
        x: { stacked: true, max: 1, ticks: { display: false }, grid: { color: '#44474e' } },
        y: { stacked: true, ticks: { color: '#c4c6d0', font: { size: 10, family: 'Roboto, sans-serif' } }, grid: { display: false } }
      },
      plugins: { legend: { labels: { color: '#e6e1e5', font: { size: 11, family: 'Roboto, sans-serif' } } } }
    }
  });
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
        backgroundColor: ['#7dd87d', '#f2b8b5'],
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
  el.textContent = 'Total: $' + total.toFixed(2);
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

function renderRecentGoals(recentGoals) {
  var tbody = document.getElementById('tbody-goals');
  tbody.innerHTML = '';
  if (!recentGoals || recentGoals.length === 0) return tbody.appendChild(emptyRow(5, 'No goals yet'));
  recentGoals.forEach(function (g) {
    var tr = document.createElement('tr');
    var st = (g.status || '').toLowerCase();
    tr.innerHTML = '<td>' + (g.id || '').substring(0, 8) + '</td>' +
      '<td>' + (g.agent_id || '').substring(0, 8) + '</td>' +
      '<td class="goal-text-cell" title="' + (g.goal_text || '').replace(/"/g, '&quot;') + '">' + (g.goal_text || '') + '</td>' +
      '<td class="td-status status-' + st + '">' + (g.status || '') + '</td>' +
      '<td>' + (g.created_at || '') + '</td>';
    tbody.appendChild(tr);
  });
}

function renderServices(services) {
  var tbody = document.getElementById('tbody-services');
  tbody.innerHTML = '';
  if (!services || services.length === 0) return tbody.appendChild(emptyRow(5, 'No services configured'));
  services.forEach(function (s) {
    var tr = document.createElement('tr');
    tr.className = 'tr-service';
    var latencyClass = Number(s.latency_ms) > 10 ? 'latency-warning' : '';
    tr.innerHTML = '<td>' + (s.name || '') + '</td><td>' + (s.port || '') + '</td><td>' + (s.type || '') + '</td>' +
      '<td class="td-status ' + (s.healthy ? 'status-healthy' : 'status-unhealthy') + '">' + (s.healthy ? 'healthy' : 'unhealthy') + '</td>' +
      '<td class="' + latencyClass + '">' + (s.latency_ms != null ? s.latency_ms : '') + '</td>';
    tbody.appendChild(tr);
  });
}

function renderWorkers(workers) {
  var tbody = document.getElementById('tbody-workers');
  tbody.innerHTML = '';
  if (!workers || workers.length === 0) return tbody.appendChild(emptyRow(5, 'No workers registered'));
  workers.forEach(function (w) {
    var tr = document.createElement('tr');
    var id = pick(w, ['id', 'ID']);
    var hostname = pick(w, ['hostname', 'Hostname']);
    var statusText = pick(w, ['status', 'Status']);
    var capabilities = pick(w, ['capabilities', 'Capabilities'], []);
    var lastHeartbeat = pick(w, ['last_heartbeat', 'LastHeartbeat', 'lastHeartbeat']);
    var status = statusText.toString().toLowerCase();
    var cls = status === 'active' ? 'status-active' : (status ? 'status-inactive' : 'status-stale');
    tr.innerHTML = '<td>' + id + '</td><td>' + hostname + '</td><td class="td-status ' + cls + '">' + statusText + '</td>' +
      '<td>' + (Array.isArray(capabilities) ? capabilities.join(',') : '') + '</td><td>' + lastHeartbeat + '</td>';
    tbody.appendChild(tr);
  });
}

function renderApprovals(items) {
  var tbody = document.getElementById('tbody-approvals');
  tbody.innerHTML = '';
  if (!items || items.length === 0) return tbody.appendChild(emptyRow(6, 'No pending approvals'));
  items.forEach(function (a) {
    var tr = document.createElement('tr');
    var st = (a.status || 'pending').toString().toLowerCase();
    tr.innerHTML = '<td>' + (a.id || '') + '</td><td>' + (a.tool_name || '') + '</td><td>' + (a.action_summary || '') + '</td>' +
      '<td class="td-status status-' + st + '">' + (a.status || '') + '</td><td>' + (a.requested_at || '') + '</td>' +
      '<td><button class="action-btn approve" data-action="approve" data-id="' + (a.id || '') + '">Approve</button>' +
      '<button class="action-btn reject" data-action="reject" data-id="' + (a.id || '') + '">Reject</button></td>';
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
    tr.innerHTML = '<td>' + (r.day || '') + '</td><td>' + (r.agent_id || '') + '</td><td>' + (r.model || '') + '</td>' +
      '<td>' + (r.tokens_in != null ? r.tokens_in : '') + '</td><td>' + (r.tokens_out != null ? r.tokens_out : '') + '</td>' +
      '<td>' + (r.cost_dollars != null ? r.cost_dollars : '') + '</td>';
    tbody.appendChild(tr);
  });
}

function renderLogs(logs) {
  var container = document.getElementById('logs-container');
  container.innerHTML = '';
  var names = logs ? Object.keys(logs) : [];
  if (names.length === 0) {
    var pre = document.createElement('pre');
    pre.className = 'empty-message';
    pre.textContent = 'No logs available';
    container.appendChild(pre);
    return;
  }
  names.sort().forEach(function (name) {
    var block = document.createElement('div');
    block.className = 'log-block';
    var title = document.createElement('h3');
    title.className = 'log-block-title';
    title.textContent = name + ' (last 20 lines)';
    var pre = document.createElement('pre');
    pre.className = 'log-block-content';
    var lines = Array.isArray(logs[name]) ? logs[name] : [];
    pre.textContent = lines.length > 0 ? lines.join('\n') : 'No logs available';
    block.appendChild(title);
    block.appendChild(pre);
    container.appendChild(block);
  });
}

function renderPids(pids) {
  var tbody = document.getElementById('tbody-pids');
  tbody.innerHTML = '';
  var names = pids ? Object.keys(pids) : [];
  if (names.length === 0) return tbody.appendChild(emptyRow(2, 'No PID data'));
  names.sort().forEach(function (name) {
    var tr = document.createElement('tr');
    tr.innerHTML = '<td>' + name + '</td><td>' + pids[name] + '</td>';
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
  fetch('/api/dashboard/approvals/' + encodeURIComponent(id) + '/' + action, {
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
  return fetch('/api/dashboard/snapshot', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderSummary(d);

      var tasks = d.jobs && d.jobs.tasks ? d.jobs.tasks : {};
      var goals = d.jobs && d.jobs.goals ? d.jobs.goals : {};
      var recentGoals = d.jobs && d.jobs.recent_goals ? d.jobs.recent_goals : [];

      renderTaskChart(tasks);
      renderGoalChart(goals);
      renderServiceChart(d.services || []);
      renderRecentGoals(recentGoals);
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

      document.getElementById('last-updated').textContent = 'Last updated: ' + new Date().toISOString();
      setStatus('Idle', false);
    })
    .catch(function (e) {
      setStatus('Error: ' + (e.message || e), true);
    })
    .finally(function () {
      inFlight = false;
    });
}

document.addEventListener('DOMContentLoaded', function () {
  document.getElementById('btn-refresh').addEventListener('click', fetchSnapshot);
  document.getElementById('tbody-approvals').addEventListener('click', function (e) {
    var t = e.target;
    if (!(t instanceof HTMLElement)) return;
    if (!t.dataset || !t.dataset.action || !t.dataset.id) return;
    submitApprovalAction(t.dataset.id, t.dataset.action);
  });
  fetchSnapshot();
  setInterval(fetchSnapshot, 5000);
});
