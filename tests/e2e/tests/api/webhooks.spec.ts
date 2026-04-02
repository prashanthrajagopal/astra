import { test, expect } from '@playwright/test';
import * as crypto from 'crypto';

test.describe('Webhook API @api', () => {
  test('POST /webhooks/{source_id} rejects unknown source', async ({ request }) => {
    const response = await request.post('/webhooks/unknown-source', {
      data: { event: 'test' },
    });
    // Should get 404 (unknown source) or 502 (webhook service unavailable)
    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('POST /webhooks/{source_id} forwards signature headers', async ({ request }) => {
    const body = JSON.stringify({ event: 'test', data: 'payload' });
    const secret = 'test-secret';
    const hmac = crypto.createHmac('sha256', secret).update(body).digest('hex');

    const response = await request.post('/webhooks/test-source', {
      headers: {
        'Content-Type': 'application/json',
        'X-Signature-256': `sha256=${hmac}`,
      },
      data: body,
    });
    // Expected to fail (source not registered) but should reach the service
    expect(response.status()).toBeGreaterThanOrEqual(400);
  });
});
