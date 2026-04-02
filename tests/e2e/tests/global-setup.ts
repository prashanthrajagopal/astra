import { test as setup } from '@playwright/test';

setup('verify astra is running', async ({ request }) => {
  const response = await request.get('/health');
  if (!response.ok()) {
    throw new Error(
      `Astra is not running at ${process.env.ASTRA_BASE_URL || 'http://localhost:8080'}. ` +
      'Start Astra with ./scripts/deploy.sh before running tests.'
    );
  }
});
