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
  setText('sum-failed-tasks', tasks.failed || 0);
  setText('sum-agents', data.agent_count != null ? data.agent_count : (Array.isArray(data.agents) ? data.agents.length : 0));

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
  var colors = ['#7dd87d', '#e6c547', '#8e9099', '#c8b8ff', '#f2b8b5'];
  labels.forEach(function (_, i) {
    if (!colors[i]) colors[i] = '#8e9099';
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
        legend: { position: 'right', labels: { color: '#e6e1e5', font: { size: 11, family: 'Roboto, sans-serif' } } }
      }
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
    tbody.appendChild(emptyRow(4, total === 0 ? 'No agents' : 'No agents on this page'));
  } else {
    slice.forEach(function (a) {
      var tr = document.createElement('tr');
      var id = pick(a, ['id', 'ID'], '');
      var name = pick(a, ['name', 'actor_type', 'Name'], '');
      var status = (pick(a, ['status', 'Status'], '') || '').toLowerCase();
      var isActive = status === 'active';
      var actionsHtml = '<td class="td-actions">' +
        '<button type="button" class="agent-action-btn agent-action-enable" data-agent-id="' + escapeHtml(id || '') + '" data-action="enable" aria-label="Enable" title="Enable">▶</button>' +
        '<button type="button" class="agent-action-btn agent-action-disable" data-agent-id="' + escapeHtml(id || '') + '" data-action="disable" aria-label="Disable" title="Disable">⏸</button>' +
        '<button type="button" class="agent-action-btn agent-action-delete" data-agent-id="' + escapeHtml(id || '') + '" data-action="delete" aria-label="Delete" title="Delete">🗑</button>' +
        '</td>';
      tr.innerHTML = '<td>' + (id ? id.substring(0, 8) : '') + '</td><td>' + (name || '—') + '</td><td class="td-status">' + (status || '—') + '</td>' + actionsHtml;
      tbody.appendChild(tr);
    });
  }
  if (prevBtn) prevBtn.disabled = page <= 1;
  if (nextBtn) nextBtn.disabled = page >= totalPages;
  if (infoEl) infoEl.textContent = 'Page ' + page + ' of ' + totalPages + (total ? ' (' + total + ' agents)' : '');
}

