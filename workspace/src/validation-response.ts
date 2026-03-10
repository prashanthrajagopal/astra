// src/validation-response.ts
import { NextApiResponse } from 'next/api';

export const validateResponseBody = (res: NextApiResponse) => {
  if (!res.body || typeof res.body !== 'object') {
    throw new Error('Invalid response body');
  }
  if (res.status >= 400) {
    throw new Error(`Invalid response status code: ${res.status}`);
  }
  return true;
};

export const validateResponse = (res: NextApiResponse) => {
  if (!validateResponseBody(res)) {
    throw new Error('Response validation failed');
  }
  return true;
};