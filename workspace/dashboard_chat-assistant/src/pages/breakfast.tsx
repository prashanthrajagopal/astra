import React from 'react';
import { twMerge } from 'tailwind-merge';

interface MenuItem {
  name: string;
  category: string;
  ingredients: string[];
}

const BreakfastMenu = ({ items }: { items: MenuItem[] }) => {
  return (
    <div className="max-w-6xl mx-auto py-8 px-4 sm:px-6 lg:py-12">
      <h1 className="text-3xl font-extrabold text-center">Breakfast Menu</h1>
      <div className="mt-8 grid gap-y-10 sm:grid-cols-2 lg:grid-cols-3">
        {items.map((item) => (
          <div
            key={item.name}
            className="bg-white rounded-lg shadow-md overflow-hidden"
          >
            <div
              className={twMerge(
                "h-40 bg-cover",
                `bg-center`,
                "rounded-t-lg"
              )}
              style={{
                backgroundImage: `url("https://example.com/${item.name}.jpg")`,
              }}
            />
            <div className="p-6">
              <h3 className="text-xl font-bold mb-2">{item.name}</h3>
              <p className="mb-2"><span className="font-semibold">Category:</span> {item.category}</p>
              <ul className="list-disc pl-5 mb-2">
                {item.ingredients.map((ingredient, index) => (
                  <li key={index}>{ingredient}</li>
                ))}
              </ul>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default BreakfastMenu;