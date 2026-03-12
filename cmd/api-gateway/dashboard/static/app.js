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
  if (!recentGoals || recentGoals.length === 0) return tbody.appendChild(emptyRow(5, 'No goals yet'));
  recentGoals.forEach(function (g) {
    var tr = document.createElement('tr');
    tr.setAttribute('data-goal-id', g.id || '');
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
  modal.hidden = false;
  body.innerHTML = '<p class="goal-modal-loading">Loading…</p>';
  fetch('/api/dashboard/goals/' + encodeURIComponent(goalId), { cache: 'no-store' })
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
      html += '<div class="goal-detail-task' + statusClass + clickableClass + '"' + resultAttr + ' data-task-type="' + escapeHtml(t.type || '') + '">';
      html += '<strong>' + escapeHtml(t.type || 'task') + '</strong> &middot; ' + escapeHtml(t.status || '') + ' (updated: ' + escapeHtml(t.updated_at || '') + ')' + (isCodeGen ? ' <span class="goal-detail-task-hint">— click to view code</span>' : '') + '';
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
  fetch('/api/dashboard/approvals/' + encodeURIComponent(approvalId), { cache: 'no-store' })
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

// ─── Chat ───────────────────────────────────────────────────────────────

var currentChatSessionId = null;

function fetchChatSessions() {
  fetch('/api/dashboard/chat/sessions', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderChatSessions(d.sessions || []);
    })
    .catch(function (e) {
      renderChatSessions([]);
      var statusEl = document.getElementById('chat-status');
      if (statusEl) statusEl.textContent = 'Sessions unavailable';
    });
}

function renderChatSessions(sessions) {
  var list = document.getElementById('chat-sessions-list');
  if (!list) return;
  list.innerHTML = '';
  if (!sessions || sessions.length === 0) {
    list.innerHTML = '<div class="chat-sessions-empty">No sessions yet. Click "New chat" to start.</div>';
    return;
  }
  sessions.forEach(function (s) {
    var div = document.createElement('div');
    div.className = 'chat-session-item' + (currentChatSessionId === s.id ? ' active' : '');
    div.setAttribute('data-session-id', s.id);
    var title = (s.title || '').trim() || ('Session ' + (s.id || '').substring(0, 8));
    div.innerHTML = '<span class="chat-session-title">' + escapeHtml(title) + '</span><span class="chat-session-meta">' + escapeHtml(s.agent_id ? s.agent_id.substring(0, 8) : '') + '</span>';
    list.appendChild(div);
  });
}

function openChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  var loading = document.getElementById('chat-new-loading');
  var list = document.getElementById('chat-new-agent-list');
  var empty = document.getElementById('chat-new-empty');
  if (!modal) return;
  modal.hidden = false;
  if (loading) loading.hidden = false;
  if (list) list.hidden = true;
  if (empty) empty.hidden = true;
  fetch('/api/dashboard/chat/agents', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      var agents = d.agents || [];
      if (loading) loading.hidden = true;
      if (agents.length === 0) {
        if (empty) empty.hidden = false;
        return;
      }
      if (list) {
        list.hidden = false;
        list.innerHTML = '';
        agents.forEach(function (a) {
          var btn = document.createElement('button');
          btn.type = 'button';
          btn.className = 'chat-agent-btn';
          btn.setAttribute('data-agent-id', a.id || '');
          btn.textContent = a.name || a.id || 'Agent';
          list.appendChild(btn);
        });
      }
    })
    .catch(function (e) {
      if (loading) loading.textContent = 'Error: ' + (e.message || e);
    });
}

function closeChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  if (modal) modal.hidden = true;
}

function createChatSession(agentId) {
  fetch('/api/dashboard/chat/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agent_id: agentId, title: '' })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      closeChatNewModal();
      fetchChatSessions();
      selectChatSession(d.id);
    })
    .catch(function (e) {
      setStatus('Chat create failed: ' + (e.message || e), true);
    });
}

function selectChatSession(sessionId) {
  currentChatSessionId = sessionId;
  var placeholder = document.getElementById('chat-placeholder');
  var view = document.getElementById('chat-view');
  var list = document.getElementById('chat-sessions-list');
  if (placeholder) placeholder.hidden = true;
  if (view) view.hidden = false;
  if (list) {
    [].forEach.call(list.querySelectorAll('.chat-session-item'), function (el) {
      el.classList.toggle('active', el.getAttribute('data-session-id') === sessionId);
    });
  }
  loadChatMessages(sessionId);
}

