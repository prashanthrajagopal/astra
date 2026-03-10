import { Flex, Box, Image, Text, Divider } from '@chakra-ui/react';

interface Product {
  id: string;
  name: string;
  category: string;
  price: number;
  rating: number;
}

const ProductCard = ({ product }: { product: Product }) => {
  return (
    <Box
      maxW="sm"
      overflow="hidden"
      rounded="md"
      boxShadow="lg"
      p={4}
    >
      <Image src={product.image} alt={product.name} />
      <Text fontSize="lg" fontWeight="bold">
        {product.name}
      </Text>
      <Text fontSize="md">
        {product.category}
      </Text>
      <Text fontSize="md" color="gray">
        ${product.price}
      </Text>
      <Text fontSize="sm" color="gray">
        Rating: {product.rating}/5
      </Text>
      <Divider />
    </Box>
  );
};

export default ProductCard;