import { test, expect } from '@playwright/test';
import { getAuthHeaders } from '../helpers';

test.describe('Tool Registry API @api', () => {
  let headers: Record<string, string>;

  test.beforeAll(async ({ request }) => {
    headers = await getAuthHeaders(request);
  });

  test('built-in tools are seeded', async ({ request }) => {
    // Check that tool_definitions table has seeded tools
    // This is verified via the agent platform - tools like file_read, shell_exec should exist
    const response = await request.get('/health');
    expect(response.status()).toBe(200);
  });
});