function loadChatMessages(sessionId) {
  var container = document.getElementById('chat-messages');
  if (!container) return;
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderChatMessages(d.messages || []);
    })
    .catch(function (e) {
      container.innerHTML = '<div class="chat-message chat-message-error">Failed to load messages.</div>';
    });
}

function renderChatMessages(messages) {
  var container = document.getElementById('chat-messages');
  if (!container) return;
  container.innerHTML = '';
  if (!messages || messages.length === 0) {
    container.innerHTML = '<div class="chat-message chat-message-system">No messages yet. Type below to send.</div>';
    return;
  }
  messages.forEach(function (m) {
    var div = document.createElement('div');
    div.className = 'chat-message chat-message-' + (m.role || 'user');
    div.innerHTML = '<div class="chat-message-role">' + escapeHtml(m.role || '') + '</div><div class="chat-message-content">' + escapeHtml(m.content || '') + '</div><div class="chat-message-meta">' + escapeHtml(m.created_at || '') + '</div>';
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
  var container = document.getElementById('chat-messages');
  var tempDiv = document.createElement('div');
  tempDiv.className = 'chat-message chat-message-user chat-message-pending';
  tempDiv.innerHTML = '<div class="chat-message-role">user</div><div class="chat-message-content">' + escapeHtml(content) + '</div>';
  if (container) {
    var sys = container.querySelector('.chat-message-system');
    if (sys) sys.remove();
    container.appendChild(tempDiv);
    container.scrollTop = container.scrollHeight;
  }
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: content })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      tempDiv.classList.remove('chat-message-pending');
      loadChatMessages(sessionId);
    })
    .catch(function (e) {
      tempDiv.innerHTML += '<div class="chat-message-error">Send failed: ' + escapeHtml(e.message || e) + '</div>';
      tempDiv.classList.remove('chat-message-pending');
    })
    .finally(function () {
      input.disabled = false;
    });
}

// ─── Chat ─────────────────────────────────────────────────────────────

var currentChatSessionId = null;

function fetchChatSessions() {
  fetch('/api/dashboard/chat/sessions', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderChatSessions(d.sessions || []);
    })
    .catch(function (e) {
      renderChatSessions([]);
      var statusEl = document.getElementById('chat-status');
      if (statusEl) statusEl.textContent = 'Chat unavailable: ' + (e.message || e);
    });
}

function renderChatSessions(sessions) {
  var list = document.getElementById('chat-sessions-list');
  var statusEl = document.getElementById('chat-status');
  if (!list) return;
  if (statusEl) statusEl.textContent = '';
  list.innerHTML = '';
  if (!sessions || sessions.length === 0) {
    list.innerHTML = '<p class="chat-empty-hint">No sessions yet. Click New chat to start.</p>';
    return;
  }
  sessions.forEach(function (s) {
    var div = document.createElement('div');
    div.className = 'chat-session-item' + (currentChatSessionId === s.id ? ' chat-session-active' : '');
    div.setAttribute('data-session-id', s.id);
    div.setAttribute('data-agent-id', s.agent_id || '');
    var title = (s.title || '').trim() || ('Session ' + (s.id || '').substring(0, 8));
    var updated = s.updated_at || s.created_at || '';
    div.innerHTML = '<span class="chat-session-title">' + escapeHtml(title) + '</span><span class="chat-session-meta">' + escapeHtml(updated) + '</span>';
    list.appendChild(div);
  });
}

function openChatSession(sessionId) {
  currentChatSessionId = sessionId;
  renderChatSessions(document.querySelectorAll('#chat-sessions-list .chat-session-item') ? [] : []);
  var placeholder = document.getElementById('chat-placeholder');
  var view = document.getElementById('chat-view');
  if (placeholder) placeholder.hidden = true;
  if (view) view.hidden = false;
  fetchChatMessages(sessionId);
  var items = document.querySelectorAll('.chat-session-item');
  items.forEach(function (el) {
    el.classList.toggle('chat-session-active', el.getAttribute('data-session-id') === sessionId);
  });
}

function fetchChatMessages(sessionId) {
  if (!sessionId) return;
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderChatMessages(d.messages || []);
    })
    .catch(function (e) {
      renderChatMessages([]);
      setStatus('Chat messages load failed: ' + (e.message || e), true);
    });
}

