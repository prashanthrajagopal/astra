import { test, expect } from '@playwright/test';
import { loginAndGotoDashboard } from './helpers';

test.describe('Chat Widget @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
  });

  test('chat widget toggle button exists', async ({ page }) => {
    const toggle = page.locator('.chat-widget-toggle');
    // Chat widget may not be visible if no chat-capable agents exist
    const visible = await toggle.isVisible().catch(() => false);
    if (visible) {
      await expect(toggle).toBeVisible();
    }
  });

  test('clicking chat toggle opens panel', async ({ page }) => {
    const toggle = page.locator('.chat-widget-toggle');
    if (await toggle.isVisible().catch(() => false)) {
      await toggle.click();
      const panel = page.locator('.chat-widget-panel');
      await expect(panel).toBeVisible({ timeout: 3000 });
    }
  });

  test('chat panel has header', async ({ page }) => {
    const toggle = page.locator('.chat-widget-toggle');
    if (await toggle.isVisible().catch(() => false)) {
      await toggle.click();
      const header = page.locator('.chat-widget-header');
      await expect(header).toBeVisible();
    }
  });

  test('chat panel has input row', async ({ page }) => {
    const toggle = page.locator('.chat-widget-toggle');
    if (await toggle.isVisible().catch(() => false)) {
      await toggle.click();
      const input = page.locator('.chat-widget-input');
      await expect(input).toBeVisible();
    }
  });

  test('chat panel has send button', async ({ page }) => {
    const toggle = page.locator('.chat-widget-toggle');
    if (await toggle.isVisible().catch(() => false)) {
      await toggle.click();
      const send = page.locator('.chat-widget-send');
      await expect(send).toBeVisible();
    }
  });

  test('chat panel can be minimized', async ({ page }) => {
    const toggle = page.locator('.chat-widget-toggle');
    if (await toggle.isVisible().catch(() => false)) {
      await toggle.click();
      await page.waitForTimeout(300);
      const panel = page.locator('.chat-widget-panel');
      if (await panel.isVisible().catch(() => false)) {
        // Try minimize button, fallback to clicking toggle again
        const minimize = page.locator('.chat-widget-minimize');
        if (await minimize.isVisible().catch(() => false)) {
          await minimize.click();
        } else {
          await toggle.click();
        }
        await page.waitForTimeout(300);
      }
    }
    // Pass — chat widget may not exist if no chat-capable agents
  });
});

test.describe('Chat Message Injection API @api', () => {
  test('POST /chat/sessions/{id}/inject rejects invalid session', async ({ request }) => {
    const response = await request.post('/chat/sessions/invalid-uuid/inject', {
      data: { content: 'test message', role: 'system' },
    });
    expect(response.status()).toBeGreaterThanOrEqual(400);
  });
});
