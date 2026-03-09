function setStatus(msg, isError) {
  const meta = document.getElementById('dashboard-meta');
  const status = document.getElementById('refresh-status');
  status.textContent = msg;
  meta.classList.toggle('has-error', !!isError);
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

function pick(obj, keys, fallback = '') {
  for (const k of keys) {
    if (obj && obj[k] !== undefined && obj[k] !== null) return obj[k];
  }
  return fallback;
}

function renderServices(services) {
  const tbody = document.getElementById('tbody-services');
  tbody.innerHTML = '';
  if (!services || services.length === 0) return tbody.appendChild(emptyRow(5, 'No services configured'));
  services.forEach((s) => {
    const tr = document.createElement('tr');
    tr.className = 'tr-service';
    tr.dataset.serviceName = s.name || '';
    const latencyClass = Number(s.latency_ms) > 10 ? 'latency-warning' : '';
    tr.innerHTML = `<td>${s.name || ''}</td><td>${s.port || ''}</td><td>${s.type || ''}</td><td class="td-status ${s.healthy ? 'status-healthy' : 'status-unhealthy'}">${s.healthy ? 'healthy' : 'unhealthy'}</td><td class="${latencyClass}">${s.latency_ms ?? ''}</td>`;
    tbody.appendChild(tr);
  });
}

function renderWorkers(workers) {
  const tbody = document.getElementById('tbody-workers');
  tbody.innerHTML = '';
  if (!workers || workers.length === 0) return tbody.appendChild(emptyRow(5, 'No workers registered'));
  workers.forEach((w) => {
    const tr = document.createElement('tr');
    tr.className = 'tr-worker';
    const id = pick(w, ['id', 'ID']);
    const hostname = pick(w, ['hostname', 'Hostname']);
    const statusText = pick(w, ['status', 'Status']);
    const capabilities = pick(w, ['capabilities', 'Capabilities'], []);
    const lastHeartbeat = pick(w, ['last_heartbeat', 'LastHeartbeat', 'lastHeartbeat']);
    const status = statusText.toString().toLowerCase();
    const cls = status === 'active' ? 'status-active' : (status ? 'status-inactive' : 'status-stale');
    tr.innerHTML = `<td>${id}</td><td>${hostname}</td><td class="td-status ${cls}">${statusText}</td><td>${Array.isArray(capabilities) ? capabilities.join(',') : ''}</td><td>${lastHeartbeat}</td>`;
    tbody.appendChild(tr);
  });
}

function renderApprovals(items) {
  const tbody = document.getElementById('tbody-approvals');
  tbody.innerHTML = '';
  if (!items || items.length === 0) return tbody.appendChild(emptyRow(6, 'No pending approvals'));
  items.forEach((a) => {
    const tr = document.createElement('tr');
    tr.className = 'tr-approval';
    tr.dataset.approvalId = a.id || '';
    const st = (a.status || 'pending').toString().toLowerCase();
    tr.innerHTML = `<td>${a.id || ''}</td><td>${a.tool_name || ''}</td><td>${a.action_summary || ''}</td><td class="td-status status-${st}">${a.status || ''}</td><td>${a.requested_at || ''}</td><td><button class="action-btn approve" data-action="approve" data-id="${a.id || ''}">Approve</button><button class="action-btn reject" data-action="reject" data-id="${a.id || ''}">Reject</button></td>`;
    tbody.appendChild(tr);
  });
}

function renderCost(cost) {
  const tbody = document.getElementById('tbody-cost');
  tbody.innerHTML = '';
  const rows = (cost && Array.isArray(cost.rows)) ? cost.rows : [];
  if (rows.length === 0) return tbody.appendChild(emptyRow(6, 'No cost data'));
  rows.forEach((r) => {
    const tr = document.createElement('tr');
    tr.className = 'tr-cost';
    tr.innerHTML = `<td>${r.day || ''}</td><td>${r.agent_id || ''}</td><td>${r.model || ''}</td><td>${r.tokens_in ?? ''}</td><td>${r.tokens_out ?? ''}</td><td>${r.cost_dollars ?? ''}</td>`;
    tbody.appendChild(tr);
  });
}

function renderLogs(logs) {
  const container = document.getElementById('logs-container');
  container.innerHTML = '';
  const names = logs ? Object.keys(logs) : [];
  if (names.length === 0) {
    const pre = document.createElement('pre');
    pre.className = 'empty-message';
    pre.textContent = 'No logs available';
    container.appendChild(pre);
    return;
  }
  names.sort().forEach((name) => {
    const block = document.createElement('div');
    block.className = 'log-block';
    const title = document.createElement('h3');
    title.className = 'log-block-title';
    title.textContent = `${name} (last 20 lines)`;
    const pre = document.createElement('pre');
    pre.className = 'log-block-content';
    const lines = Array.isArray(logs[name]) ? logs[name] : [];
    pre.textContent = lines.length > 0 ? lines.join('\n') : 'No logs available';
    block.appendChild(title);
    block.appendChild(pre);
    container.appendChild(block);
  });
}

function renderPids(pids) {
  const tbody = document.getElementById('tbody-pids');
  tbody.innerHTML = '';
  const names = pids ? Object.keys(pids) : [];
  if (names.length === 0) return tbody.appendChild(emptyRow(2, 'No PID data'));
  names.sort().forEach((name) => {
    const tr = document.createElement('tr');
    tr.className = 'tr-pid';
    tr.innerHTML = `<td>${name}</td><td>${pids[name]}</td>`;
    tbody.appendChild(tr);
  });
}

let inFlight = false;
let approvalActionInFlight = false;

async function submitApprovalAction(id, action) {
  if (!id || approvalActionInFlight) return;
  approvalActionInFlight = true;
  setStatus(`Submitting ${action} for ${id}`, false);
  try {
    const res = await fetch(`/api/dashboard/approvals/${encodeURIComponent(id)}/${action}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ decided_by: 'dashboard-ui' })
    });
    if (!res.ok) throw new Error(`status ${res.status}`);
    await fetchSnapshot();
  } catch (e) {
    setStatus(`Error: ${e.message || e}`, true);
  } finally {
    approvalActionInFlight = false;
  }
}

async function fetchSnapshot() {
  if (inFlight) return;
  inFlight = true;
  setStatus('Refreshing', false);
  try {
    const res = await fetch('/api/dashboard/snapshot', { cache: 'no-store' });
    if (!res.ok) throw new Error(`status ${res.status}`);
    const d = await res.json();
    renderServices(d.services || []);
    renderWorkers(d.workers || []);
    renderApprovals(d.approvals || []);
    renderCost(d.cost || { rows: [] });
    renderLogs(d.logs || {});
    renderPids(d.pids || {});
    document.getElementById('last-updated').textContent = `Last updated: ${new Date().toISOString()}`;
    setStatus('Idle', false);
  } catch (e) {
    setStatus(`Error: ${e.message || e}`, true);
  } finally {
    inFlight = false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('btn-refresh').addEventListener('click', fetchSnapshot);
  document.getElementById('tbody-approvals').addEventListener('click', (e) => {
    const t = e.target;
    if (!(t instanceof HTMLElement)) return;
    if (!t.dataset || !t.dataset.action || !t.dataset.id) return;
    submitApprovalAction(t.dataset.id, t.dataset.action);
  });
  fetchSnapshot();
  setInterval(fetchSnapshot, 5000);
});