function renderChatMessages(messages) {
  var cont = document.getElementById('chat-messages');
  if (!cont) return;
  cont.innerHTML = '';
  (messages || []).forEach(function (m) {
    var div = document.createElement('div');
    div.className = 'chat-message chat-message-' + (m.role || 'user');
    var role = (m.role || 'user');
    var content = escapeHtml((m.content || '').trim() || '(empty)');
    div.innerHTML = '<span class="chat-message-role">' + escapeHtml(role) + '</span><div class="chat-message-content">' + content.replace(/\n/g, '<br>') + '</div>';
    cont.appendChild(div);
  });
  cont.scrollTop = cont.scrollHeight;
}

function sendChatMessage(content) {
  if (!currentChatSessionId || !content || !content.trim()) return;
  var input = document.getElementById('chat-input');
  if (input) input.disabled = true;
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(currentChatSessionId) + '/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: content.trim() })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return fetchChatMessages(currentChatSessionId);
    })
    .catch(function (e) {
      setStatus('Send failed: ' + (e.message || e), true);
    })
    .finally(function () {
      if (input) input.disabled = false;
    });
}

function openChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  var loading = document.getElementById('chat-new-loading');
  var list = document.getElementById('chat-new-agent-list');
  var empty = document.getElementById('chat-new-empty');
  if (!modal) return;
  modal.hidden = false;
  if (loading) loading.hidden = false;
  if (list) { list.hidden = true; list.innerHTML = ''; }
  if (empty) empty.hidden = true;
  fetch('/api/dashboard/chat/agents', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      var agents = d.agents || [];
      if (loading) loading.hidden = true;
      if (agents.length === 0) {
        if (empty) empty.hidden = false;
      } else {
        if (list) {
          list.hidden = false;
          agents.forEach(function (a) {
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'chat-agent-btn';
            btn.setAttribute('data-agent-id', a.id || '');
            btn.textContent = a.name || a.id || 'Agent';
            list.appendChild(btn);
          });
        }
      }
    })
    .catch(function (e) {
      if (loading) loading.textContent = 'Error: ' + (e.message || e);
    });
}

function closeChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  if (modal) modal.hidden = true;
}

function createChatSession(agentId) {
  closeChatNewModal();
  fetch('/api/dashboard/chat/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agent_id: agentId, title: '' })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      fetchChatSessions();
      if (d.id) openChatSession(d.id);
    })
    .catch(function (e) {
      setStatus('Create session failed: ' + (e.message || e), true);
    });
}

// ─── Chat ─────────────────────────────────────────────────────────────

var currentChatSessionId = null;
var chatSessionsList = [];

function fetchChatSessions() {
  fetch('/api/dashboard/chat/sessions', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      chatSessionsList = d.sessions || [];
      renderChatSessions();
    })
    .catch(function () {
      chatSessionsList = [];
      renderChatSessions();
    });
}

function renderChatSessions() {
  var listEl = document.getElementById('chat-sessions-list');
  if (!listEl) return;
  listEl.innerHTML = '';
  if (chatSessionsList.length === 0) {
    listEl.innerHTML = '<p class="chat-empty-hint">No sessions. Click "New chat" to start.</p>';
    return;
  }
  chatSessionsList.forEach(function (s) {
    var title = (s.title || '').toString() || 'Session ' + (s.id || '').substring(0, 8);
    var item = document.createElement('div');
    item.className = 'chat-session-item' + (currentChatSessionId === (s.id || '') ? ' active' : '');
    item.setAttribute('data-session-id', s.id || '');
    item.setAttribute('data-agent-id', s.agent_id || '');
    item.innerHTML = '<span class="chat-session-title">' + escapeHtml(title) + '</span><span class="chat-session-meta">' + escapeHtml((s.updated_at || '').toString().substring(0, 16)) + '</span>';
    listEl.appendChild(item);
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
  fetch('/api/dashboard/chat/agents', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      var agents = d.agents || [];
      if (loadingEl) loadingEl.hidden = true;
      if (agents.length === 0) {
        if (emptyEl) emptyEl.hidden = false;
        return;
      }
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
    })
    .catch(function () {
      if (loadingEl) loadingEl.hidden = true;
      if (emptyEl) { emptyEl.hidden = false; emptyEl.textContent = 'Failed to load agents.'; }
    });
}

function closeChatNewModal() {
  var modal = document.getElementById('chat-new-modal');
  if (modal) modal.hidden = true;
}

function createChatSession(agentId, agentName) {
  closeChatNewModal();
  fetch('/api/dashboard/chat/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ agent_id: agentId, title: agentName || '' })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      fetchChatSessions();
      selectChatSession(d.id);
    })
    .catch(function (e) {
      setStatus('Chat create failed: ' + (e.message || e), true);
    });
}

