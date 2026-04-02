import { test, expect } from '@playwright/test';
import { loginAndGotoDashboard, waitForDashboardData, switchTab } from './helpers';

test.describe('Dashboard Overview @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
  });

  // --- Navigation ---
  test('dashboard loads with correct title', async ({ page }) => {
    await expect(page).toHaveTitle('Astra Platform Dashboard');
  });

  test('topnav displays logo and navigation', async ({ page }) => {
    await expect(page.locator('.logo')).toContainText('Astra');
    await expect(page.locator('.logo')).toContainText('Platform');
    await expect(page.locator('.nav-tab[data-tab="overview"]')).toBeVisible();
    await expect(page.locator('.nav-tab[data-tab="slack"]')).toBeVisible();
  });

  test('overview tab is active by default', async ({ page }) => {
    await expect(page.locator('.nav-tab[data-tab="overview"]')).toHaveClass(/active/);
  });

  test('can switch to Slack tab', async ({ page }) => {
    await switchTab(page, 'slack');
    await expect(page.locator('.nav-tab[data-tab="slack"]')).toHaveClass(/active/);
    await expect(page.locator('#tab-slack')).toBeVisible();
    await expect(page.locator('#tab-overview')).toBeHidden();
  });

  test('can switch back to Overview tab', async ({ page }) => {
    await switchTab(page, 'slack');
    await switchTab(page, 'overview');
    await expect(page.locator('.nav-tab[data-tab="overview"]')).toHaveClass(/active/);
    await expect(page.locator('#tab-overview')).toBeVisible();
  });

  // --- Stats Grid ---
  test('stats grid displays all stat cards', async ({ page }) => {
    const statCards = page.locator('.stats-grid .stat-card');
    await expect(statCards).toHaveCount(9);
  });

  test('stat cards show correct labels', async ({ page }) => {
    const labels = [
      'Active Goals', 'Completed Tasks', 'Running Tasks',
      'Failed Goals', 'Active Workers', 'Pending Approvals',
      'Agents', 'Tokens In', 'Tokens Out'
    ];
    for (const label of labels) {
      await expect(page.locator('.stat-label', { hasText: label })).toBeVisible();
    }
  });

  test('stat values are numeric', async ({ page }) => {
    await waitForDashboardData(page);
    const values = page.locator('.stat-value');
    const count = await values.count();
    for (let i = 0; i < count; i++) {
      const text = await values.nth(i).textContent();
      expect(text).toMatch(/^\d+/);
    }
  });

  // --- Charts ---
  test('charts grid has 4 charts', async ({ page }) => {
    const charts = page.locator('.charts-grid .chart-card');
    await expect(charts).toHaveCount(4);
  });

  test('chart titles are correct', async ({ page }) => {
    const titles = ['Task Status Distribution', 'Goal Status', 'Service Health', 'Agents'];
    for (const title of titles) {
      await expect(page.locator('.chart-title', { hasText: title })).toBeVisible();
    }
  });

  test('chart canvases render', async ({ page }) => {
    await expect(page.locator('#chart-tasks')).toBeVisible();
    await expect(page.locator('#chart-goals')).toBeVisible();
    await expect(page.locator('#chart-services')).toBeVisible();
    await expect(page.locator('#chart-agents')).toBeVisible();
  });

  // --- Theme Toggle ---
  test('theme toggle button exists', async ({ page }) => {
    await expect(page.locator('#btn-theme-toggle')).toBeVisible();
  });

  test('can toggle to light theme', async ({ page }) => {
    await page.click('#btn-theme-toggle');
    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).toBe('light');
  });

  test('can toggle back to dark theme', async ({ page }) => {
    await page.click('#btn-theme-toggle');
    await page.click('#btn-theme-toggle');
    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).toBe('dark');
  });

  test('theme persists across page reload', async ({ page }) => {
    await page.click('#btn-theme-toggle');
    await page.reload();
    await page.waitForLoadState('networkidle');
    const theme = await page.locator('html').getAttribute('data-theme');
    expect(theme).toBe('light');
  });

  // --- Refresh ---
  test('refresh button exists and is clickable', async ({ page }) => {
    await expect(page.locator('#btn-refresh')).toBeVisible();
    await page.click('#btn-refresh');
    await expect(page.locator('#refresh-status')).toBeVisible();
  });

  test('last updated timestamp shows after refresh', async ({ page }) => {
    await page.click('#btn-refresh');
    await page.waitForTimeout(2000);
    const text = await page.locator('#last-updated').textContent();
    expect(text).not.toBe('Last updated: never');
  });

  // --- API Docs Link ---
  test('API docs link exists', async ({ page }) => {
    const apiLink = page.locator('a.btn-sm.primary', { hasText: 'API Docs' });
    await expect(apiLink).toBeVisible();
    await expect(apiLink).toHaveAttribute('href', /swagger/);
  });

  // --- Dashboard Sections ---
  test('agents section is visible', async ({ page }) => {
    await expect(page.locator('#section-agents')).toBeVisible();
    await expect(page.locator('.section-name', { hasText: 'Agents' })).toBeVisible();
  });

  test('recent goals section is visible', async ({ page }) => {
    await expect(page.locator('#section-recent-goals')).toBeVisible();
    await expect(page.locator('.section-name', { hasText: 'Recent Goals' })).toBeVisible();
  });

  test('services section is visible', async ({ page }) => {
    await expect(page.locator('#section-services')).toBeVisible();
  });

  test('workers section is visible', async ({ page }) => {
    await expect(page.locator('#section-workers')).toBeVisible();
  });

  test('approvals section is visible', async ({ page }) => {
    await expect(page.locator('#section-approvals')).toBeVisible();
  });

  test('cost section is visible', async ({ page }) => {
    await expect(page.locator('#section-cost')).toBeVisible();
  });

  test('logs section is visible', async ({ page }) => {
    await expect(page.locator('#section-logs')).toBeVisible();
  });

  test('PIDs section is visible', async ({ page }) => {
    await expect(page.locator('#section-pids')).toBeVisible();
  });

  // --- Sidebar ---
  test('sidebar has health card', async ({ page }) => {
    await expect(page.locator('#sidebar-health')).toBeVisible();
    await expect(page.locator('.sidebar-card-title', { hasText: 'Health at a Glance' })).toBeVisible();
  });

  test('sidebar has task queue card', async ({ page }) => {
    await expect(page.locator('#sidebar-task-queue')).toBeVisible();
    await expect(page.locator('.sidebar-card-title', { hasText: 'Task Queue' })).toBeVisible();
  });

  test('sidebar shows waiting and running counts', async ({ page }) => {
    await expect(page.locator('#task-queue-waiting')).toBeVisible();
    await expect(page.locator('#task-queue-running')).toBeVisible();
  });

  test('health donut chart renders', async ({ page }) => {
    await expect(page.locator('#chart-health-donut')).toBeVisible();
  });
});