function renderAgents(agents) {
  agentsList = Array.isArray(agents) ? agents : [];
  agentsPage = 1;
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
    var actionsHtml = isCancellable
      ? '<button class="cancel-goal-btn" data-goal-id="' + escapeHtml(g.id || '') + '" title="Cancel goal">✕</button>'
      : '';
    tr.innerHTML = '<td>' + (g.id || '').substring(0, 8) + '</td>' +
      '<td>' + (g.agent_id || '').substring(0, 8) + '</td>' +
      '<td class="goal-text-cell" title="' + (g.goal_text || '').replace(/"/g, '&quot;') + '">' + (g.goal_text || '') + '</td>' +
      '<td class="td-status status-' + st + '">' + (g.status || '') + '</td>' +
      '<td>' + (g.created_at || '') + '</td>' +
      '<td class="td-actions">' + actionsHtml + '</td>';
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
  if (!items || items.length === 0) return tbody.appendChild(emptyRow(7, 'No pending approvals'));
  items.forEach(function (a) {
    var tr = document.createElement('tr');
    tr.className = 'approval-row';
    tr.setAttribute('data-approval-id', a.id || '');
    var reqType = (a.request_type || 'risky_task').toLowerCase();
    var typeLabel = reqType === 'plan' ? 'Plan' : 'Risky task';
    var toolOrSummary = reqType === 'plan' ? (a.summary || '—') : (a.tool_name || '');
    var st = (a.status || 'pending').toString().toLowerCase();
    tr.innerHTML = '<td class="td-type">' + typeLabel + '</td><td>' + (a.id ? a.id.substring(0, 8) : '') + '</td><td class="tool-summary-cell">' + escapeHtml((toolOrSummary || '').toString().substring(0, 80)) + (toolOrSummary && toolOrSummary.length > 80 ? '…' : '') + '</td><td>' + escapeHtml((a.action_summary || '').toString().substring(0, 60)) + '</td>' +
      '<td class="td-status status-' + st + '">' + (a.status || '') + '</td><td>' + (a.requested_at || '') + '</td>' +
      '<td><button class="action-btn view" data-id="' + (a.id || '') + '">View</button>' +
      '<button class="action-btn approve" data-action="approve" data-id="' + (a.id || '') + '">Approve</button>' +
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

document.addEventListener('DOMContentLoaded', function () {
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
      if (action === 'enable') {
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
  setInterval(fetchSnapshot, 5000);

  // ─── Tab navigation ────────────────────────────────────────────────
  document.querySelectorAll('.nav-tab').forEach(function(tab) {
    tab.addEventListener('click', function() {
      document.querySelectorAll('.nav-tab').forEach(function(t) { t.classList.remove('active'); });
      tab.classList.add('active');
      var target = tab.dataset.tab;
      document.querySelectorAll('.tab-panel').forEach(function(p) { p.hidden = true; });
      var panel = document.getElementById('tab-' + target);
      if (panel) panel.hidden = false;
      if (target === 'orgs') loadOrgs();
      if (target === 'users') loadUsers();
    });
  });

  // ─── Org management ────────────────────────────────────────────────
  var orgModal = document.getElementById('org-modal');
  if (orgModal) {
    document.getElementById('org-modal-close').addEventListener('click', function() { orgModal.hidden = true; });
    orgModal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { orgModal.hidden = true; });
  }
  var orgAdminSelect = document.getElementById('org-admin-select');
  var orgAdminNewFields = document.getElementById('org-admin-new-fields');
  if (orgAdminSelect) orgAdminSelect.addEventListener('change', function() {
    orgAdminNewFields.hidden = !!orgAdminSelect.value;
  });
  var orgNameInput = document.getElementById('org-name');
  if (orgNameInput) orgNameInput.addEventListener('input', function() {
    var slugEl = document.getElementById('org-slug');
    if (slugEl) slugEl.value = orgNameInput.value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
  });

  var btnCreateOrg = document.getElementById('btn-create-org');
  if (btnCreateOrg) btnCreateOrg.addEventListener('click', function() {
    document.getElementById('org-name').value = '';
    document.getElementById('org-slug').value = '';
    document.getElementById('org-admin-name') && (document.getElementById('org-admin-name').value = '');
    document.getElementById('org-admin-email') && (document.getElementById('org-admin-email').value = '');
    document.getElementById('org-admin-password') && (document.getElementById('org-admin-password').value = '');
    if (orgAdminSelect) { orgAdminSelect.value = ''; orgAdminNewFields.hidden = false; }
    document.getElementById('org-modal-error').hidden = true;
    document.getElementById('org-modal-title').textContent = 'Create Organization';
    populateOrgAdminDropdown();
    orgModal.hidden = false;
  });
  var orgSaveBtn = document.getElementById('org-modal-save');
  if (orgSaveBtn) orgSaveBtn.addEventListener('click', function() {
    var name = document.getElementById('org-name').value.trim();
    var slug = document.getElementById('org-slug').value.trim();
    if (!name || !slug) { showOrgError('Name and slug required'); return; }
    var existingUserId = orgAdminSelect ? orgAdminSelect.value : '';
    var adminName = (document.getElementById('org-admin-name') || {}).value || '';
    var adminEmail = (document.getElementById('org-admin-email') || {}).value || '';
    var adminPass = (document.getElementById('org-admin-password') || {}).value || '';
    if (!existingUserId && (!adminName.trim() || !adminEmail.trim() || !adminPass)) {
      showOrgError('Org admin is required — select an existing user or fill in new admin details');
      return;
    }
    orgSaveBtn.disabled = true; orgSaveBtn.textContent = 'Creating...';
    authFetch('/superadmin/api/orgs', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({name:name,slug:slug}) })
      .then(function(r) { return r.json().then(function(d) { return {ok:r.ok,data:d}; }); })
      .then(function(res) {
        if (!res.ok) { showOrgError(res.data.error || 'Failed to create org'); orgSaveBtn.disabled = false; orgSaveBtn.textContent = 'Create Organization'; return; }
        var orgId = res.data.id;
        if (existingUserId) {
          return addOrgAdmin(orgId, existingUserId);
        } else {
          return authFetch('/superadmin/api/users', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({email:adminEmail.trim(),name:adminName.trim(),password:adminPass}) })
            .then(function(r2) { return r2.json().then(function(d2) { return {ok:r2.ok,data:d2}; }); })
            .then(function(res2) {
              if (!res2.ok) { showOrgError('Org created but admin user failed: ' + (res2.data.error || '')); orgSaveBtn.disabled = false; orgSaveBtn.textContent = 'Create Organization'; return; }
              return addOrgAdmin(orgId, res2.data.id);
            });
        }
      })
      .catch(function(e) { showOrgError(e.message); orgSaveBtn.disabled = false; orgSaveBtn.textContent = 'Create Organization'; });
  });
  function addOrgAdmin(orgId, userId) {
    return authFetch('/superadmin/api/orgs/' + orgId + '/admins', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({user_id:userId}) })
      .then(function() {
        orgModal.hidden = true;
        var btn = document.getElementById('org-modal-save');
        btn.disabled = false; btn.textContent = 'Create Organization';
        loadOrgs();
      });
  }
  function populateOrgAdminDropdown() {
    if (!orgAdminSelect) return;
    while (orgAdminSelect.options.length > 1) orgAdminSelect.remove(1);
    authFetch('/superadmin/api/users?per_page=200')
      .then(function(r) { return r.json(); })
      .then(function(d) {
        (d.users || []).forEach(function(u) {
          var opt = document.createElement('option');
          opt.value = u.id; opt.textContent = u.name + ' (' + u.email + ')';
          orgAdminSelect.appendChild(opt);
        });
      }).catch(function() {});
  }

  // ─── User management ───────────────────────────────────────────────
  var userModal = document.getElementById('user-modal');
  if (userModal) {
    document.getElementById('user-modal-close').addEventListener('click', function() { userModal.hidden = true; });
    userModal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { userModal.hidden = true; });
  }
  var userOrgSelect = document.getElementById('user-org-select');
  var userOrgRoleField = document.getElementById('user-org-role-field');
  if (userOrgSelect) userOrgSelect.addEventListener('change', function() {
    userOrgRoleField.hidden = !userOrgSelect.value;
  });

  var btnCreateUser = document.getElementById('btn-create-user');
  if (btnCreateUser) btnCreateUser.addEventListener('click', function() {
    document.getElementById('user-name').value = '';
    document.getElementById('user-email').value = '';
    document.getElementById('user-password').value = '';
    document.getElementById('user-superadmin').checked = false;
    if (userOrgSelect) { userOrgSelect.value = ''; userOrgRoleField.hidden = true; }
    document.getElementById('user-org-role') && (document.getElementById('user-org-role').value = 'member');
    document.getElementById('user-modal-error').hidden = true;
    populateUserOrgDropdown();
    userModal.hidden = false;
  });
  var userSaveBtn = document.getElementById('user-modal-save');
  if (userSaveBtn) userSaveBtn.addEventListener('click', function() {
    var name = document.getElementById('user-name').value.trim();
    var email = document.getElementById('user-email').value.trim();
    var password = document.getElementById('user-password').value;
    var isSuperAdmin = document.getElementById('user-superadmin').checked;
    if (!name || !email || !password) { showUserError('Name, email, and password required'); return; }
    var selectedOrg = userOrgSelect ? userOrgSelect.value : '';
    var orgRole = (document.getElementById('user-org-role') || {}).value || 'member';
    userSaveBtn.disabled = true; userSaveBtn.textContent = 'Creating...';
    authFetch('/superadmin/api/users', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({email:email,name:name,password:password,is_super_admin:isSuperAdmin}) })
      .then(function(r) { return r.json().then(function(d) { return {ok:r.ok,data:d}; }); })
      .then(function(res) {
        if (!res.ok) { showUserError(res.data.error || 'Failed'); userSaveBtn.disabled = false; userSaveBtn.textContent = 'Create User'; return; }
        var userId = res.data.id;
        if (selectedOrg) {
          return authFetch('/superadmin/api/users/' + userId + '/orgs', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({org_id:selectedOrg,role:orgRole}) })
            .then(function() {
              userModal.hidden = true; userSaveBtn.disabled = false; userSaveBtn.textContent = 'Create User';
              loadUsers();
            });
        }
        userModal.hidden = true; userSaveBtn.disabled = false; userSaveBtn.textContent = 'Create User';
        loadUsers();
      })
      .catch(function(e) { showUserError(e.message); userSaveBtn.disabled = false; userSaveBtn.textContent = 'Create User'; });
  });
  function populateUserOrgDropdown() {
    if (!userOrgSelect) return;
    while (userOrgSelect.options.length > 1) userOrgSelect.remove(1);
    authFetch('/superadmin/api/orgs?limit=200&offset=0')
      .then(function(r) { return r.json(); })
      .then(function(d) {
        (d.orgs || d || []).forEach(function(o) {
          var opt = document.createElement('option');
          opt.value = o.id; opt.textContent = o.name + ' (' + o.slug + ')';
          userOrgSelect.appendChild(opt);
        });
      }).catch(function() {});
  }

  var usersSearch = document.getElementById('users-search');
  if (usersSearch) usersSearch.addEventListener('input', debounce(function() { usersPage = 1; loadUsers(); }, 300));
  var usersOrgFilter = document.getElementById('users-org-filter');
  if (usersOrgFilter) usersOrgFilter.addEventListener('change', function() { usersPage = 1; loadUsers(); });
  var usersStatusFilter = document.getElementById('users-status-filter');
  if (usersStatusFilter) usersStatusFilter.addEventListener('change', function() { usersPage = 1; loadUsers(); });
  var usersPrev = document.getElementById('users-prev');
  var usersNext = document.getElementById('users-next');
  if (usersPrev) usersPrev.addEventListener('click', function() { usersPage = Math.max(1, usersPage - 1); loadUsers(); });
  if (usersNext) usersNext.addEventListener('click', function() { usersPage++; loadUsers(); });
});

