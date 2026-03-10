// src/api-server.ts
import { NextApiRequest, NextApiResponse } from 'next';
import { v4 as uuidv4 } from 'uuid';

interface Goal {
  id: string;
  title: string;
  description: string;
}

const goalList: Goal[] = [];

const apiServer = async (req: NextApiRequest, res: NextApiResponse) => {
  switch (req.method) {
    case 'GET':
      if (req.url === '/goals') {
        const goals = goalList.map((goal) => goal);
        res.status(200).json(goals);
      } else if (req.url.match(/\/goals\/[a-zA-Z0-9]+$/)) {
        const id = req.url.split('/').pop();
        const goal = goalList.find((goal) => goal.id === id);
        if (goal) {
          res.status(200).json(goal);
        } else {
          res.status(404).json({ message: 'Not Found' });
        }
      } else {
        res.status(404).json({ message: 'Not Found' });
      }
      break;
    case 'POST':
      if (req.url === '/goals') {
        const goal: Goal = {
          id: uuidv4(),
          title: req.body.title,
          description: req.body.description,
        };
        goalList.push(goal);
        res.status(201).json(goal);
      } else {
        res.status(404).json({ message: 'Not Found' });
      }
      break;
    case 'PUT':
      if (req.url.match(/\/goals\/[a-zA-Z0-9]+$/)) {
        const id = req.url.split('/').pop();
        const existingGoal = goalList.find((goal) => goal.id === id);
        if (existingGoal) {
          existingGoal.title = req.body.title;
          existingGoal.description = req.body.description;
          res.status(200).json(existingGoal);
        } else {
          res.status(404).json({ message: 'Not Found' });
        }
      } else {
        res.status(404).json({ message: 'Not Found' });
      }
      break;
    case 'DELETE':
      if (req.url.match(/\/goals\/[a-zA-Z0-9]+$/)) {
        const id = req.url.split('/').pop();
        goalList = goalList.filter((goal) => goal.id !== id);
        res.status(204).send();
      } else {
        res.status(404).json({ message: 'Not Found' });
      }
      break;
    default:
      res.status(405).json({ message: 'Method Not Allowed' });
  }
};

export default apiServer;