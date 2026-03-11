import { useState, useEffect } from 'react';
import { Grid, GridItem, Container } from '@clayui/core';
import ProductCard from '../ProductCard';

interface Product {
  id: number;
  name: string;
  image: string;
  price: number;
}

const featuredProducts: Product[] = [
  {
    id: 1,
    name: 'Product 1',
    image: 'https://via.placeholder.com/100x100',
    price: 19.99,
  },
  {
    id: 2,
    name: 'Product 2',
    image: 'https://via.placeholder.com/100x100',
    price: 29.99,
  },
  {
    id: 3,
    name: 'Product 3',
    image: 'https://via.placeholder.com/100x100',
    price: 39.99,
  },
  {
    id: 4,
    name: 'Product 4',
    image: 'https://via.placeholder.com/100x100',
    price: 49.99,
  },
  {
    id: 5,
    name: 'Product 5',
    image: 'https://via.placeholder.com/100x100',
    price: 59.99,
  },
  {
    id: 6,
    name: 'Product 6',
    image: 'https://via.placeholder.com/100x100',
    price: 69.99,
  },
];

const FeaturedProductsGrid = () => {
  const [filteredProducts, setFilteredProducts] = useState<Product[]>(featuredProducts);

  useEffect(() => {
    setFilteredProducts(featuredProducts);
  }, []);

  const handleCategoryChange = (category: string) => {
    const filteredProducts = featuredProducts.filter((product) => product.category === category);
    setFilteredProducts(filteredProducts);
  };

  return (
    <Container fluid={true} className="my-4">
      <Grid fluid={true} columns={3}>
        {filteredProducts.map((product) => (
          <GridItem key={product.id} xs={6} sm={4} md={3} lg={2} xl={1}>
            <ProductCard product={product} />
          </GridItem>
        ))}
      </Grid>
    </Container>
  );
};

export default FeaturedProductsGrid;