function selectChatSession(sessionId) {
  currentChatSessionId = sessionId;
  renderChatSessions();
  var placeholder = document.getElementById('chat-placeholder');
  var view = document.getElementById('chat-view');
  if (placeholder) placeholder.hidden = true;
  if (view) { view.hidden = false; }
  fetchChatMessages(sessionId);
}

function fetchChatMessages(sessionId) {
  var messagesEl = document.getElementById('chat-messages');
  if (!messagesEl) return;
  messagesEl.innerHTML = '<p class="chat-loading">Loading…</p>';
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', { cache: 'no-store' })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      return res.json();
    })
    .then(function (d) {
      renderChatMessages(d.messages || []);
    })
    .catch(function () {
      messagesEl.innerHTML = '<p class="chat-error">Failed to load messages.</p>';
    });
}

function renderChatMessages(messages) {
  var messagesEl = document.getElementById('chat-messages');
  if (!messagesEl) return;
  messagesEl.innerHTML = '';
  messages.forEach(function (m) {
    var div = document.createElement('div');
    div.className = 'chat-message chat-message-' + (m.role || 'user');
    div.innerHTML = '<div class="chat-message-role">' + escapeHtml(m.role || '') + '</div><div class="chat-message-content">' + escapeHtml(m.content || '') + '</div><div class="chat-message-meta">' + escapeHtml((m.created_at || '').toString()) + '</div>';
    messagesEl.appendChild(div);
  });
  messagesEl.scrollTop = messagesEl.scrollHeight;
}

function sendChatMessage() {
  var sessionId = currentChatSessionId;
  var input = document.getElementById('chat-input');
  if (!sessionId || !input) return;
  var content = (input.value || '').trim();
  if (!content) return;
  input.value = '';
  var messagesEl = document.getElementById('chat-messages');
  var div = document.createElement('div');
  div.className = 'chat-message chat-message-user';
  div.innerHTML = '<div class="chat-message-role">user</div><div class="chat-message-content">' + escapeHtml(content) + '</div><div class="chat-message-meta">Sending…</div>';
  messagesEl.appendChild(div);
  messagesEl.scrollTop = messagesEl.scrollHeight;

  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content: content })
  })
    .then(function (res) {
      if (!res.ok) throw new Error('status ' + res.status);
      fetchChatMessages(sessionId);
    })
    .catch(function (e) {
      var meta = div.querySelector('.chat-message-meta');
      if (meta) meta.textContent = 'Failed: ' + (e.message || e);
    });
}

