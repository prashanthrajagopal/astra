import { Layout } from '../components/Layout';
import { ProductGrid } from '../components/ProductGrid';
import { CategoryCards } from '../components/CategoryCards';

const HomePage = () => {
  return (
    <Layout>
      <section className="hero bg-indigo-700 h-screen flex justify-center items-center">
        {/* hero content */}
      </section>
      <ProductGrid />
      <CategoryCards />
    </Layout>
  );
};

export default HomePage;