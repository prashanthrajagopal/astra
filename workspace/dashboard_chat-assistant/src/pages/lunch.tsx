import { useState } from "react";
import Link from "next/link";

interface MenuItem {
  name: string;
  category: string;
  ingredients: string[];
}

const menuItems: MenuItem[] = [
  {
    name: "Grilled Chicken Salad",
    category: "Salad",
    ingredients: ["grilled chicken", "mixed greens", "cucumber", "tomato", "cheddar cheese"]
  },
  {
    name: "Beef Burrito",
    category: "Burrito",
    ingredients: ["steak", "queso", "lettuce", "tomato"]
  },
  {
    name: "Vegetarian Wrap",
    category: "Wrap",
    ingredients: ["spinach", "mushrooms", "peppers", "olives", "cheddar cheese"]
  }
];

const LunchMenu = () => {
  return (
    <div className="max-w-screen-xl mx-auto py-16 px-4">
      <h2 className="text-3xl font-bold mb-8">Lunch Menu</h2>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-8">
        {menuItems.map((item) => (
          <Link key={item.name} href={`/lunch/${item.name}`}>
            <a className="block border-2 shadow-md rounded-lg transition-transform duration-305 transform hover:scale-105">
              <div className="p-4 bg-white border-b">
                <h3 className="text-xl font-semibold mb-2">{item.name}</h3>
                <p className="mb-4 text-gray-600">{item.category}</p>
              </div>
              <ul className="space-y-2">
                {item.ingredients.map((ingredient) => (
                  <li key={ingredient} className="text-gray-700">
                    {ingredient}
                  </li>
                ))}
              </ul>
            </a>
          </Link>
        ))}
      </div>
    </div>
  );
};

export default LunchMenu;