function loadSettings() {
  fetch('/api/dashboard/settings', { cache: 'no-store' })
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
  fetch('/api/dashboard/chat/sessions', { cache: 'no-store' })
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
  fetch('/api/dashboard/chat/agents', { cache: 'no-store' })
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
  fetch('/api/dashboard/chat/sessions', {
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
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', { cache: 'no-store' })
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
  fetch('/api/dashboard/chat/sessions/' + encodeURIComponent(sessionId) + '/messages', {
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
      var tr = e.target && e.target.closest && e.target.closest('tr[data-goal-id]');
      if (tr && tr.dataset && tr.dataset.goalId) openGoalModal(tr.dataset.goalId);
    });
  }
  var goalModal = document.getElementById('goal-modal');
  if (goalModal) {
    document.getElementById('goal-modal-close').addEventListener('click', closeGoalModal);
    goalModal.querySelector('.goal-modal-backdrop').addEventListener('click', closeGoalModal);
    goalModal.addEventListener('click', function (e) {
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
  fetchChatSessions();
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
        fetch('/api/dashboard/agents/' + encodeURIComponent(agentId) + '/status', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: 'active' }) })
          .then(function (r) { if (r.ok) fetchSnapshot(); else r.text().then(function (t) { setStatus('Agent enable failed: ' + t, true); }); });
      } else if (action === 'disable') {
        fetch('/api/dashboard/agents/' + encodeURIComponent(agentId) + '/status', { method: 'PATCH', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ status: 'stopped' }) })
          .then(function (r) { if (r.ok) fetchSnapshot(); else r.text().then(function (t) { setStatus('Agent disable failed: ' + t, true); }); });
      } else if (action === 'delete') {
        if (!confirm('Delete this agent and all its goals and tasks?')) return;
        fetch('/api/dashboard/agents/' + encodeURIComponent(agentId), { method: 'DELETE' })
          .then(function (r) { if (r.ok) fetchSnapshot(); else r.text().then(function (t) { setStatus('Agent delete failed: ' + t, true); }); });
      }
    });
  }
  var chatNewBtn = document.getElementById('chat-new-btn');
  if (chatNewBtn) chatNewBtn.addEventListener('click', openChatNewModal);
  var chatSendBtn = document.getElementById('chat-send-btn');
  if (chatSendBtn) chatSendBtn.addEventListener('click', sendChatMessage);
  var chatSessionsListEl = document.getElementById('chat-sessions-list');
  if (chatSessionsListEl) {
    chatSessionsListEl.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-session-btn');
      if (btn && btn.dataset && btn.dataset.sessionId) selectChatSession(btn.dataset.sessionId, btn.dataset.agentId || '');
    });
  }
  var chatAgentListEl = document.getElementById('chat-new-agent-list');
  if (chatAgentListEl) {
    chatAgentListEl.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-agent-btn');
      if (btn && btn.dataset && btn.dataset.agentId) createChatSession(btn.dataset.agentId, btn.dataset.agentName || '');
    });
  }
  var chatNewModalClose = document.getElementById('chat-new-modal-close');
  var chatNewModalBackdrop = document.getElementById('chat-new-modal-backdrop');
  if (chatNewModalClose) chatNewModalClose.addEventListener('click', closeChatNewModal);
  if (chatNewModalBackdrop) chatNewModalBackdrop.addEventListener('click', closeChatNewModal);
  var chatInput = document.getElementById('chat-input');
  if (chatInput) {
    chatInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChatMessage(); }
    });
  }
  fetchChatSessions();
  fetchChatSessions();
  var chatNewBtn = document.getElementById('chat-new-btn');
  if (chatNewBtn) chatNewBtn.addEventListener('click', openChatNewModal);
  var chatSendBtn = document.getElementById('chat-send-btn');
  if (chatSendBtn) chatSendBtn.addEventListener('click', sendChatMessage);
  var chatSessionsListEl = document.getElementById('chat-sessions-list');
  if (chatSessionsListEl) {
    chatSessionsListEl.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-session-btn');
      if (btn && btn.dataset && btn.dataset.sessionId) selectChatSession(btn.dataset.sessionId, btn.dataset.agentId || '');
    });
  }
  var chatAgentList = document.getElementById('chat-new-agent-list');
  if (chatAgentList) {
    chatAgentList.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-agent-btn');
      if (btn && btn.dataset && btn.dataset.agentId) createChatSession(btn.dataset.agentId, btn.dataset.agentName || '');
    });
  }
  var chatNewClose = document.getElementById('chat-new-modal-close');
  var chatNewBackdrop = document.getElementById('chat-new-modal-backdrop');
  if (chatNewClose) chatNewClose.addEventListener('click', closeChatNewModal);
  if (chatNewBackdrop) chatNewBackdrop.addEventListener('click', closeChatNewModal);
  var chatInput = document.getElementById('chat-input');
  if (chatInput) {
    chatInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChatMessage(); }
    });
  }
  var chatNewBtn = document.getElementById('chat-new-btn');
  if (chatNewBtn) chatNewBtn.addEventListener('click', openChatNewModal);
  var chatSendBtn = document.getElementById('chat-send-btn');
  if (chatSendBtn) chatSendBtn.addEventListener('click', sendChatMessage);
  var chatSessionsListEl = document.getElementById('chat-sessions-list');
  if (chatSessionsListEl) {
    chatSessionsListEl.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-session-btn');
      if (btn && btn.dataset && btn.dataset.sessionId) selectChatSession(btn.dataset.sessionId, btn.dataset.agentId || '');
    });
  }
  var chatNewAgentList = document.getElementById('chat-new-agent-list');
  if (chatNewAgentList) {
    chatNewAgentList.addEventListener('click', function (e) {
      var btn = e.target && e.target.closest && e.target.closest('.chat-agent-btn');
      if (btn && btn.dataset && btn.dataset.agentId) createChatSession(btn.dataset.agentId, btn.dataset.agentName || '');
    });
  }
  var chatNewModalClose = document.getElementById('chat-new-modal-close');
  var chatNewModalBackdrop = document.getElementById('chat-new-modal-backdrop');
  if (chatNewModalClose) chatNewModalClose.addEventListener('click', closeChatNewModal);
  if (chatNewModalBackdrop) chatNewModalBackdrop.addEventListener('click', closeChatNewModal);
  var chatInput = document.getElementById('chat-input');
  if (chatInput) {
    chatInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); sendChatMessage(); }
    });
  }
  fetchChatSessions();
  fetchSnapshot();
  setInterval(fetchSnapshot, 5000);
});
