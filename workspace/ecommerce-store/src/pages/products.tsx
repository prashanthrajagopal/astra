import Head from 'next/head';
import { useState, useEffect } from 'react';
import { Grid, GridItem, VStack } from '@chakra/react';
import { useSort } from '../hooks/useSort';
import { useFilter } from '../hooks/useFilter';
import ProductCards from '../components/ProductCards';
import CategoryFilterSidebar from '../components/CategoryFilterSidebar';

function Products() {
  const [sortOption, setSortOption] = useState('price');
  const [filterOption, setFilterOption] = useState('all');
  const { sortedProducts } = useSort(sortOption);
  const { filteredProducts } = useFilter(sortedProducts, filterOption);

  useEffect(() => {
    // You can put any initialization code here
  }, [sortOption, filterOption]);

  return (
    <VStack align="stretch" spacing={4}>
      <CategoryFilterSidebar
        sortOption={sortOption}
        setSortOption={setSortOption}
        filterOption={filterOption}
        setFilterOption={setFilterOption}
      />
      <Grid
        templateColumns={{
          base: 'repeat(1, 1fr)',
          md: 'repeat(2, 1fr)',
          lg: 'repeat(3, 1fr)',
        }}
        gap={4}
      >
        {filteredProducts.map((product) => (
          <GridItem key={product.id} />
          <ProductCards product={product} />
        ))}
      </Grid>
    </VStack>
  );
}

export default Products;