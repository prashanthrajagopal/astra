import { Grid, GridItem, Link, Stack, Text } from '@chakra-ui/react';
import { useGetProducts } from '../hooks/useGetProducts';

const Home = () => {
  const { products } = useGetProducts();

  return (
    <div className="bg-white">
      <h1 className="text-3xl font-bold">Featured Products</h1>
      <Grid
        templateColumns={[
          'repeat(2, 1fr)',
          'repeat(3, 1fr)',
          'repeat(4, 1fr)',
        ]}
        gap={6}
      >
        {products.map((product) => (
          <GridItem key={product.id} colSpan={1}>
            <Link href={`/products/${product.id}`}>
              <a>
                <img src={product.image} alt={product.name} />
                <Text>{product.name}</Text>
              </a>
            </Link>
          </GridItem>
        ))}
      </Grid>
    </div>
  );
};

export default Home;