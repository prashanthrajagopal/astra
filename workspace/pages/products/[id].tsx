import { Layout } from '../components/Layout';
import { ProductDetail } from '../components/ProductDetail';

const ProductDetailPage = ({ params }) => {
  const product = // get product by ID from API or database
  return (
    <Layout>
      <ProductDetail product={product} />
    </Layout>
  );
};

export default ProductDetailPage;