export interface MealItem {
  name: string;
  category: "breakfast" | "lunch" | "dinner";
  ingredients: string[];
}