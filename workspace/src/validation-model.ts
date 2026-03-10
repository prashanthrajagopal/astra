// src/validation-model.ts
import { z } from 'zod';

export const GoalSchema = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string(),
  targetDate: z.date(),
  status: z.enum(['Not Started', 'In Progress', 'Completed']),
});

export type GoalRequest = z.infer<typeof GoalSchema>;

export const GoalResponse = z.object({
  id: z.string(),
  title: z.string(),
  description: z.string(),
  targetDate: z.date(),
  status: z.enum(['Not Started', 'In Progress', 'Completed']),
  createdAt: z.date(),
  updatedAt: z.date(),
});

export type GoalResponseData = z.infer<typeof GoalResponse>;