import Head from 'next/head';
import { Product } from '../types';
import ProductDetail from '../components/ProductDetail';

const ProductPage = ({ product }: { product: Product }) => {
  return (
    <div>
      <Head>
        <title>{product.name} | Your Online Store</title>
      </Head>
      <ProductDetail product={product} />
    </div>
  );
};

export async function getStaticProps() {
  const product: Product = await fetch(`https://your-api.com/products/${[id]}`)
    .then(response => response.json())
    .then(data => data);

  return { props: { product } };
}

export default ProductPage;