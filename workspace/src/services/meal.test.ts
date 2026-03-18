import { describe, test, expect } from '@jest/globals';
import { getMealData } from '@/services/meal';

describe('Get Meal Data', () => {
  test('Returns breakfast data correctly', async () => {
    const mealData = await getMealData('breakfast');
    expect(mealData).toHaveProperty('breakfast');
    expect(mealData.breakfast).toBeArrayOfSize(5);
  });

  test('Returns lunch data correctly', async () => {
    const mealData = await getMealData('lunch');
    expect(mealData).toHaveProperty('lunch');
    expect(mealData.lunch).toBeArrayOfSize(5);
  });

  test('Returns dinner data correctly', async () => {
    const mealData = await getMealData('dinner');
    expect(mealData).toHaveProperty('dinner');
    expect(mealData.dinner).toBeArrayOfSize(5);
  });
});