function showOrgError(msg) {
  var el = document.getElementById('org-modal-error');
  el.textContent = msg; el.hidden = false;
}
function showUserError(msg) {
  var el = document.getElementById('user-modal-error');
  el.textContent = msg; el.hidden = false;
}
function debounce(fn, ms) {
  var t; return function() { clearTimeout(t); t = setTimeout(fn, ms); };
}

function loadOrgs() {
  authFetch('/superadmin/api/orgs?limit=100&offset=0')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var list = d.orgs || d || [];
      if (!Array.isArray(list)) list = [];
      var tbody = document.getElementById('tbody-orgs');
      var empty = document.getElementById('orgs-empty');
      if (!tbody) return;
      tbody.innerHTML = '';
      if (list.length === 0) { empty.hidden = false; return; }
      empty.hidden = true;
      list.forEach(function(o) {
        var tr = document.createElement('tr');
        tr.style.cursor = 'pointer';
        tr.setAttribute('data-org-id', o.id);
        var st = (o.status || 'active').toLowerCase();
        tr.innerHTML = '<td>' + esc(o.name) + '</td><td><code>' + esc(o.slug) + '</code></td>' +
          '<td class="badge-' + st + '">' + esc(o.status || 'active') + '</td>' +
          '<td>' + esc((o.created_at || '').substring(0, 10)) + '</td>' +
          '<td><button class="action-btn reject org-delete-btn" data-id="' + o.id + '" style="font-size:.75rem">Delete</button></td>';
        tbody.appendChild(tr);
      });
      var sel = document.getElementById('users-org-filter');
      if (sel && sel.options.length <= 1) {
        list.forEach(function(o) {
          var opt = document.createElement('option');
          opt.value = o.id; opt.textContent = o.name;
          sel.appendChild(opt);
        });
      }
    })
    .catch(function() {});
}

