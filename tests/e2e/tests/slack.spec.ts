import { test, expect } from '@playwright/test';
import { loginAndGotoDashboard, switchTab } from './helpers';

test.describe('Slack Configuration @dashboard', () => {
  test.beforeEach(async ({ page }) => {
    await loginAndGotoDashboard(page);
    await switchTab(page, 'slack');
  });

  test('slack tab shows configuration form', async ({ page }) => {
    await expect(page.locator('.slack-config-form')).toBeVisible();
  });

  test('slack config has signing secret field', async ({ page }) => {
    await expect(page.locator('#slack-signing-secret')).toBeVisible();
    await expect(page.locator('#slack-signing-secret')).toHaveAttribute('type', 'password');
  });

  test('slack config has client ID field', async ({ page }) => {
    await expect(page.locator('#slack-client-id')).toBeVisible();
  });

  test('slack config has client secret field', async ({ page }) => {
    await expect(page.locator('#slack-client-secret')).toBeVisible();
    await expect(page.locator('#slack-client-secret')).toHaveAttribute('type', 'password');
  });

  test('slack config has OAuth redirect URL field', async ({ page }) => {
    await expect(page.locator('#slack-oauth-redirect')).toBeVisible();
    await expect(page.locator('#slack-oauth-redirect')).toHaveAttribute('type', 'url');
  });

  test('save button exists', async ({ page }) => {
    await expect(page.locator('#btn-save-slack-config')).toBeVisible();
    await expect(page.locator('#btn-save-slack-config')).toHaveText('Save');
  });

  test('slack description links to Slack API', async ({ page }) => {
    const link = page.locator('.section-desc a[href*="api.slack.com"]');
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute('href', 'https://api.slack.com/apps');
  });

  test('can enter slack config values', async ({ page }) => {
    await page.fill('#slack-client-id', 'test-client-id');
    await page.fill('#slack-oauth-redirect', 'https://test.example.com/callback');
    await expect(page.locator('#slack-client-id')).toHaveValue('test-client-id');
    await expect(page.locator('#slack-oauth-redirect')).toHaveValue('https://test.example.com/callback');
  });
});
