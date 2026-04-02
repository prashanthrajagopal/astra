import { test, expect } from '@playwright/test';

test.describe('Health Endpoints @api', () => {
  test('GET /health returns 200', async ({ request }) => {
    const response = await request.get('/health');
    expect(response.status()).toBe(200);
    expect(await response.text()).toBe('ok');
  });

  test('GET /ready returns 200 when healthy', async ({ request }) => {
    const response = await request.get('/ready');
    expect(response.status()).toBe(200);
  });
});