(function() {
  var tbodyOrgs = document.getElementById('tbody-orgs');
  if (tbodyOrgs) tbodyOrgs.addEventListener('click', function(e) {
    var btn = e.target && e.target.closest && e.target.closest('.org-delete-btn');
    if (btn) {
      e.stopPropagation();
      if (!confirm('Delete this organization and all its data?')) return;
      authFetch('/superadmin/api/orgs/' + btn.dataset.id, { method: 'DELETE' })
        .then(function() { loadOrgs(); });
      return;
    }
    var tr = e.target && e.target.closest && e.target.closest('tr[data-org-id]');
    if (tr && tr.dataset.orgId) openOrgDetail(tr.dataset.orgId);
  });
})();

function openOrgDetail(orgId) {
  var modal = document.getElementById('org-detail-modal');
  if (!modal) return;
  modal.hidden = false;
  document.getElementById('org-detail-error').hidden = true;
  document.getElementById('org-detail-id').value = orgId;
  document.getElementById('org-detail-title').textContent = 'Loading...';
  authFetch('/superadmin/api/orgs/' + orgId)
    .then(function(r) { return r.json(); })
    .then(function(o) {
      document.getElementById('org-detail-title').textContent = o.name || 'Organization';
      document.getElementById('org-detail-name').value = o.name || '';
      document.getElementById('org-detail-slug').value = o.slug || '';
      document.getElementById('org-detail-status').value = o.status || 'active';
      loadOrgMembers(orgId);
      loadOrgAddUserDropdown(orgId);
    })
    .catch(function() { document.getElementById('org-detail-title').textContent = 'Error loading org'; });
}

