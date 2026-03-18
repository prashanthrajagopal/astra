import { Link } from 'next/router';
import MealCategoryLink from '../components/MealCategoryLink';

export default function Home() {
  return (
    <div className="p-8">
      <h1 className="text-3xl font-bold mb-4">Meal Menus</h1>
      <div className="flex gap-8">
        <MealCategoryLink meal="breakfast" />
        <MealCategoryLink meal="lunch" />
        <MealCategoryLink meal="dinner" />
      </div>
    </div>
  );
}