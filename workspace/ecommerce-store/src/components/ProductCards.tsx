import { VStack, HStack, Image, Text, Flex, Box } from '@chakra/react';
import { useState } from 'react';

interface ProductProps {
  product: {
    id: string;
    name: string;
    price: number;
    rating: number;
    image: string;
  };
}

const ProductCards = ({ product }: ProductProps) => {
  return (
    <VStack align="start" spacing={4}>
      <Image src={product.image} width={100} height={100} />
      <HStack justify="space-between">
        <Text fontSize="lg" fontWeight="bold">
          {product.name}
        </Text>
        <Flex justify="space-between" alignItems="center">
          <Text fontSize="lg" color="gray">
            {product.price}$
          </Text>
          <Box>
            <Text fontSize="lg" color="gray">
              {product.rating}/5
            </Text>
          </Box>
        </Flex>
      </HStack>
    </VStack>
  );
};

export default ProductCards;