function loadOrgMembers(orgId) {
  var tbody = document.getElementById('org-detail-members');
  var empty = document.getElementById('org-detail-members-empty');
  if (!tbody) return;
  tbody.innerHTML = '<tr><td colspan="4" style="color:var(--md-sys-color-on-surface-variant)">Loading...</td></tr>';
  authFetch('/org/api/members?org_id=' + orgId)
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var members = d.members || d || [];
      if (!Array.isArray(members)) members = [];
      tbody.innerHTML = '';
      if (members.length === 0) { empty.hidden = false; return; }
      empty.hidden = true;
      members.forEach(function(m) {
        var tr = document.createElement('tr');
        tr.innerHTML = '<td>' + esc(m.name) + '</td><td>' + esc(m.email) + '</td>' +
          '<td>' + esc(m.role) + '</td>' +
          '<td><button class="action-btn reject org-member-remove-btn" data-uid="' + m.user_id + '" data-oid="' + orgId + '" style="font-size:.7rem;padding:2px 8px">Remove</button></td>';
        tbody.appendChild(tr);
      });
    })
    .catch(function() { tbody.innerHTML = ''; empty.hidden = false; });
}

function loadOrgAddUserDropdown(orgId) {
  var sel = document.getElementById('org-detail-add-user');
  if (!sel) return;
  while (sel.options.length > 1) sel.remove(1);
  authFetch('/superadmin/api/users?per_page=200')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      (d.users || []).forEach(function(u) {
        var opt = document.createElement('option');
        opt.value = u.id; opt.textContent = u.name + ' (' + u.email + ')';
        sel.appendChild(opt);
      });
    }).catch(function() {});
}

