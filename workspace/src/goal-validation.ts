// src/goal-validation.ts
import { NextApiRequest } from 'next/api';
import { NextApiResponse } from 'next/api';

export const validateGoalRequest = (req: NextApiRequest) => {
  if (!validateRequest(req)) {
    throw new Error('Invalid request');
  }
  const { goalName, goalDescription, goalTargetDate } = req.body;
  if (!goalName || typeof goalName !== 'string') {
    throw new Error('Invalid goal name');
  }
  if (!goalDescription || typeof goalDescription !== 'string') {
    throw new Error('Invalid goal description');
  }
  if (!goalTargetDate || typeof goalTargetDate !== 'string') {
    throw new Error('Invalid goal target date');
  }
  return true;
};

export const validateGoalResponse = (res: NextApiResponse) => {
  if (!validateResponse(res)) {
    throw new Error('Invalid response');
  }
  const { goalId, goalName, goalStatus } = res.body;
  if (!goalId || typeof goalId !== 'string') {
    throw new Error('Invalid goal ID');
  }
  if (!goalName || typeof goalName !== 'string') {
    throw new Error('Invalid goal name');
  }
  if (!goalStatus || typeof goalStatus !== 'string') {
    throw new Error('Invalid goal status');
  }
  return true;
};

export const validateGoal = (req: NextApiRequest, res: NextApiResponse) => {
  if (!validateGoalRequest(req)) {
    throw new Error('Goal request validation failed');
  }
  if (!validateGoalResponse(res)) {
    throw new Error('Goal response validation failed');
  }
  return true;
};

export const validateGoalUpdate = (req: NextApiRequest, res: NextApiResponse) => {
  if (!validateGoalRequest(req)) {
    throw new Error('Goal update request validation failed');
  }
  if (!validateGoalResponse(res)) {
    throw new Error('Goal update response validation failed');
  }
  return true;
};