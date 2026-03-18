import React from 'react';
import { Box, Grid, Text } from '@chakra-ui/react';

const DinnerMenu = () => {
  const menuItems = [
    {
      name: 'Grilled Salmon',
      category: 'Entree',
      ingredients: ['Salmon', 'Lemon', 'Garlic']
    },
    {
      name: 'Caesar Salad',
      category: 'Salad',
      ingredients: ['Romaine lettuce', 'Crumble Parmesan', 'Croutons']
    },
    {
      name: 'Baked Ziti',
      category: 'Entree',
      ingredients: ['Ziti pasta', 'Tomato sauce', 'Cheese']
    },
    {
      name: 'Caprese Skewers',
      category: 'Appetizer',
      ingredients: ['Tomatoes', 'Fresh mozzarella', 'Basil']
    },
    {
      name: 'Mixed Fruit Platter',
      category: 'Dessert',
      ingredients: ['Strawberries', 'Blueberries', 'Watermelon']
    }
  ];

  return (
    <Box p={8} bg="gray.100" h="full">
      <Text fontSize="2xl" mb={4} textAlign="center">Dinner Menu</Text>
      <Grid templateColumns={['repeat(1, 1fr)', 'repeat(2, 1fr)']} gap={8}>
        {menuItems.map((item, index) => (
          <Box key={index} bg="white" p={4} rounded="md">
            <Text fontWeight="bold">{item.name}</Text>
            <Text fontSize="sm" mt={2}>
              {item.category}
            </Text>
            <ul>
              {item.ingredients.map((ingredient, idx) => (
                <li key={idx}>{ingredient}</li>
              ))}
            </ul>
          </Box>
        ))}
      </Grid>
    </Box>
  );
};

export default DinnerMenu;