(function() {
  var modal = document.getElementById('org-detail-modal');
  if (!modal) return;
  document.getElementById('org-detail-close').addEventListener('click', function() { modal.hidden = true; });
  modal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { modal.hidden = true; });

  document.getElementById('org-detail-save').addEventListener('click', function() {
    var orgId = document.getElementById('org-detail-id').value;
    var name = document.getElementById('org-detail-name').value.trim();
    var slug = document.getElementById('org-detail-slug').value.trim();
    var status = document.getElementById('org-detail-status').value;
    if (!name || !slug) { var e = document.getElementById('org-detail-error'); e.textContent = 'Name and slug required'; e.hidden = false; return; }
    authFetch('/superadmin/api/orgs/' + orgId, { method: 'PATCH', headers: {'Content-Type':'application/json'}, body: JSON.stringify({name:name,slug:slug,status:status}) })
      .then(function(r) { return r.json().then(function(d) { return {ok:r.ok,data:d}; }); })
      .then(function(res) {
        if (!res.ok) { var e = document.getElementById('org-detail-error'); e.textContent = res.data.error || 'Update failed'; e.hidden = false; return; }
        document.getElementById('org-detail-title').textContent = name;
        document.getElementById('org-detail-error').hidden = true;
        loadOrgs();
      });
  });

  document.getElementById('org-detail-add-btn').addEventListener('click', function() {
    var orgId = document.getElementById('org-detail-id').value;
    var userId = document.getElementById('org-detail-add-user').value;
    var role = document.getElementById('org-detail-add-role').value;
    if (!userId) return;
    authFetch('/superadmin/api/orgs/' + orgId + '/admins', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({user_id:userId,role:role}) })
      .then(function() {
        if (role === 'member') {
          return authFetch('/org/api/members?org_id=' + orgId, { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({user_id:userId,role:role}) });
        }
      })
      .then(function() { loadOrgMembers(orgId); document.getElementById('org-detail-add-user').value = ''; });
  });

  document.getElementById('org-detail-members').addEventListener('click', function(e) {
    var btn = e.target && e.target.closest && e.target.closest('.org-member-remove-btn');
    if (!btn) return;
    var orgId = btn.dataset.oid;
    var userId = btn.dataset.uid;
    if (!confirm('Remove this member from the organization?')) return;
    authFetch('/superadmin/api/users/' + userId + '/orgs/' + orgId, { method: 'DELETE' })
      .then(function() { loadOrgMembers(orgId); });
  });
})();

var usersPage = 1;
var USERS_PAGE_SIZE = 20;
function loadUsers() {
  var q = (document.getElementById('users-search') || {}).value || '';
  var orgId = (document.getElementById('users-org-filter') || {}).value || '';
  var status = (document.getElementById('users-status-filter') || {}).value || '';
  var params = '?per_page=' + USERS_PAGE_SIZE + '&page=' + usersPage;
  if (q) params += '&q=' + encodeURIComponent(q);
  if (orgId) params += '&org_id=' + encodeURIComponent(orgId);
  if (status) params += '&status=' + encodeURIComponent(status);
  authFetch('/superadmin/api/users' + params)
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var users = d.users || [];
      var total = d.total || users.length;
      var tbody = document.getElementById('tbody-users');
      if (!tbody) return;
      tbody.innerHTML = '';
      if (users.length === 0) {
        var tr = document.createElement('tr');
        tr.innerHTML = '<td colspan="7" class="empty-message">No users found</td>';
        tbody.appendChild(tr);
      } else {
        users.forEach(function(u) {
          var tr = document.createElement('tr');
          tr.style.cursor = 'pointer';
          tr.setAttribute('data-user-id', u.id);
          var st = (u.status || 'active').toLowerCase();
          tr.innerHTML = '<td>' + esc(u.name) + '</td><td>' + esc(u.email) + '</td>' +
            '<td class="user-orgs-cell" id="user-orgs-' + u.id + '"><span style="color:var(--md-sys-color-outline)">—</span></td>' +
            '<td class="badge-' + st + '">' + esc(u.status || 'active') + '</td>' +
            '<td>' + (u.is_super_admin ? '<span class="badge-super">SUPER</span>' : '') + '</td>' +
            '<td>' + esc(u.last_login_at ? u.last_login_at.substring(0, 16).replace('T', ' ') : 'Never') + '</td>' +
            '<td>' +
              (st === 'active' ? '<button class="action-btn reject user-action-btn" data-id="' + u.id + '" data-action="suspend" style="font-size:.75rem">Suspend</button>' : '') +
              (st === 'suspended' ? '<button class="action-btn approve user-action-btn" data-id="' + u.id + '" data-action="activate" style="font-size:.75rem">Activate</button>' : '') +
            '</td>';
          tbody.appendChild(tr);
          fetchUserOrgsForCell(u.id);
        });
      }
      var totalPages = Math.max(1, Math.ceil(total / USERS_PAGE_SIZE));
      var info = document.getElementById('users-page-info');
      if (info) info.textContent = 'Page ' + usersPage + ' of ' + totalPages + ' (' + total + ' users)';
      var prev = document.getElementById('users-prev');
      var next = document.getElementById('users-next');
      if (prev) prev.disabled = usersPage <= 1;
      if (next) next.disabled = usersPage >= totalPages;
    })
    .catch(function() {});
  var tbodyUsers = document.getElementById('tbody-users');
  if (tbodyUsers) tbodyUsers.addEventListener('click', function(e) {
    var btn = e.target && e.target.closest && e.target.closest('.user-action-btn');
    if (btn) {
      e.stopPropagation();
      var action = btn.dataset.action;
      authFetch('/superadmin/api/users/' + btn.dataset.id + '/' + action, { method: 'POST' })
        .then(function() { loadUsers(); });
      return;
    }
    var tr = e.target && e.target.closest && e.target.closest('tr[data-user-id]');
    if (tr && tr.dataset.userId) openUserDetail(tr.dataset.userId);
  });
}

