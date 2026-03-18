import Link from 'next/link';

interface MealCategoryLinkProps {
  meal: string;
}

export default function MealCategoryLink({ meal }: MealCategoryLinkProps) {
  return (
    <Link href={`/${meal}`} className="text-blue-500 hover:text-blue-700 transition duration-300 ease-in-out">
      {meal} Menu
    </Link>
  );
}