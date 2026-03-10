import Head from 'next/head';
import { useState, useEffect } from 'react';
import { Container, Grid, Row, Col } from '@nextui-org/react';
import { Link } from 'next/link';
import { fetchProducts } from '../api/products';
import { FeaturedProduct } from '../components/FeaturedProduct';
import { ProductCard } from '../components/ProductCard';

const HomePage = () => {
  const [products, setProducts] = useState([]);
  const [filteredProducts, setFilteredProducts] = useState([]);

  useEffect(() => {
    const fetch = async () => {
      const products = await fetchProducts();
      setProducts(products);
      setFilteredProducts(products);
    };
    fetch();
  }, []);

  const handleFilter = (category: string) => {
    setFilteredProducts(products.filter((product) => product.category === category));
  };

  return (
    <Container>
      <Head>
        <title>Home Page</title>
      </Head>
      <Grid.Container gap={2} justify="center">
        <Grid xs={12} sm={6} md={4} lg={3} xl={2} xxl={1}>
          <Row>
            <Col>
              <h1 className="text-4xl font-bold">Welcome to Our Store!</h1>
            </Col>
          </Row>
        </Grid>
        <Grid xs={12} sm={6} md={8} lg={10} xl={12} xxl={12}>
          <Row>
            <Col>
              <section className="hero bg-cover bg-center h-screen" style={{ backgroundImage: `url('/images/hero.jpg')` }}>
                <h2 className="text-3xl font-bold text-white">Discover the latest products!</h2>
              </section>
            </Col>
          </Row>
        </Grid>
        <Grid xs={12} sm={6} md={8} lg={10} xl={12} xxl={12}>
          <Row>
            <Col>
              <h2 className="text-2xl font-bold">Featured Products</h2>
              <ul className="flex flex-wrap -mx-2">
                {products.slice(0, 4).map((product) => (
                  <li key={product.id} className="mx-2 w-full md:w-1/2 xl:w-1/3">
                    <FeaturedProduct product={product} />
                  </li>
                ))}
              </ul>
            </Col>
          </Row>
        </Grid>
        <Grid xs={12} sm={6} md={8} lg={10} xl={12} xxl={12}>
          <Row>
            <Col>
              <h2 className="text-2xl font-bold">Categories</h2>
              <ul className="flex flex-wrap -mx-2">
                {['Electronics', 'Fashion', 'Home & Kitchen', 'Toys & Games'].map((category) => (
                  <li key={category} className="mx-2 w-full md:w-1/2 xl:w-1/3">
                    <Link href={`/categories/${category}`} passHref>
                      <a className="bg-orange-500 hover:bg-orange-700 text-orange-50 font-bold py-2 px-4 rounded">
                        {category}
                      </a>
                    </Link>
                  </li>
                ))}
              </ul>
            </Col>
          </Row>
        </Grid>
      </Grid.Container>
      <Grid.Container gap={2} justify="center">
        <Grid xs={12} sm={6} md={8} lg={10} xl={12} xxl={12}>
          <Row>
            <Col>
              <h2 className="text-2xl font-bold">Products</h2>
              <ul className="flex flex-wrap -mx-2">
                {filteredProducts.map((product) => (
                  <li key={product.id} className="mx-2 w-full md:w-1/2 xl:w-1/3">
                    <ProductCard product={product} />
                  </li>
                ))}
              </ul>
            </Col>
          </Row>
        </Grid>
      </Grid.Container>
    </Container>
  );
};

export default HomePage;