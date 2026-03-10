import { Layout } from '../components/Layout';
import { ProductList } from '../components/ProductList';
import { CategoryFilter } from '../components/CategoryFilter';
import { SortBy } from '../components/SortBy';

const ProductsPage = () => {
  return (
    <Layout>
      <CategoryFilter />
      <SortBy />
      <ProductList />
    </Layout>
  );
};

export default ProductsPage;