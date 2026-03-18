import React from "react";

const DinnerMenuItem = ({ name, category, ingredients }: { name: string; category: string; ingredients: string[] }) => {
  return (
    <div className="p-4 border-b last:border-0">
      <h2 className="text-lg font-semibold">{name}</h2>
      <p className="mt-1 text-sm">Category: {category}</p>
      <ul className="mt-2 space-y-1">
        {ingredients.map((ingredient, index) => (
          <li key={index} className="text-sm">
            {ingredient}
          </li>
        ))}
      </ul>
    </div>
  );
};

const DinnerMenu = () => {
  const menuItems = [
    {
      name: "Grilled Salmon",
      category: "Main Course",
      ingredients: ["Salmon fillet", "Olive oil", "Lemon slices", "Herb mix"]
    },
    {
      name: "Caprese Salad",
      category: "Appetizer",
      ingredients: ["Tomatoes", "Fresh mozzarella", "Basil leaves", "Balsamic glaze"]
    },
    {
      name: "Pesto Pasta",
      category: "Main Course",
      ingredients: ["Pasta", "Pesto sauce", "Grated Parmesan"]
    }
  ];

  return (
    <div className="container mx-auto py-8">
      <h1 className="text-4xl font-bold mb-6">Dinner Menu</h1>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-x-8 gap-y-6">
        {menuItems.map((item, index) => (
          <DinnerMenuItem key={index} {...item} />
        ))}
      </div>
    </div>
  );
};

export default DinnerMenu;