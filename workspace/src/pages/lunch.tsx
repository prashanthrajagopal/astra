import React from 'react';
import styles from './lunch.module.css';

interface MenuItem {
  name: string;
  category: string;
  ingredients: string[];
}

const MenuItems: MenuItem[] = [
  {
    name: 'Grilled Chicken Caesar Wrap',
    category: 'Salad',
    ingredients: ['grilled chicken', 'romaine lettuce', 'parmesan cheese', 'caesar dressing', 'whole wheat wrap']
  },
  {
    name: 'Vegetarian Pasta',
    category: 'Entree',
    ingredients: ['pasta', 'tomato sauce', 'spinach', 'mushrooms', 'olive oil']
  },
  {
    name: 'Tomato Soup',
    category: 'Soup',
    ingredients: ['tomatoes', 'salt', 'basil']
  }
];

const LunchMenu = () => {
  return (
    <div className={styles.menu}>
      <h1>Lunch Menu</h1>
      {MenuItems.map((item, index) => (
        <div key={index} className={styles.item}>
          <h2>{item.name}</h2>
          <p>Category: {item.category}</p>
          <ul className={styles.ingredients}>
            {item.ingredients.map((ingredient, idx) => (
              <li key={idx}>{ingredient}</li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  );
};

export default LunchMenu;