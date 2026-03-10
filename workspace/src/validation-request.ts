// src/validation-request.ts
import { NextApiRequest } from 'next/api';

export const validateRequestBody = (req: NextApiRequest) => {
  if (!req.body || typeof req.body !== 'object') {
    throw new Error('Invalid request body');
  }
  if (!req.body.required || typeof req.body.required !== 'object') {
    throw new Error('Invalid request body required field');
  }
  const requiredKeys = Object.keys(req.body.required);
  for (const key of requiredKeys) {
    if (!req.body[key]) {
      throw new Error(`Missing required field: ${key}`);
    }
  }
  return true;
};

export const validateRequestQueryParams = (req: NextApiRequest) => {
  if (!req.query || typeof req.query !== 'object') {
    throw new Error('Invalid query parameters');
  }
  const queryKeys = Object.keys(req.query);
  for (const key of queryKeys) {
    if (typeof req.query[key] !== 'string') {
      throw new Error(`Invalid query parameter value for: ${key}`);
    }
  }
  return true;
};

export const validateRequest = (req: NextApiRequest) => {
  if (!validateRequestBody(req)) {
    throw new Error('Request body validation failed');
  }
  if (!validateRequestQueryParams(req)) {
    throw new Error('Query parameter validation failed');
  }
  return true;
};