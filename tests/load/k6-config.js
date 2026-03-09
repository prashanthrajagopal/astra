import http from 'k6/http';
import { check, sleep } from 'k6';

const baseUrl = __ENV.BASE_URL || 'http://localhost:8080';
const token = __ENV.JWT_TOKEN || '';

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-vus',
      vus: 5,
      duration: '30s'
    }
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<500']
  }
};

function authHeaders() {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  return headers;
}

export default function () {
  const health = http.get(`${baseUrl}/health`);
  check(health, { 'health is 200': (r) => r.status === 200 });

  const spawn = http.post(
    `${baseUrl}/agents`,
    JSON.stringify({ actor_type: 'k6-agent', config: '{}' }),
    { headers: authHeaders() }
  );

  check(spawn, {
    'spawn returns 200/201': (r) => r.status === 200 || r.status === 201
  });

  sleep(1);
}
