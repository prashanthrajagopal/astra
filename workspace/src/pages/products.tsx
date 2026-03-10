import Head from 'next/head';
import { useState, useEffect } from 'react';
import { Grid, GridItem, Flex, Divider, Box } from '@chakra-ui/react';
import Products from '../components/Products';
import ProductCard from '../components/ProductCard';
import Sidebar from '../components/Sidebar';

const ProductsPage = () => {
  const [products, setProducts] = useState([]);
  const [category, setCategory] = useState('all');
  const [sortBy, setSortBy] = useState('price');

  useEffect(() => {
    // fetch products from API
    const productsData = [...]; // replace with actual API response
    setProducts(productsData);
  }, []);

  const handleCategoryChange = (newCategory: string) => {
    setCategory(newCategory);
  };

  const handleSortChange = (newSortBy: string) => {
    setSortBy(newSortBy);
  };

  return (
    <Box>
      <Head>
        <title>Product Listing</title>
      </Head>
      <Flex direction="row" gap={4}>
        <Sidebar category={category} onChange={handleCategoryChange} />
        <Grid
          templateColumns="repeat(3, 1fr)"
          gap={4}
          justifyContent="center"
        >
          {products.map((product) => (
            <GridItem key={product.id}>
              <ProductCard product={product} />
            </GridItem>
          ))}
        </Grid>
      </Flex>
    </Box>
  );
};

export default ProductsPage;