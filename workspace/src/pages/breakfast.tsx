import React from 'react';

const BreakfastMenuItem = ({ name, category, ingredients }: { name: string; category: string; ingredients: string[] }) => {
    return (
        <div className="p-4 border rounded-lg mb-4">
            <h2 className="text-xl font-bold">{name}</h2>
            <p className="text-lg mt-1">Category: {category}</p>
            <ul className="mt-2">
                {ingredients.map((ingredient, index) => (
                    <li key={index} className="mb-1">
                        {ingredient}
                    </li>
                ))}
            </ul>
        </div>
    );
};

const BreakfastMenu = () => {
    const breakfastItems = [
        {
            name: "Bacon and Eggs",
            category: "American",
            ingredients: ["Bacon", "Eggs"]
        },
        {
            name: "Avocado Toast",
            category: "Healthy",
            ingredients: ["Avocado", "Whole Wheat Bread"]
        },
        {
            name: "French Toast",
            category: "Breakfast for Kids",
            ingredients: ["Bread", "Eggs"]
        }
    ];

    return (
        <div className="container mx-auto p-4">
            <h1 className="text-3xl font-bold mb-4">Breakfast Menu</h1>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                {breakfastItems.map((item, index) => (
                    <BreakfastMenuItem key={index} {...item} />
                ))}
            </div>
        </div>
    );
};

export default BreakfastMenu;