function openUserDetail(userId) {
  var modal = document.getElementById('user-detail-modal');
  if (!modal) return;
  modal.hidden = false;
  document.getElementById('user-detail-error').hidden = true;
  document.getElementById('user-detail-pw-row').hidden = true;
  document.getElementById('user-detail-id').value = userId;
  document.getElementById('user-detail-title').textContent = 'Loading...';
  authFetch('/superadmin/api/users/' + userId)
    .then(function(r) { return r.json(); })
    .then(function(u) {
      document.getElementById('user-detail-title').textContent = u.name || 'User';
      document.getElementById('user-detail-name').value = u.name || '';
      document.getElementById('user-detail-email').value = u.email || '';
      document.getElementById('user-detail-status').value = u.status || 'active';
      document.getElementById('user-detail-superadmin').checked = !!u.is_super_admin;
      loadUserOrgs(userId);
      loadUserAddOrgDropdown();
    })
    .catch(function() { document.getElementById('user-detail-title').textContent = 'Error loading user'; });
}

function loadUserOrgs(userId) {
  var tbody = document.getElementById('user-detail-orgs');
  var empty = document.getElementById('user-detail-orgs-empty');
  if (!tbody) return;
  tbody.innerHTML = '<tr><td colspan="3" style="color:var(--md-sys-color-on-surface-variant)">Loading...</td></tr>';
  authFetch('/superadmin/api/users/' + userId + '/orgs')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var orgs = d.memberships || d.orgs || d || [];
      if (!Array.isArray(orgs)) orgs = [];
      tbody.innerHTML = '';
      if (orgs.length === 0) { empty.hidden = false; return; }
      empty.hidden = true;
      orgs.forEach(function(m) {
        var tr = document.createElement('tr');
        tr.innerHTML = '<td>' + esc(m.org_name || m.name || '') + '</td>' +
          '<td>' + esc(m.role || '') + '</td>' +
          '<td><button class="action-btn reject user-org-remove-btn" data-uid="' + userId + '" data-oid="' + (m.org_id || m.id || '') + '" style="font-size:.7rem;padding:2px 8px">Remove</button></td>';
        tbody.appendChild(tr);
      });
    })
    .catch(function() { tbody.innerHTML = ''; empty.hidden = false; });
}

function loadUserAddOrgDropdown() {
  var sel = document.getElementById('user-detail-add-org');
  if (!sel) return;
  while (sel.options.length > 1) sel.remove(1);
  authFetch('/superadmin/api/orgs?limit=200&offset=0')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      (d.orgs || d || []).forEach(function(o) {
        var opt = document.createElement('option');
        opt.value = o.id; opt.textContent = o.name + ' (' + o.slug + ')';
        sel.appendChild(opt);
      });
    }).catch(function() {});
}

