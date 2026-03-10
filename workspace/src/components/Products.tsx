import { Grid, GridItem } from '@chakra-ui/react';
import ProductCard from './ProductCard';

const Products = ({ products, category, sortBy }) => {
  const filteredProducts = products.filter((product) => {
    if (category === 'all') {
      return true;
    }
    return product.category === category;
  });

  const sortedProducts = filteredProducts.sort((a, b) => {
    if (sortBy === 'price') {
      return a.price - b.price;
    }
    if (sortBy === 'name') {
      return a.name.localeCompare(b.name);
    }
    if (sortBy === 'rating') {
      return a.rating - b.rating;
    }
    return 0;
  });

  return (
    <Grid templateColumns="repeat(3, 1fr)" gap={4}>
      {sortedProducts.map((product) => (
        <GridItem key={product.id}>
          <ProductCard product={product} />
        </GridItem>
      ))}
    </Grid>
  );
};

export default Products;