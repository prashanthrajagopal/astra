import Head from 'next/head';
import { useState, useEffect } from 'react';
import { useRouter } from 'next/router';
import { Container, Grid, Sidebar, CategoryFilter, ProductCard } from '../components';
import { getProductList } from '../api/products';

const Products = () => {
  const [products, setProducts] = useState([]);
  const [category, setCategory] = useState('all');
  const [sort, setSort] = useState('price');
  const router = useRouter();

  useEffect(() => {
    const fetchProducts = async () => {
      const response = await getProductList(category, sort);
      setProducts(response.data);
    };
    fetchProducts();
  }, [category, sort]);

  const handleCategoryChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setCategory(event.target.value);
  };

  const handleSortChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setSort(event.target.value);
  };

  return (
    <Container>
      <Head>
        <title>Products</title>
      </Head>
      <Grid templateColumns={['repeat(3, 1fr)', 'repeat(4, 1fr)', 'repeat(6, 1fr)']} gap={6}>
        {products.map((product) => (
          <ProductCard key={product.id} product={product} />
        ))}
      </Grid>
      <Sidebar>
        <CategoryFilter
          value={category}
          onChange={handleCategoryChange}
          options={['all', 'electronics', 'fashion', 'home']}
        />
        <div>
          <label>Sort by:</label>
          <select value={sort} onChange={handleSortChange}>
            <option value="price">Price</option>
            <option value="name">Name</option>
            <option value="rating">Rating</option>
          </select>
        </div>
      </Sidebar>
    </Container>
  );
};

export default Products;