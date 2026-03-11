import { VStack, HStack, Button, Text } from '@chakra/react';
import { useState } from 'react';

interface CategoryFilterSidebarProps {
  sortOption: string;
  setSortOption: (option: string) => void;
  filterOption: string;
  setFilterOption: (option: string) => void;
}

const CategoryFilterSidebar = ({
  sortOption,
  setSortOption,
  filterOption,
  setFilterOption,
}: CategoryFilterSidebarProps) => {
  const options = ['all', 'electronics', 'fashion', 'home'];

  return (
    <VStack align="stretch" spacing={4}>
      <HStack justify="space-between">
        <Text>Filter by category:</Text>
        <Button
          variant="link"
          colorScheme="gray"
          onClick={() => setFilterOption('all')}
        >
          All
        </Button>
        {options.map((option) => (
          <Button
            key={option}
            variant="link"
            colorScheme="gray"
            onClick={() => setFilterOption(option)}
          >
            {option}
          </Button>
        ))}
      </HStack>
      <HStack justify="space-between">
        <Text>Sort by:</Text>
        <Button
          variant="link"
          colorScheme="gray"
          onClick={() => setSortOption('price')}
        >
          Price
        </Button>
        <Button
          variant="link"
          colorScheme="gray"
          onClick={() => setSortOption('name')}
        >
          Name
        </Button>
        <Button
          variant="link"
          colorScheme="gray"
          onClick={() => setSortOption('rating')}
        >
          Rating
        </Button>
      </HStack>
    </VStack>
  );
};

export default CategoryFilterSidebar;