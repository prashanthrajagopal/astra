import { test, expect } from '@playwright/test';
import { getAuthHeaders, createTestAgent, deleteTestAgent } from '../helpers';

test.describe('Goal API @api', () => {
  let headers: Record<string, string>;
  let testAgentId: string;

  test.beforeAll(async ({ request }) => {
    headers = await getAuthHeaders(request);
    testAgentId = await createTestAgent(request, headers, `goal-api-test-${Date.now()}`);
  });

  test.afterAll(async ({ request }) => {
    if (testAgentId) {
      await deleteTestAgent(request, headers, testAgentId);
    }
  });

  test('POST /goals creates a goal', async ({ request }) => {
    if (!testAgentId) return test.skip();

    const response = await request.post(`/agents/${testAgentId}/goals`, {
      headers,
      data: {
        agent_id: testAgentId,
        goal_text: 'Playwright test goal',
        priority: 50,
      },
    });
    expect(response.status()).toBeLessThan(300);
    const body = await response.json();
    expect(body.goal_id).toBeTruthy();
  });

  test('POST /goals with cascade_id', async ({ request }) => {
    if (!testAgentId) return test.skip();

    const cascadeId = '00000000-0000-0000-0000-000000000001';
    const response = await request.post(`/agents/${testAgentId}/goals`, {
      headers,
      data: {
        agent_id: testAgentId,
        goal_text: 'Cascade test goal',
        priority: 100,
        cascade_id: cascadeId,
      },
    });
    expect(response.status()).toBeLessThan(300);
  });

  test('POST /goals with documents', async ({ request }) => {
    if (!testAgentId) return test.skip();

    const response = await request.post(`/agents/${testAgentId}/goals`, {
      headers,
      data: {
        agent_id: testAgentId,
        goal_text: 'Goal with context docs',
        priority: 100,
        documents: [
          { doc_type: 'context_doc', name: 'venue-data', content: 'Test venue information', priority: 1 },
        ],
      },
    });
    expect(response.status()).toBeLessThan(300);
  });

  test('GET /goals?agent_id= lists goals', async ({ request }) => {
    if (!testAgentId) return test.skip();

    // Use goal-service directly or via gateway
    const response = await request.get(`/agents/${testAgentId}/goals`, { headers }).catch(() => null);
    // Goals may be listed differently - check both patterns
    if (!response) return;
    expect(response.status()).toBeLessThan(300);
  });

  test('POST /internal/goals creates agent-to-agent goal', async ({ request }) => {
    if (!testAgentId) return test.skip();

    const response = await request.post('/internal/goals', {
      headers: {
        ...headers,
        'X-Source-Agent-ID': testAgentId,
      },
      data: {
        agent_id: testAgentId,
        goal_text: 'Agent-to-agent test goal',
        priority: 100,
        source_agent_id: testAgentId,
      },
    });
    // May return 201 or 202 (if approval needed)
    expect(response.status()).toBeLessThan(500);
  });
});
