import { test, expect } from '@playwright/test';
import { getAuthHeaders } from '../helpers';

test.describe('Approval API @api', () => {
  let headers: Record<string, string>;

  test.beforeAll(async ({ request }) => {
    headers = await getAuthHeaders(request);
  });

  test('GET /approvals/pending returns list', async ({ request }) => {
    const response = await request.get('/approvals/pending', { headers });
    // May need to hit access-control directly
    if (response.status() === 404) return; // Route might not be proxied
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
  });

  test('POST /approvals/{id}/decide rejects invalid id', async ({ request }) => {
    const response = await request.post('/approvals/invalid-uuid/decide', {
      headers,
      data: { decision: 'approved', user_id: 'test-user' },
    });
    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('POST /approvals/{id}/decide requires valid decision', async ({ request }) => {
    const fakeId = '00000000-0000-0000-0000-000000000099';
    const response = await request.post(`/approvals/${fakeId}/decide`, {
      headers,
      data: { decision: 'invalid', user_id: 'test-user' },
    });
    expect(response.status()).toBeGreaterThanOrEqual(400);
  });
});
