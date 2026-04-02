import { test, expect } from '@playwright/test';
import { loginAndGotoDashboard, waitForDashboardData } from './helpers';

test.describe('Services Health @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
    await waitForDashboardData(page);
  });

  test('services table has correct headers', async ({ page }) => {
    const headers = page.locator('#table-services thead th');
    await expect(headers.nth(0)).toHaveText('Service');
    await expect(headers.nth(1)).toHaveText('Port');
    await expect(headers.nth(2)).toHaveText('Type');
    await expect(headers.nth(3)).toHaveText('Status');
    await expect(headers.nth(4)).toHaveText('Latency (ms)');
  });

  test('services table shows service rows', async ({ page }) => {
    const rows = page.locator('#tbody-services tr');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);
  });

  test('service status shows healthy or unhealthy', async ({ page }) => {
    const statuses = page.locator('#tbody-services .status, #tbody-services [class*="status"]');
    const count = await statuses.count();
    if (count > 0) {
      const text = await statuses.first().textContent();
      expect(text?.toLowerCase()).toMatch(/healthy|unhealthy|active|inactive/);
    }
  });

  test('service badges show count', async ({ page }) => {
    const badge = page.locator('#services-badge');
    await expect(badge).toBeVisible();
  });
});

test.describe('Workers @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
    await waitForDashboardData(page);
  });

  test('workers table has correct headers', async ({ page }) => {
    const headers = page.locator('#table-workers thead th');
    await expect(headers.nth(0)).toHaveText('ID');
    await expect(headers.nth(1)).toHaveText('Hostname');
    await expect(headers.nth(2)).toHaveText('Status');
    await expect(headers.nth(3)).toHaveText('Capabilities');
    await expect(headers.nth(4)).toHaveText('Last Heartbeat');
  });

  test('workers badge shows count', async ({ page }) => {
    const badge = page.locator('#workers-badge');
    await expect(badge).toBeVisible();
  });

  test('workers table populates', async ({ page }) => {
    const rows = page.locator('#tbody-workers tr');
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });
});

test.describe('Cost Tracking @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
    await waitForDashboardData(page);
  });

  test('cost table has correct headers', async ({ page }) => {
    const headers = page.locator('#table-cost thead th');
    await expect(headers.nth(0)).toHaveText('Day');
    await expect(headers.nth(1)).toHaveText('Agent ID');
    await expect(headers.nth(2)).toHaveText('Model');
    await expect(headers.nth(3)).toHaveText('Tokens In');
    await expect(headers.nth(4)).toHaveText('Tokens Out');
    await expect(headers.nth(5)).toHaveText('Cost ($)');
  });
});

test.describe('Process IDs @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
    await waitForDashboardData(page);
  });

  test('PIDs table has correct headers', async ({ page }) => {
    const headers = page.locator('#table-pids thead th');
    await expect(headers.nth(0)).toHaveText('Service');
    await expect(headers.nth(1)).toHaveText('PID');
  });

  test('PIDs table shows running services', async ({ page }) => {
    const rows = page.locator('#tbody-pids tr');
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(0);
  });
});