(function() {
  var modal = document.getElementById('user-detail-modal');
  if (!modal) return;
  document.getElementById('user-detail-close').addEventListener('click', function() { modal.hidden = true; });
  modal.querySelector('.goal-modal-backdrop').addEventListener('click', function() { modal.hidden = true; });

  document.getElementById('user-detail-save').addEventListener('click', function() {
    var userId = document.getElementById('user-detail-id').value;
    var name = document.getElementById('user-detail-name').value.trim();
    var email = document.getElementById('user-detail-email').value.trim();
    var status = document.getElementById('user-detail-status').value;
    var isSuperAdmin = document.getElementById('user-detail-superadmin').checked;
    if (!name || !email) { var e = document.getElementById('user-detail-error'); e.textContent = 'Name and email required'; e.hidden = false; return; }
    authFetch('/superadmin/api/users/' + userId, { method: 'PATCH', headers: {'Content-Type':'application/json'}, body: JSON.stringify({name:name,email:email,status:status,is_super_admin:isSuperAdmin}) })
      .then(function(r) { return r.json().then(function(d) { return {ok:r.ok,data:d}; }); })
      .then(function(res) {
        if (!res.ok) { var e = document.getElementById('user-detail-error'); e.textContent = res.data.error || 'Update failed'; e.hidden = false; return; }
        document.getElementById('user-detail-title').textContent = name;
        document.getElementById('user-detail-error').hidden = true;
        loadUsers();
      });
  });

  document.getElementById('user-detail-reset-pw').addEventListener('click', function() {
    document.getElementById('user-detail-pw-row').hidden = false;
    document.getElementById('user-detail-new-pw').value = '';
  });
  document.getElementById('user-detail-pw-cancel').addEventListener('click', function() {
    document.getElementById('user-detail-pw-row').hidden = true;
  });
  document.getElementById('user-detail-pw-confirm').addEventListener('click', function() {
    var userId = document.getElementById('user-detail-id').value;
    var pw = document.getElementById('user-detail-new-pw').value;
    if (!pw) return;
    authFetch('/superadmin/api/users/' + userId + '/reset-password', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({new_password:pw}) })
      .then(function(r) {
        if (r.ok) { document.getElementById('user-detail-pw-row').hidden = true; alert('Password reset successfully.'); }
        else { alert('Password reset failed.'); }
      });
  });

  document.getElementById('user-detail-add-org-btn').addEventListener('click', function() {
    var userId = document.getElementById('user-detail-id').value;
    var orgId = document.getElementById('user-detail-add-org').value;
    var role = document.getElementById('user-detail-add-org-role').value;
    if (!orgId) return;
    authFetch('/superadmin/api/users/' + userId + '/orgs', { method: 'POST', headers: {'Content-Type':'application/json'}, body: JSON.stringify({org_id:orgId,role:role}) })
      .then(function() { loadUserOrgs(userId); document.getElementById('user-detail-add-org').value = ''; });
  });

  document.getElementById('user-detail-orgs').addEventListener('click', function(e) {
    var btn = e.target && e.target.closest && e.target.closest('.user-org-remove-btn');
    if (!btn) return;
    if (!confirm('Remove this user from the organization?')) return;
    authFetch('/superadmin/api/users/' + btn.dataset.uid + '/orgs/' + btn.dataset.oid, { method: 'DELETE' })
      .then(function() { loadUserOrgs(btn.dataset.uid); });
  });
})();

function fetchUserOrgsForCell(userId) {
  var cell = document.getElementById('user-orgs-' + userId);
  if (!cell) return;
  authFetch('/superadmin/api/users/' + userId + '/orgs')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var orgs = d.memberships || d.orgs || d || [];
      if (!Array.isArray(orgs) || orgs.length === 0) { cell.innerHTML = '<span style="color:var(--md-sys-color-outline)">—</span>'; return; }
      cell.innerHTML = orgs.map(function(m) {
        var name = esc(m.org_name || m.name || '?');
        var role = m.role === 'admin' ? ' <span class="badge-super" style="font-size:.65rem">admin</span>' : '';
        return '<span class="org-badge">' + name + role + '</span>';
      }).join(' ');
    })
    .catch(function() {});
}

function esc(s) { if (!s) return ''; var d = document.createElement('div'); d.textContent = s; return d.innerHTML; }
