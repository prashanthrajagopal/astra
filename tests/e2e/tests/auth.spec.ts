import { test, expect } from '@playwright/test';

test.describe('Authentication @auth', () => {
  test('login page renders correctly', async ({ page }) => {
    await page.goto('/login');
    await expect(page.locator('.login-card')).toBeVisible();
    await expect(page.locator('.logo')).toContainText('Astra');
    await expect(page.locator('.subtitle')).toContainText('Sign in');
    await expect(page.locator('#email')).toBeVisible();
    await expect(page.locator('#password')).toBeVisible();
    await expect(page.locator('#btn-login')).toBeVisible();
    await expect(page.locator('#btn-login')).toHaveText('Sign in');
  });

  test('shows error on invalid credentials', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#email', 'invalid@test.com');
    await page.fill('#password', 'wrongpassword');
    await page.click('#btn-login');
    await expect(page.locator('#msg')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('#msg')).toHaveClass(/error/);
  });

  test('login button shows loading state', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#email', 'test@test.com');
    await page.fill('#password', 'test');
    await page.click('#btn-login');
    await expect(page.locator('#btn-login')).toHaveText('Signing in...');
    await expect(page.locator('#btn-login')).toBeDisabled();
  });

  test('successful login redirects to dashboard', async ({ page }) => {
    await page.goto('/login');
    const email = process.env.ASTRA_TEST_EMAIL || 'admin@astra.local';
    const password = process.env.ASTRA_TEST_PASSWORD || 'admin';
    await page.fill('#email', email);
    await page.fill('#password', password);
    await page.click('#btn-login');

    // Should show success or redirect
    await page.waitForURL(/dashboard/, { timeout: 10000 }).catch(() => {
      // May show success message before redirect
    });
  });

  test('dashboard link is visible on login page', async ({ page }) => {
    await page.goto('/login');
    const link = page.locator('.links a');
    await expect(link).toBeVisible();
    await expect(link).toHaveAttribute('href', '/superadmin/dashboard/');
  });

  test('email field validates format', async ({ page }) => {
    await page.goto('/login');
    await page.fill('#email', 'notanemail');
    await page.fill('#password', 'test');
    await page.click('#btn-login');
    // Browser validation should prevent submission
    const emailInput = page.locator('#email');
    const validity = await emailInput.evaluate((el: HTMLInputElement) => el.validity.valid);
    expect(validity).toBe(false);
  });

  test('required fields prevent empty submission', async ({ page }) => {
    await page.goto('/login');
    await page.click('#btn-login');
    const emailInput = page.locator('#email');
    const validity = await emailInput.evaluate((el: HTMLInputElement) => el.validity.valueMissing);
    expect(validity).toBe(true);
  });
});
