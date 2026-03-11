import Head from 'next/head';
import Hero from '../components/Hero';
import FeaturedProductsGrid from '../components/FeaturedProductsGrid';

const Home = () => {
  return (
    <div>
      <Head>
        <title>My App - Home</title>
      </Head>
      <Hero />
      <FeaturedProductsGrid />
    </div>
  );
};

export default Home;