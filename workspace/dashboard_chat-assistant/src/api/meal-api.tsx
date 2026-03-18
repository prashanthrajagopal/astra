import { NextApiRequest, NextApiResponse } from 'next';
import { getMealData, MealDataByType, MealData } from '../../services/meal';

export default async function handler(req: NextApiRequest, res: NextApiResponse) {
  const mealType = req.query.type as 'breakfast' | 'lunch' | 'dinner';
  const mealData = getMealData(mealType);

  if (!mealData || mealData.length === 0) {
    return res.status(404).json({ error: 'Meal data not found' });
  }

  res.status(200).json(mealData);
}